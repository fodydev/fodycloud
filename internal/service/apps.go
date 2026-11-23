// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/repo"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// App is the public view of an application.
type App struct {
	ID             int64        `json:"-"`
	UserID         int64        `json:"-"`
	Name           string       `json:"name"`
	RepoURL        string       `json:"repo_url"`
	Branch         string       `json:"branch"`
	Status         string       `json:"status"`
	URL            string       `json:"url"`
	Domains        []string     `json:"custom_domains"`
	Stack          stacks.Stack `json:"stack"`
	StackConfirmed bool         `json:"stack_confirmed"`
	// HealthCheckPath is checked over HTTP before a new deployment goes live.
	HealthCheckPath string            `json:"health_check_path,omitempty"`
	Detection       *stacks.Detection `json:"detection,omitempty"`
	Deployment      *Deployment       `json:"current_deployment,omitempty"`
	LastRequestAt   *time.Time        `json:"last_request_at,omitempty"` // last visit seen by the monitor
	// StopsOn is set when the app is unused and will be stopped on that date
	// unless it is visited or deployed (abandoned-app rule of its plan).
	StopsOn         *time.Time `json:"stops_on,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	AbandonWarnedAt *time.Time `json:"-"`
	currentDepl     *int64
}

// NewApp is a request to create an app.
type NewApp struct {
	Name    string `json:"name"`
	RepoURL string `json:"repo_url"`
	Branch  string `json:"branch"`
	// Stack, when given with a language, is the student's explicit choice: the
	// app is built right away. Without it the repository is analysed first and
	// the student reviews the detected stack before anything is deployed; a
	// stack with only root_dir analyses that folder.
	Stack *stacks.Stack     `json:"stack,omitempty"`
	Env   map[string]string `json:"env"`
	// HealthCheckPath, optional, is the path checked before each deployment
	// goes live (see SetHealthCheck).
	HealthCheckPath string `json:"health_check_path,omitempty"`
}

var appName = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

var reservedNames = []string{"www", "api", "app", "apps", "admin", "mcp", "dashboard", "mail", "smtp", "status", "docs", "static", "assets", "traefik", "fodycloud"}

// ValidateAppName checks an app name, which is also its subdomain.
func ValidateAppName(name string) error {
	if !appName.MatchString(name) || strings.Contains(name, "--") {
		return invalid("app name must be 3-32 characters: lowercase letters, digits and single hyphens, starting with a letter")
	}
	if slices.Contains(reservedNames, name) {
		return invalid("app name %q is reserved", name)
	}
	return nil
}

const appColumns = `a.id, a.user_id, a.name, a.repo_url, a.branch, a.status, a.created_at, a.current_deployment_id,
	a.stack_language, a.stack_framework, a.stack_version, a.build_command, a.start_command, a.publish_dir, a.root_dir, a.stack_confirmed, a.detection,
	a.last_request_at, a.abandon_warned_at, a.health_path`

func scanApp(row pgx.Row) (*App, error) {
	var a App
	var detection []byte
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &a.RepoURL, &a.Branch, &a.Status, &a.CreatedAt, &a.currentDepl,
		&a.Stack.Language, &a.Stack.Framework, &a.Stack.Version, &a.Stack.BuildCommand, &a.Stack.StartCommand, &a.Stack.PublishDir, &a.Stack.RootDir,
		&a.StackConfirmed, &detection, &a.LastRequestAt, &a.AbandonWarnedAt, &a.HealthCheckPath)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil && len(detection) > 0 {
		a.Detection = &stacks.Detection{}
		err = json.Unmarshal(detection, a.Detection)
	}
	return &a, err
}

func (s *Service) decorate(ctx context.Context, a *App) error {
	a.URL = "https://" + s.Deployer.Hostname(a.Name)
	a.StopsOn = s.AbandonWarning(a)
	doms, err := s.appDomains(ctx, a.ID)
	if err != nil {
		return err
	}
	a.Domains = doms
	if a.currentDepl != nil {
		d, err := s.deployment(ctx, a.ID, *a.currentDepl)
		if err == nil {
			a.Deployment = d
		}
	}
	return nil
}

// ListApps lists the user's apps.
func (s *Service) ListApps(ctx context.Context, u *auth.User) ([]*App, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+appColumns+` FROM apps a WHERE a.user_id=$1 ORDER BY a.name`, u.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []*App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, a := range apps {
		if err := s.decorate(ctx, a); err != nil {
			return nil, err
		}
	}
	return apps, nil
}

// GetApp returns one of the user's apps. Apps of other users are reported as
// not found, so their existence is not leaked.
func (s *Service) GetApp(ctx context.Context, u *auth.User, name string) (*App, error) {
	a, err := scanApp(s.DB.QueryRow(ctx, `SELECT `+appColumns+` FROM apps a WHERE a.name=$1 AND a.user_id=$2`, name, u.ID))
	if err != nil {
		return nil, err
	}
	return a, s.decorate(ctx, a)
}

func requireWrite(u *auth.User) error {
	if !u.Can(auth.ScopeWrite) {
		return fmt.Errorf("%w: this token is read-only (scope %s required)", ErrForbidden, auth.ScopeWrite)
	}
	return nil
}

// CreateApp registers an app (build settings and initial environment
// included) after validating its repository, then queues its first build.
func (s *Service) CreateApp(ctx context.Context, u *auth.User, in NewApp) (*App, *Deployment, error) {
	if err := s.canBuild(u); err != nil {
		return nil, nil, err
	}
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if err := ValidateAppName(name); err != nil {
		return nil, nil, err
	}
	branch := strings.TrimSpace(in.Branch)
	if branch == "" {
		branch = "main"
	}
	r, err := repo.Parse(in.RepoURL)
	if err != nil {
		return nil, nil, &InvalidError{Msg: err.Error()}
	}
	if err := repo.ValidateBranch(branch); err != nil {
		return nil, nil, &InvalidError{Msg: err.Error()}
	}
	if s.RepoBanned(ctx, r.CloneURL) {
		return nil, nil, fmt.Errorf("%w: this repository is not allowed on FodyCloud", ErrForbidden)
	}
	var st stacks.Stack
	explicit := in.Stack != nil && strings.TrimSpace(in.Stack.Language) != ""
	if explicit {
		st = *in.Stack
		if err := st.Normalize(); err != nil {
			return nil, nil, invalid("%s", err.Error())
		}
	} else if in.Stack != nil { // only a working directory: analyse that folder
		dir, err := stacks.CleanRootDir(in.Stack.RootDir)
		if err != nil {
			return nil, nil, invalid("%s", err.Error())
		}
		st.RootDir = dir
	}
	if err := ValidateEnv(ptrMap(in.Env)); err != nil {
		return nil, nil, err
	}
	health, err := CleanHealthPath(in.HealthCheckPath)
	if err != nil {
		return nil, nil, err
	}
	info, err := s.PlanInfo(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	if info.AppsUsed >= info.Limits.MaxApps {
		return nil, nil, &QuotaError{Msg: fmt.Sprintf("the %s plan allows %d apps; delete an app or ask an admin for a bigger plan", info.Plan, info.Limits.MaxApps)}
	}
	if _, err := s.Git.ResolveBranch(ctx, r, branch); err != nil {
		return nil, nil, &InvalidError{Msg: err.Error()}
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var appID int64
	status := "detecting"
	if explicit {
		status = "building"
	}
	err = tx.QueryRow(ctx, `INSERT INTO apps(user_id, name, repo_url, branch, status, stack_language, stack_framework, stack_version,
		build_command, start_command, publish_dir, root_dir, stack_confirmed, health_path)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT (name) DO NOTHING RETURNING id`,
		u.ID, name, r.CloneURL, branch, status, st.Language, st.Framework, st.Version,
		st.BuildCommand, st.StartCommand, st.PublishDir, st.RootDir, explicit, health).Scan(&appID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, invalid("the name %q is already taken", name)
	}
	if err != nil {
		return nil, nil, err
	}
	for k, v := range in.Env {
		if _, err := tx.Exec(ctx, `INSERT INTO env_vars(app_id,key,value) VALUES ($1,$2,$3)`, appID, k, v); err != nil {
			return nil, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	kind := "detect"
	if explicit {
		kind = "build"
	}
	d, err := s.enqueue(ctx, appID, kind, "")
	if err != nil {
		return nil, nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	return a, d, err
}

// healthPath allows the characters of a URL path and query (RFC 3986), so
// the path can be sent as is in the health check's request line.
var healthPath = regexp.MustCompile(`^/[A-Za-z0-9._~!$&'()*+,;=:@%/?-]*$`)

// CleanHealthPath validates a health-check path ("" turns the check off).
func CleanHealthPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", nil
	}
	if len(p) > 200 || !healthPath.MatchString(p) {
		return "", invalid("the health-check path must start with / and contain only URL characters (no spaces), for example /healthz")
	}
	return p, nil
}

// SetHealthCheck sets the HTTP path checked before each new deployment of
// the app goes live ("" turns the check off). It applies from the next
// deployment.
func (s *Service) SetHealthCheck(ctx context.Context, u *auth.User, name, path string) (*App, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	p, err := CleanHealthPath(path)
	if err != nil {
		return nil, err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE apps SET health_path=$2, updated_at=now() WHERE id=$1`, a.ID, p); err != nil {
		return nil, err
	}
	a.HealthCheckPath = p
	return a, nil
}

// StopApp stops the app's containers.
func (s *Service) StopApp(ctx context.Context, u *auth.User, name string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return err
	}
	if err := s.Deployer.Stop(ctx, a.Name); err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE apps SET status='stopped', updated_at=now() WHERE id=$1`, a.ID)
	return err
}

// StartApp starts a stopped app (within the plan's app limit).
func (s *Service) StartApp(ctx context.Context, u *auth.User, name string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return err
	}
	if a.Status == "sleeping" {
		_, err := s.WakeApp(ctx, a.Name)
		return err
	}
	if a.Status != "stopped" {
		return invalid("app is %s, not stopped", a.Status)
	}
	info, err := s.PlanInfo(ctx, u)
	if err != nil {
		return err
	}
	var running int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM apps WHERE user_id=$1 AND status <> 'stopped'`, u.ID).Scan(&running); err != nil {
		return err
	}
	if running >= info.Limits.MaxApps {
		return &QuotaError{Msg: fmt.Sprintf("the %s plan allows %d running apps", info.Plan, info.Limits.MaxApps)}
	}
	if err := s.ensureCache(ctx, a.ID, a.Name, info.Limits); err != nil {
		return err
	}
	ok, err := s.Deployer.Start(ctx, a.Name)
	if err != nil {
		return err
	}
	if !ok {
		_, err = s.Restart(ctx, u, name)
		return err
	}
	_, err = s.DB.Exec(ctx, `UPDATE apps SET status='running', updated_at=now() WHERE id=$1`, a.ID)
	return err
}

// DeleteApp removes the app, its containers, images and history.
func (s *Service) DeleteApp(ctx context.Context, u *auth.User, name, confirm string) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	if confirm != name {
		return invalid("confirmation does not match the app name")
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return err
	}
	// Cancel pending work first so the worker does not resurrect the app.
	if _, err := s.DB.Exec(ctx, `UPDATE deployments SET status='cancelled' WHERE app_id=$1 AND status='queued'`, a.ID); err != nil {
		return err
	}
	if err := s.Deployer.Remove(ctx, a.Name); err != nil {
		return err
	}
	if err := s.dropStorage(ctx, a.ID, a.Name); err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM apps WHERE id=$1`, a.ID)
	return err
}
