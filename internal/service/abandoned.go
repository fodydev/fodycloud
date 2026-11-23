// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

// abandonCandidate is a running or sleeping app and its last activity.
type abandonCandidate struct {
	id, userID    int64
	name, email   string
	lastActivity  time.Time
	abandonWarned *time.Time
}

// CheckAbandoned applies the abandoned-app rule of each plan
// (Plan.AbandonAfterDays > 0): an app without visits or deployments for that
// long gets a warning email; if it stays unused for AbandonGrace after the
// warning, it is stopped (never deleted) and its owner is told. Activity
// after the warning cancels it. It returns how many apps were warned and
// stopped.
func (s *Service) CheckAbandoned(ctx context.Context) (warned, stopped int, err error) {
	rows, err := s.DB.Query(ctx, `SELECT a.id, a.user_id, a.name, u.email,
			greatest(a.created_at, coalesce(a.last_request_at, a.created_at),
				coalesce((SELECT max(d.created_at) FROM deployments d WHERE d.app_id=a.id AND d.kind <> 'detect'), a.created_at)),
			a.abandon_warned_at
		FROM apps a JOIN users u ON u.id=a.user_id
		WHERE a.status IN ('running', 'sleeping') AND u.banned_at IS NULL`)
	if err != nil {
		return 0, 0, err
	}
	apps, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (abandonCandidate, error) {
		var c abandonCandidate
		return c, r.Scan(&c.id, &c.userID, &c.name, &c.email, &c.lastActivity, &c.abandonWarned)
	})
	if err != nil {
		return 0, 0, err
	}
	now := s.now()
	for _, a := range apps {
		plan, err := s.PlanFor(ctx, a.userID)
		if err != nil {
			return warned, stopped, err
		}
		if plan.AbandonAfterDays <= 0 {
			continue
		}
		after := time.Duration(plan.AbandonAfterDays * 24 * float64(time.Hour))
		data := map[string]any{"App": a.name, "URL": "https://" + s.Deployer.Hostname(a.name), "Plan": plan.Name,
			"Since": a.lastActivity.Format("2 January 2006"), "Days": formatDays(plan.AbandonAfterDays)}
		switch {
		case a.abandonWarned != nil && a.lastActivity.After(*a.abandonWarned):
			// Visited or deployed since the warning: the app is in use again.
			if _, err := s.DB.Exec(ctx, `UPDATE apps SET abandon_warned_at=NULL WHERE id=$1`, a.id); err != nil {
				return warned, stopped, err
			}
		case a.abandonWarned == nil && now.Sub(a.lastActivity) >= after:
			data["StopOn"] = now.Add(s.AbandonGrace).Format("2 January 2006")
			if _, err := s.DB.Exec(ctx, `UPDATE apps SET abandon_warned_at=$2 WHERE id=$1`, a.id, now); err != nil {
				return warned, stopped, err
			}
			s.mail(ctx, a.email, "app-abandon-warning", data)
			s.Log.Info("abandoned app warned", "app", a.name)
			warned++
		case a.abandonWarned != nil && now.Sub(*a.abandonWarned) >= s.AbandonGrace:
			if err := s.stopByName(ctx, a.name); err != nil {
				return warned, stopped, err
			}
			if _, err := s.DB.Exec(ctx, `UPDATE apps SET abandon_warned_at=NULL WHERE id=$1`, a.id); err != nil {
				return warned, stopped, err
			}
			s.mail(ctx, a.email, "app-abandon-stopped", data)
			s.Log.Info("abandoned app stopped", "app", a.name)
			stopped++
		}
	}
	return warned, stopped, nil
}

// AbandonWarning tells when a warned app will be stopped (nil when the app
// isn't warned), for the dashboard banner.
func (s *Service) AbandonWarning(a *App) *time.Time {
	if a.AbandonWarnedAt == nil {
		return nil
	}
	t := a.AbandonWarnedAt.Add(s.AbandonGrace)
	return &t
}

func formatDays(d float64) string {
	if d == math.Trunc(d) {
		return fmt.Sprintf("%d", int(d))
	}
	return fmt.Sprintf("%.1f", d)
}
