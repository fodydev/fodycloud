// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/plans"
)

// PlanInfo describes a user's plan and usage.
type PlanInfo struct {
	Plan         string     `json:"plan"`
	AssignedPlan string     `json:"assigned_plan"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	Expired      bool       `json:"expired"`
	Limits       plans.Plan `json:"limits"`
	AppsUsed     int        `json:"apps_used"`
}

// PlanInfo returns the user's effective plan and usage.
func (s *Service) PlanInfo(ctx context.Context, u *auth.User) (*PlanInfo, error) {
	return s.planInfo(ctx, u.ID)
}

func (s *Service) planInfo(ctx context.Context, userID int64) (*PlanInfo, error) {
	info := &PlanInfo{AssignedPlan: plans.Default}
	err := s.DB.QueryRow(ctx, `SELECT plan, valid_until FROM subscriptions WHERE user_id=$1`, userID).
		Scan(&info.AssignedPlan, &info.ValidUntil)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	info.Plan = plans.Effective(info.AssignedPlan, info.ValidUntil, s.now())
	info.Expired = info.Plan != info.AssignedPlan
	p, ok := s.Plans.Get(info.Plan)
	if !ok { // plan removed from plans.json: fall back to the default
		info.Plan = plans.Default
		p, _ = s.Plans.Get(plans.Default)
	}
	info.Limits = p
	if err := s.DB.QueryRow(ctx, `SELECT count(*) FROM apps WHERE user_id=$1`, userID).Scan(&info.AppsUsed); err != nil {
		return nil, err
	}
	return info, nil
}

// PlanFor returns the plan limits that apply to a user (used by the worker).
func (s *Service) PlanFor(ctx context.Context, userID int64) (plans.Plan, error) {
	info, err := s.planInfo(ctx, userID)
	if err != nil {
		return plans.Plan{}, err
	}
	return info.Limits, nil
}

// SetPlan assigns a plan to a user. Admins only. validUntil == nil means no expiry.
func (s *Service) SetPlan(ctx context.Context, admin *auth.Admin, email, plan string, validUntil *time.Time, note string) (*PlanInfo, error) {
	if err := requireAdmin(admin); err != nil {
		return nil, err
	}
	if _, ok := s.Plans.Get(plan); !ok {
		return nil, invalid("unknown plan %q (available: %s)", plan, strings.Join(s.Plans.Names(), ", "))
	}
	if validUntil != nil && !validUntil.After(s.now()) {
		return nil, invalid("valid_until must be in the future")
	}
	var userID int64
	err := s.DB.QueryRow(ctx, `SELECT id FROM users WHERE email=$1`, strings.ToLower(strings.TrimSpace(email))).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var by *int64 // nil = system / CLI without an admin account
	if admin.ID != 0 {
		by = &admin.ID
	}
	if err := s.writePlan(ctx, userID, plan, validUntil, by, note); err != nil {
		return nil, err
	}
	return s.planInfo(ctx, userID)
}

func (s *Service) writePlan(ctx context.Context, userID int64, plan string, validUntil *time.Time, by *int64, note string) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	old := plans.Default
	err = tx.QueryRow(ctx, `SELECT plan FROM subscriptions WHERE user_id=$1 FOR UPDATE`, userID).Scan(&old)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if plan == plans.Default {
		_, err = tx.Exec(ctx, `DELETE FROM subscriptions WHERE user_id=$1`, userID)
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO subscriptions(user_id, plan, valid_until, assigned_by, note, updated_at)
			VALUES ($1,$2,$3,$4,$5,now())
			ON CONFLICT (user_id) DO UPDATE SET plan=EXCLUDED.plan, valid_until=EXCLUDED.valid_until,
			assigned_by=EXCLUDED.assigned_by, note=EXCLUDED.note, updated_at=now()`, userID, plan, validUntil, by, note)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO plan_changes(user_id, old_plan, new_plan, valid_until, changed_by, note)
		VALUES ($1,$2,$3,$4,$5,$6)`, userID, old, plan, validUntil, by, note); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return s.ApplyPlan(ctx, userID)
}

// ApplyPlan enforces the user's current plan on live apps: limits are updated
// in place, and apps beyond max_apps are stopped (never deleted), newest first.
func (s *Service) ApplyPlan(ctx context.Context, userID int64) error {
	info, err := s.planInfo(ctx, userID)
	if err != nil {
		return err
	}
	rows, err := s.DB.Query(ctx, `SELECT id, name FROM apps WHERE user_id=$1 AND status <> 'stopped' ORDER BY created_at, id`, userID)
	if err != nil {
		return err
	}
	type app struct {
		id   int64
		name string
	}
	apps, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (app, error) {
		var a app
		return a, r.Scan(&a.id, &a.name)
	})
	if err != nil {
		return err
	}
	var errs []error
	for i, a := range apps {
		if i >= info.Limits.MaxApps {
			s.Log.Info("plan limit: stopping app", "app", a.name, "plan", info.Plan)
			if err := s.Deployer.Stop(ctx, a.name); err != nil {
				errs = append(errs, err)
				continue
			}
			if _, err := s.DB.Exec(ctx, `UPDATE apps SET status='stopped', updated_at=now() WHERE id=$1`, a.id); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if err := s.Deployer.ApplyLimits(ctx, a.name, info.Limits); err != nil {
			errs = append(errs, err)
		}
		errs = append(errs, s.applyStoragePlan(ctx, a.id, a.name, info.Limits))
	}
	return errors.Join(errs...)
}

// ExpireSubscriptions moves expired subscriptions back to the default plan.
func (s *Service) ExpireSubscriptions(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `SELECT s.user_id, s.plan, u.email FROM subscriptions s JOIN users u ON u.id=s.user_id
		WHERE s.valid_until IS NOT NULL AND s.valid_until <= $1`, s.now())
	if err != nil {
		return 0, err
	}
	type expired struct {
		id          int64
		plan, email string
	}
	subs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (expired, error) {
		var e expired
		return e, r.Scan(&e.id, &e.plan, &e.email)
	})
	if err != nil {
		return 0, err
	}
	for _, e := range subs {
		if err := s.writePlan(ctx, e.id, plans.Default, nil, nil, "subscription expired"); err != nil {
			return 0, err
		}
		s.mail(ctx, e.email, "plan-expired", map[string]any{"Plan": e.plan})
		s.Log.Info("subscription expired", "user_id", e.id)
	}
	return len(subs), nil
}

// SendReminders emails users whose plan ends within 7 days (and again within
// 1 day). Each reminder is sent once per subscription period.
func (s *Service) SendReminders(ctx context.Context) (int, error) {
	rows, err := s.DB.Query(ctx, `SELECT s.user_id, u.email, s.plan, s.valid_until FROM subscriptions s JOIN users u ON u.id=s.user_id
		WHERE s.valid_until IS NOT NULL AND s.valid_until > $1 AND s.valid_until <= $1 + interval '7 days' AND u.banned_at IS NULL`, s.now())
	if err != nil {
		return 0, err
	}
	type sub struct {
		id          int64
		email, plan string
		until       time.Time
	}
	subs, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (sub, error) {
		var x sub
		return x, r.Scan(&x.id, &x.email, &x.plan, &x.until)
	})
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, x := range subs {
		kind, when := "7d", "in less than a week"
		if x.until.Sub(s.now()) <= 24*time.Hour {
			kind, when = "1d", "within 24 hours"
		}
		tag, err := s.DB.Exec(ctx, `INSERT INTO reminders_sent(user_id, kind, valid_until) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			x.id, kind, x.until)
		if err != nil {
			return sent, err
		}
		if tag.RowsAffected() == 1 {
			s.mail(ctx, x.email, "plan-expiring", map[string]any{"Plan": x.plan, "When": when, "Until": x.until.Format("2006-01-02")})
			sent++
		}
	}
	return sent, nil
}

// UserSummary is a student, as shown on the admin page.
type UserSummary struct {
	Email      string     `json:"email"`
	Plan       string     `json:"plan"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	Apps       int        `json:"apps"`
	CreatedAt  time.Time  `json:"created_at"`
	Banned     bool       `json:"banned"`
	Verified   bool       `json:"email_verified"`
}

// ListUsers lists all users (admins only).
func (s *Service) ListUsers(ctx context.Context, admin *auth.Admin) ([]UserSummary, error) {
	if err := requireAdmin(admin); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT u.email, COALESCE(s.plan,'free'), s.valid_until,
		(SELECT count(*) FROM apps a WHERE a.user_id=u.id), u.created_at, u.banned_at IS NOT NULL, u.email_verified_at IS NOT NULL
		FROM users u LEFT JOIN subscriptions s ON s.user_id=u.id ORDER BY u.created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (UserSummary, error) {
		var x UserSummary
		err := r.Scan(&x.Email, &x.Plan, &x.ValidUntil, &x.Apps, &x.CreatedAt, &x.Banned, &x.Verified)
		if err == nil {
			x.Plan = plans.Effective(x.Plan, x.ValidUntil, s.now())
		}
		return x, err
	})
}
