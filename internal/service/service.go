// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package service holds every business rule of the control plane. The JSON
// API, the dashboard and the MCP server are thin layers over it, so ownership,
// quotas and repository restrictions are enforced in exactly one place.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fodydev/fodycloud/internal/appsdb"
	"github.com/fodydev/fodycloud/internal/deployer"
	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/ratelimit"
	"github.com/fodydev/fodycloud/internal/repo"
)

// Errors returned by the service; the HTTP layers map them to status codes.
var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
)

// InvalidError is a user-facing validation error (HTTP 400 / 422).
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return &InvalidError{Msg: fmt.Sprintf(format, a...)} }

// QuotaError is returned when a plan limit is reached (HTTP 403).
type QuotaError struct{ Msg string }

func (e *QuotaError) Error() string { return e.Msg }

// Service implements the control plane.
type Service struct {
	DB       *pgxpool.Pool
	Plans    *plans.Catalog
	Git      *repo.Git
	Deployer *deployer.Deployer
	AppsDB   *appsdb.Server // students' PostgreSQL databases (nil = not offered)
	Log      *slog.Logger
	PublicIP string
	Now      func() time.Time

	Mail      Mailer
	PublicURL string // dashboard URL, used in email links
	// RequireVerification blocks app creation and deploys until the user's
	// email is confirmed (enabled when email delivery is configured).
	RequireVerification bool
	// BuildLimiter bounds build requests per user (nil = unlimited).
	BuildLimiter *ratelimit.Limiter
	// AbandonGrace is the delay between the abandoned-app warning and the stop.
	AbandonGrace time.Duration

	hubOnce sync.Once
	logs    *logHub // live build-log followers (logstream.go)
}

// Mailer queues templated email (implemented by internal/mail).
type Mailer interface {
	Template(ctx context.Context, to, name string, data any) error
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// mail queues a templated email; delivery happens in the background
// (internal/mail). It never fails a request: errors are logged.
func (s *Service) mail(ctx context.Context, to, template string, data any) {
	if s.Mail == nil {
		return
	}
	if err := s.Mail.Template(ctx, to, template, data); err != nil {
		s.Log.Error("queue email", "template", template, "err", err)
	}
}
