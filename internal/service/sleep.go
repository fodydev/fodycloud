// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/plans"
)

// RunningApp is an app the monitor watches.
type RunningApp struct {
	Name              string
	SleepAfterMinutes int
	Plan              plans.Plan
}

// RunningApps lists running apps with their owner's plan.
func (s *Service) RunningApps(ctx context.Context) ([]RunningApp, error) {
	rows, err := s.DB.Query(ctx, `SELECT name, user_id FROM apps WHERE status='running'`)
	if err != nil {
		return nil, err
	}
	type row struct {
		name string
		user int64
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (row, error) {
		var x row
		return x, r.Scan(&x.name, &x.user)
	})
	if err != nil {
		return nil, err
	}
	out := make([]RunningApp, 0, len(list))
	for _, x := range list {
		p, err := s.PlanFor(ctx, x.user)
		if err != nil {
			return nil, err
		}
		out = append(out, RunningApp{Name: x.name, SleepAfterMinutes: p.SleepAfterMinutes, Plan: p})
	}
	return out, nil
}

// SleepApp stops an idle app; it wakes on the next request.
func (s *Service) SleepApp(ctx context.Context, name string) error {
	tag, err := s.DB.Exec(ctx, `UPDATE apps SET status='sleeping', updated_at=now() WHERE name=$1 AND status='running'`, name)
	if err != nil || tag.RowsAffected() == 0 {
		return err
	}
	s.Log.Info("app sleeping (idle)", "app", name)
	return s.Deployer.Stop(ctx, name)
}

// WakeApp starts a sleeping app. It returns the app's status, or ErrNotFound.
func (s *Service) WakeApp(ctx context.Context, name string) (string, error) {
	var status string
	err := s.DB.QueryRow(ctx, `SELECT status FROM apps WHERE name=$1`, name).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status != "sleeping" {
		return status, nil
	}
	if _, err := s.Deployer.Start(ctx, name); err != nil {
		return status, err
	}
	_, err = s.DB.Exec(ctx, `UPDATE apps SET status='running', updated_at=now() WHERE name=$1 AND status='sleeping'`, name)
	s.Log.Info("app woken up", "app", name)
	return "waking", err
}
