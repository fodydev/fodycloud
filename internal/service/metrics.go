// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fodydev/fodycloud/internal/auth"
)

// Metrics retention: minute samples, and hourly roll-ups of them.
const (
	MinuteMetricsKept = 7 * 24 * time.Hour
	HourMetricsKept   = 30 * 24 * time.Hour
)

// MetricSample is one minute of an app, measured by the monitor.
type MetricSample struct {
	App         string
	CPUPercent  float64 // of one CPU: 100 = one full core
	MemoryBytes int64
	Requests    int // requests during the minute
}

// RecordMetrics stores one minute of samples. An app that received requests
// is active: its last visit is updated and an abandoned-app warning is
// cleared.
func (s *Service) RecordMetrics(ctx context.Context, at time.Time, samples []MetricSample) error {
	minute := at.UTC().Truncate(time.Minute)
	batch := &pgx.Batch{}
	for _, m := range samples {
		batch.Queue(`INSERT INTO app_metrics(app_id, resolution, ts, cpu_percent, memory_bytes, requests)
			SELECT id, 'minute', $2, $3, $4, $5 FROM apps WHERE name=$1
			ON CONFLICT (app_id, resolution, ts) DO UPDATE SET cpu_percent=EXCLUDED.cpu_percent,
				memory_bytes=EXCLUDED.memory_bytes, requests=app_metrics.requests+EXCLUDED.requests`,
			m.App, minute, m.CPUPercent, m.MemoryBytes, m.Requests)
		if m.Requests > 0 {
			batch.Queue(`UPDATE apps SET last_request_at=$2, abandon_warned_at=NULL WHERE name=$1`, m.App, at)
		}
	}
	return s.DB.SendBatch(ctx, batch).Close()
}

// RollupMetrics aggregates the last complete hours of minute samples into
// hourly rows, then deletes samples older than their retention.
func (s *Service) RollupMetrics(ctx context.Context, now time.Time) error {
	hour := now.UTC().Truncate(time.Hour)
	if _, err := s.DB.Exec(ctx, `INSERT INTO app_metrics(app_id, resolution, ts, cpu_percent, memory_bytes, requests)
		SELECT app_id, 'hour', date_trunc('hour', ts), avg(cpu_percent), avg(memory_bytes)::bigint, sum(requests)
		FROM app_metrics WHERE resolution='minute' AND ts >= $1 AND ts < $2
		GROUP BY app_id, date_trunc('hour', ts)
		ON CONFLICT (app_id, resolution, ts) DO UPDATE SET cpu_percent=EXCLUDED.cpu_percent,
			memory_bytes=EXCLUDED.memory_bytes, requests=EXCLUDED.requests`,
		hour.Add(-3*time.Hour), hour); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, `DELETE FROM app_metrics
		WHERE (resolution='minute' AND ts < $1) OR (resolution='hour' AND ts < $2)`,
		now.Add(-MinuteMetricsKept), now.Add(-HourMetricsKept))
	return err
}

// MetricPoint is one point of a metrics series.
type MetricPoint struct {
	T           time.Time `json:"t"`
	CPUPercent  float64   `json:"cpu_percent"`
	MemoryBytes int64     `json:"memory_bytes"`
	Requests    int       `json:"requests"`
}

// Metrics is an app's series for a time range, with its plan's limits.
type Metrics struct {
	Range            string        `json:"range"` // 24h | 7d | 30d
	Step             string        `json:"step"`  // minute | hour
	CPULimitPercent  float64       `json:"cpu_limit_percent"`
	MemoryLimitBytes int64         `json:"memory_limit_bytes"`
	Points           []MetricPoint `json:"points"`
}

// MetricRanges are the time ranges AppMetrics accepts.
var MetricRanges = []string{"24h", "7d", "30d"}

// AppMetrics returns an app's metrics: per minute over 24 hours, or per hour
// over 7 or 30 days (computed from minute samples while they are kept).
func (s *Service) AppMetrics(ctx context.Context, u *auth.User, name, rng string) (*Metrics, error) {
	a, err := s.GetApp(ctx, u, name)
	if err != nil {
		return nil, err
	}
	if rng == "" {
		rng = "24h"
	}
	now := s.now()
	var q string
	var since time.Time
	m := &Metrics{Range: rng, Step: "hour", Points: []MetricPoint{}}
	switch rng {
	case "24h":
		m.Step, since = "minute", now.Add(-24*time.Hour)
		q = `SELECT ts, cpu_percent, memory_bytes, requests FROM app_metrics
			WHERE app_id=$1 AND resolution='minute' AND ts >= $2 ORDER BY ts`
	case "7d":
		since = now.Add(-7 * 24 * time.Hour)
		q = `SELECT date_trunc('hour', ts), avg(cpu_percent), avg(memory_bytes)::bigint, sum(requests)::int FROM app_metrics
			WHERE app_id=$1 AND resolution='minute' AND ts >= $2 GROUP BY 1 ORDER BY 1`
	case "30d":
		since = now.Add(-30 * 24 * time.Hour)
		q = `SELECT ts, cpu_percent, memory_bytes, requests FROM app_metrics
			WHERE app_id=$1 AND resolution='hour' AND ts >= $2 ORDER BY ts`
	default:
		return nil, invalid("unknown range %q (available: 24h, 7d, 30d)", rng)
	}
	rows, err := s.DB.Query(ctx, q, a.ID, since)
	if err != nil {
		return nil, err
	}
	pts, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (MetricPoint, error) {
		var p MetricPoint
		var cpu float64
		err := r.Scan(&p.T, &cpu, &p.MemoryBytes, &p.Requests)
		p.CPUPercent = cpu
		return p, err
	})
	if err != nil {
		return nil, fmt.Errorf("metrics: %w", err)
	}
	m.Points = append(m.Points, pts...)
	plan, err := s.PlanFor(ctx, a.UserID)
	if err != nil {
		return nil, err
	}
	m.CPULimitPercent = plan.CPUs * 100
	m.MemoryLimitBytes, _ = plan.MemoryBytes()
	return m, nil
}
