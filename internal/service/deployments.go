// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
)

// Deployment is the public view of a deployment.
type Deployment struct {
	ID         int64      `json:"id"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	// DurationSeconds is how long the deployment ran, once it is finished.
	DurationSeconds *int64 `json:"duration_seconds,omitempty"`
	image           string
}

// FormatDuration writes a duration for people: 9s, 1m 05s, 1h 02m.
func FormatDuration(d time.Duration) string {
	s := int64(d.Round(time.Second) / time.Second)
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm %02ds", s/60, s%60)
	default:
		return fmt.Sprintf("%dh %02dm", s/3600, s%3600/60)
	}
}

// canBuild gates everything that starts a build: write scope, confirmed
// email and the per-user build rate.
func (s *Service) canBuild(u *auth.User) error {
	if err := requireWrite(u); err != nil {
		return err
	}
	if s.RequireVerification && !u.EmailVerified {
		return fmt.Errorf("%w: confirm your email address first (check your inbox, or resend the link from the dashboard)", ErrForbidden)
	}
	if s.BuildLimiter != nil {
		if ok, wait := s.BuildLimiter.Allow(fmt.Sprint(u.ID)); !ok {
			return &QuotaError{Msg: fmt.Sprintf("too many builds, try again in %s", wait.Round(time.Second))}
		}
	}
	return nil
}

func (s *Service) enqueue(ctx context.Context, appID int64, kind, image string) (*Deployment, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	// A newer request supersedes deployments that have not started yet.
	if _, err := tx.Exec(ctx, `UPDATE deployments SET status='cancelled', finished_at=now()
		WHERE app_id=$1 AND status='queued'`, appID); err != nil {
		return nil, err
	}
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO deployments(app_id, kind, image) VALUES ($1,$2,$3) RETURNING id`,
		appID, kind, image).Scan(&id); err != nil {
		return nil, err
	}
	next := "building"
	if kind == "detect" {
		next = "detecting"
	}
	if _, err := tx.Exec(ctx, `UPDATE apps SET status = CASE WHEN status IN ('failed','review','detecting') THEN $2 ELSE status END,
		updated_at=now() WHERE id=$1`, appID, next); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return s.deployment(ctx, appID, id)
}

// Deploy queues a fresh build of the branch head.
func (s *Service) Deploy(ctx context.Context, u *auth.User, name string) (*Deployment, error) {
	if err := s.canBuild(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if err := s.checkRepoAllowed(ctx, a); err != nil {
		return nil, err
	}
	if !a.StackConfirmed {
		return nil, invalid("review and confirm the detected stack first")
	}
	return s.enqueue(ctx, a.ID, "build", "")
}

// Restart redeploys the current image with the current configuration
// (environment, domains, plan limits) without rebuilding.
func (s *Service) Restart(ctx context.Context, u *auth.User, name string) (*Deployment, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if a.Deployment == nil || a.Deployment.image == "" {
		return s.enqueue(ctx, a.ID, "build", "")
	}
	return s.enqueue(ctx, a.ID, "restart", a.Deployment.image)
}

// Rollback redeploys the image of the previous successful deployment.
func (s *Service) Rollback(ctx context.Context, u *auth.User, name string) (*Deployment, error) {
	if err := requireWrite(u); err != nil {
		return nil, err
	}
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	var image string
	err = s.DB.QueryRow(ctx, `SELECT image FROM deployments WHERE app_id=$1 AND image<>'' AND finished_at IS NOT NULL
		AND status IN ('running','superseded') AND id <> COALESCE($2, 0) AND image <> COALESCE((SELECT image FROM deployments WHERE id=$2), '')
		ORDER BY id DESC LIMIT 1`, a.ID, a.currentDepl).Scan(&image)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, invalid("there is no previous successful deployment to roll back to")
	}
	if err != nil {
		return nil, err
	}
	return s.enqueue(ctx, a.ID, "rollback", image)
}

// ListDeployments lists the latest deployments of an app.
func (s *Service) ListDeployments(ctx context.Context, u *auth.User, name string, limit int) ([]*Deployment, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.DB.Query(ctx, `SELECT `+deplColumns+` FROM deployments WHERE app_id=$1 ORDER BY id DESC LIMIT $2`, a.ID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const deplColumns = `id, kind, status, commit_sha, error, image, created_at, started_at, finished_at`

func scanDeployment(row pgx.Row) (*Deployment, error) {
	var d Deployment
	err := row.Scan(&d.ID, &d.Kind, &d.Status, &d.CommitSHA, &d.Error, &d.image, &d.CreatedAt, &d.StartedAt, &d.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if d.StartedAt != nil && d.FinishedAt != nil {
		secs := int64(d.FinishedAt.Sub(*d.StartedAt).Round(time.Second) / time.Second)
		d.DurationSeconds = &secs
	}
	return &d, err
}

func (s *Service) deployment(ctx context.Context, appID, id int64) (*Deployment, error) {
	return scanDeployment(s.DB.QueryRow(ctx, `SELECT `+deplColumns+` FROM deployments WHERE app_id=$1 AND id=$2`, appID, id))
}

// GetDeployment returns one deployment of the user's app.
func (s *Service) GetDeployment(ctx context.Context, u *auth.User, name string, id int64) (*Deployment, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	return s.deployment(ctx, a.ID, id)
}
