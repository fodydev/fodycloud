// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/repo"
)

// requireAdmin guards admin-only methods. They take *auth.Admin, a separate
// account type, so a student can never be passed in; nil means no admin.
func requireAdmin(a *auth.Admin) error {
	if a == nil {
		return fmt.Errorf("%w: admins only", ErrForbidden)
	}
	return nil
}

// Ban suspends a user: apps stopped, sessions and tokens revoked, login refused.
func (s *Service) Ban(ctx context.Context, admin *auth.Admin, a *auth.Store, email, reason string) error {
	if err := requireAdmin(admin); err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return invalid("a reason is required")
	}
	var id int64
	err := s.DB.QueryRow(ctx, `UPDATE users SET banned_at=now(), ban_reason=$2 WHERE email=$1 RETURNING id`,
		strings.ToLower(strings.TrimSpace(email)), reason).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := a.RevokeAll(ctx, id); err != nil {
		return err
	}
	if err := s.stopAllApps(ctx, id); err != nil {
		return err
	}
	s.mail(ctx, strings.ToLower(strings.TrimSpace(email)), "banned", map[string]any{"Reason": reason})
	s.Log.Warn("user banned", "email", email, "by", admin.Email, "reason", reason)
	return nil
}

// Unban lifts a suspension (apps stay stopped until the user starts them).
func (s *Service) Unban(ctx context.Context, admin *auth.Admin, email string) error {
	if err := requireAdmin(admin); err != nil {
		return err
	}
	tag, err := s.DB.Exec(ctx, `UPDATE users SET banned_at=NULL, ban_reason='' WHERE email=$1`, strings.ToLower(strings.TrimSpace(email)))
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Service) stopAllApps(ctx context.Context, userID int64) error {
	rows, err := s.DB.Query(ctx, `SELECT name FROM apps WHERE user_id=$1 AND status <> 'stopped'`, userID)
	if err != nil {
		return err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `UPDATE deployments SET status='cancelled', finished_at=now()
		WHERE status='queued' AND app_id IN (SELECT id FROM apps WHERE user_id=$1)`, userID); err != nil {
		return err
	}
	var errs []error
	for _, n := range names {
		errs = append(errs, s.stopByName(ctx, n))
	}
	return errors.Join(errs...)
}

func (s *Service) stopByName(ctx context.Context, name string) error {
	if err := s.Deployer.Stop(ctx, name); err != nil {
		return err
	}
	if err := s.Deployer.StopValkey(ctx, name); err != nil { // recreated when the app starts again
		return err
	}
	_, err := s.DB.Exec(ctx, `UPDATE apps SET status='stopped', updated_at=now() WHERE name=$1`, name)
	return err
}

// BannedRepo is a repository refused by FodyCloud.
type BannedRepo struct {
	RepoURL   string    `json:"repo_url"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// BanRepo refuses a repository for new apps and builds.
func (s *Service) BanRepo(ctx context.Context, admin *auth.Admin, url, reason string) (string, error) {
	if err := requireAdmin(admin); err != nil {
		return "", err
	}
	r, err := repo.Parse(url)
	if err != nil {
		return "", &InvalidError{Msg: err.Error()}
	}
	var by *int64
	if admin.ID != 0 {
		by = &admin.ID
	}
	_, err = s.DB.Exec(ctx, `INSERT INTO banned_repos(repo_url, reason, banned_by) VALUES ($1,$2,$3)
		ON CONFLICT (repo_url) DO UPDATE SET reason=EXCLUDED.reason`, r.CloneURL, reason, by)
	return r.CloneURL, err
}

// UnbanRepo allows a repository again.
func (s *Service) UnbanRepo(ctx context.Context, admin *auth.Admin, url string) error {
	if err := requireAdmin(admin); err != nil {
		return err
	}
	r, err := repo.Parse(url)
	if err != nil {
		return &InvalidError{Msg: err.Error()}
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM banned_repos WHERE repo_url=$1`, r.CloneURL)
	return err
}

// ListBannedRepos lists banned repositories.
func (s *Service) ListBannedRepos(ctx context.Context, admin *auth.Admin) ([]BannedRepo, error) {
	if err := requireAdmin(admin); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT repo_url, reason, created_at FROM banned_repos ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (BannedRepo, error) {
		var b BannedRepo
		return b, r.Scan(&b.RepoURL, &b.Reason, &b.CreatedAt)
	})
}

// checkRepoAllowed refuses builds of banned repositories up front (the
// worker checks again in case the ban happened after queueing).
func (s *Service) checkRepoAllowed(ctx context.Context, a *App) error {
	if s.RepoBanned(ctx, a.RepoURL) {
		return fmt.Errorf("%w: this repository is not allowed on FodyCloud", ErrForbidden)
	}
	return nil
}

// RepoBanned reports whether a canonical clone URL is banned.
func (s *Service) RepoBanned(ctx context.Context, cloneURL string) bool {
	var banned bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM banned_repos WHERE repo_url=$1)`, cloneURL).Scan(&banned); err != nil {
		s.Log.Error("check banned repo", "err", err)
	}
	return banned
}

// AbuseFlag is shown to admins.
type AbuseFlag struct {
	ID        int64     `json:"id"`
	App       string    `json:"app"`
	Email     string    `json:"email"`
	Kind      string    `json:"kind"`
	Detail    string    `json:"detail"`
	Action    string    `json:"action"`
	CreatedAt time.Time `json:"created_at"`
}

// FlagAbuse records sustained abuse of an app. Free-plan apps flagged twice
// within 24 hours are stopped. Admins are notified.
func (s *Service) FlagAbuse(ctx context.Context, appName, kind, detail string) error {
	var appID, userID int64
	var email string
	err := s.DB.QueryRow(ctx, `SELECT a.id, a.user_id, u.email FROM apps a JOIN users u ON u.id=a.user_id WHERE a.name=$1`, appName).
		Scan(&appID, &userID, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // app deleted meanwhile
	}
	if err != nil {
		return err
	}
	var recent int
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM abuse_flags WHERE app_id=$1 AND kind=$2 AND created_at > now() - interval '24 hours'`,
		appID, kind).Scan(&recent); err != nil {
		return err
	}
	info, err := s.planInfo(ctx, userID)
	if err != nil {
		return err
	}
	action := ""
	if recent >= 1 && info.Plan == plans.Default {
		if err := s.stopByName(ctx, appName); err != nil {
			return err
		}
		action = "stopped"
		s.mail(ctx, email, "app-stopped-abuse", map[string]any{"App": appName})
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO abuse_flags(app_id, app_name, user_id, kind, detail, action) VALUES ($1,$2,$3,$4,$5,$6)`,
		appID, appName, userID, kind, detail, action); err != nil {
		return err
	}
	s.Log.Warn("abuse flag", "app", appName, "kind", kind, "detail", detail, "action", action)
	s.NotifyAdmins(ctx, fmt.Sprintf("%s abuse: %s", kind, appName),
		fmt.Sprintf("App %s (owner %s, %s plan): %s.\nAction: %s\nReview it on the Abuse admin page.", appName, email, info.Plan, detail, orNone(action)))
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none (flag only)"
	}
	return s
}

// ListAbuseFlags lists open flags (admins only).
func (s *Service) ListAbuseFlags(ctx context.Context, admin *auth.Admin) ([]AbuseFlag, error) {
	if err := requireAdmin(admin); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT f.id, f.app_name, COALESCE(u.email,''), f.kind, f.detail, f.action, f.created_at
		FROM abuse_flags f LEFT JOIN users u ON u.id=f.user_id WHERE f.resolved_at IS NULL ORDER BY f.created_at DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (AbuseFlag, error) {
		var f AbuseFlag
		return f, r.Scan(&f.ID, &f.App, &f.Email, &f.Kind, &f.Detail, &f.Action, &f.CreatedAt)
	})
}

// ResolveAbuseFlag closes a flag.
func (s *Service) ResolveAbuseFlag(ctx context.Context, admin *auth.Admin, id int64) error {
	if err := requireAdmin(admin); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, `UPDATE abuse_flags SET resolved_at=now() WHERE id=$1`, id)
	return err
}

// NotifyAdmins emails every admin.
func (s *Service) NotifyAdmins(ctx context.Context, subject, body string) {
	rows, err := s.DB.Query(ctx, `SELECT email FROM admins`)
	if err != nil {
		s.Log.Error("list admins", "err", err)
		return
	}
	emails, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return
	}
	for _, e := range emails {
		s.mail(ctx, e, "admin-alert", map[string]any{"Subject": subject, "Body": body})
	}
}
