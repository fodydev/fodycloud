// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package monitor watches running apps once a minute: it records their
// metrics (CPU, memory, requests), flags sustained CPU abuse, puts idle apps
// to sleep (they wake on the next request) and applies the abandoned-app rule.
package monitor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/internal/dockercli"
	"github.com/fodydev/fodycloud/internal/service"
)

// Monitor samples app containers.
type Monitor struct {
	Svc    *service.Service
	Docker *dockercli.Docker
	Log    *slog.Logger
	// MetricsURL is Traefik's internal Prometheus endpoint.
	MetricsURL string
	// CPUMinutes is how many consecutive samples at >= 90% of the CPU limit
	// raise an abuse flag.
	CPUMinutes int
	Interval   time.Duration
	// AbandonInterval is how often the abandoned-app rule runs (default 1h).
	AbandonInterval time.Duration
	Now             func() time.Time

	high         map[string]int
	idle         map[string]idleState
	prevRequests map[string]float64 // Traefik counter per app at the last tick
	lastRollup   time.Time
	lastAbandon  time.Time
}

// AppStats is one app's resource use from `docker stats`.
type AppStats struct {
	CPUPercent  float64
	MemoryBytes int64
}

type idleState struct {
	requests float64
	since    time.Time
}

// Run samples until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	if m.Interval == 0 {
		m.Interval = time.Minute
	}
	if m.AbandonInterval == 0 {
		m.AbandonInterval = time.Hour
	}
	if m.Now == nil {
		m.Now = time.Now
	}
	m.high, m.idle, m.prevRequests = map[string]int{}, map[string]idleState{}, map[string]float64{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(m.Interval):
		}
		if err := m.Tick(ctx); err != nil {
			m.Log.Error("monitor", "err", err)
		}
	}
}

// Tick takes one sample and runs every rule that depends on it.
func (m *Monitor) Tick(ctx context.Context) error {
	apps, err := m.Svc.RunningApps(ctx)
	if err != nil {
		return err
	}
	m.forgetStopped(apps)
	now := m.Now()
	stats := m.stats(ctx)
	requests := m.requests(ctx)
	m.checkCPU(ctx, apps, stats)
	m.checkIdle(ctx, apps, requests, now)
	m.recordMetrics(ctx, apps, stats, requests, now)
	if now.Sub(m.lastRollup) >= time.Hour {
		m.lastRollup = now
		if err := m.Svc.RollupMetrics(ctx, now); err != nil {
			m.Log.Error("metrics rollup", "err", err)
		}
	}
	if now.Sub(m.lastAbandon) >= m.AbandonInterval {
		m.lastAbandon = now
		if warned, stopped, err := m.Svc.CheckAbandoned(ctx); err != nil {
			m.Log.Error("abandoned apps", "err", err)
		} else if warned+stopped > 0 {
			m.Log.Info("abandoned apps", "warned", warned, "stopped", stopped)
		}
	}
	return nil
}

// forgetStopped drops the per-app state of apps that no longer run.
func (m *Monitor) forgetStopped(apps []service.RunningApp) {
	seen := map[string]bool{}
	for _, a := range apps {
		seen[a.Name] = true
	}
	for name := range m.high {
		if !seen[name] {
			delete(m.high, name)
		}
	}
	for name := range m.idle {
		if !seen[name] {
			delete(m.idle, name)
		}
	}
}

// stats returns each running app's CPU and memory (nil on error).
func (m *Monitor) stats(ctx context.Context) map[string]AppStats {
	out, err := m.Docker.Run(ctx, "stats", "--no-stream", "--format", "{{.Name}} {{.CPUPerc}} {{.MemUsage}}")
	if err != nil {
		m.Log.Error("docker stats", "err", err)
		return nil
	}
	return ParseStats(out)
}

// requests returns Traefik's request counter per app service (nil when
// metrics are disabled or unreachable).
func (m *Monitor) requests(ctx context.Context) map[string]float64 {
	if m.MetricsURL == "" {
		return nil
	}
	counts, err := m.scrape(ctx)
	if err != nil {
		m.Log.Error("traefik metrics", "err", err)
		return nil
	}
	return counts
}

func serviceOf(app string) string { return "app-" + app + "@docker" }

func (m *Monitor) checkCPU(ctx context.Context, apps []service.RunningApp, stats map[string]AppStats) {
	if stats == nil {
		return
	}
	for _, a := range apps {
		st, ok := stats[a.Name]
		limit := a.Plan.CPUs * 100
		if !ok || limit <= 0 || st.CPUPercent < 0.9*limit {
			m.high[a.Name] = 0
			continue
		}
		m.high[a.Name]++
		if m.high[a.Name] >= max(1, m.CPUMinutes) {
			m.high[a.Name] = 0
			detail := fmt.Sprintf("CPU at %.0f%% (limit %.0f%%) for %d consecutive minutes", st.CPUPercent, limit, max(1, m.CPUMinutes))
			if err := m.Svc.FlagAbuse(ctx, a.Name, "cpu", detail); err != nil {
				m.Log.Error("flag abuse", "app", a.Name, "err", err)
			}
		}
	}
}

func (m *Monitor) checkIdle(ctx context.Context, apps []service.RunningApp, counts map[string]float64, now time.Time) {
	if counts == nil {
		return
	}
	for _, a := range apps {
		if a.SleepAfterMinutes <= 0 {
			continue
		}
		c := counts[serviceOf(a.Name)]
		st, ok := m.idle[a.Name]
		if !ok || c != st.requests {
			m.idle[a.Name] = idleState{requests: c, since: now}
			continue
		}
		if now.Sub(st.since) >= time.Duration(a.SleepAfterMinutes)*time.Minute {
			delete(m.idle, a.Name)
			if err := m.Svc.SleepApp(ctx, a.Name); err != nil {
				m.Log.Error("sleep app", "app", a.Name, "err", err)
			}
		}
	}
}

// recordMetrics stores one minute per running app. Requests are the growth
// of Traefik's counter since the previous tick.
func (m *Monitor) recordMetrics(ctx context.Context, apps []service.RunningApp, stats map[string]AppStats, counts map[string]float64, now time.Time) {
	samples := make([]service.MetricSample, 0, len(apps))
	for _, a := range apps {
		st := stats[a.Name]
		reqs := 0
		if counts != nil {
			c := counts[serviceOf(a.Name)]
			if prev, ok := m.prevRequests[a.Name]; ok {
				reqs = RequestDelta(prev, c)
			}
			m.prevRequests[a.Name] = c
		}
		samples = append(samples, service.MetricSample{App: a.Name, CPUPercent: st.CPUPercent, MemoryBytes: st.MemoryBytes, Requests: reqs})
	}
	if len(samples) == 0 {
		return
	}
	if err := m.Svc.RecordMetrics(ctx, now, samples); err != nil {
		m.Log.Error("record metrics", "err", err)
	}
}

// RequestDelta is the number of requests between two readings of a
// counter. A counter that went down was reset (Traefik restarted): the new
// value is all we know.
func RequestDelta(prev, cur float64) int {
	if cur < prev {
		return int(cur)
	}
	return int(cur - prev)
}

func (m *Monitor) scrape(ctx context.Context) (map[string]float64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.MetricsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics: HTTP %d", resp.StatusCode)
	}
	return ParseServiceRequests(io.LimitReader(resp.Body, 32<<20))
}

// ParseServiceRequests sums traefik_service_requests_total per service label.
func ParseServiceRequests(r io.Reader) (map[string]float64, error) {
	out := map[string]float64{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	const metric = "traefik_service_requests_total{"
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, metric) {
			continue
		}
		end := strings.LastIndexByte(line, '}')
		if end < 0 {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(line[end+1:]), 64)
		if err != nil {
			continue
		}
		if svc := label(line[len(metric):end], "service"); svc != "" {
			out[svc] += v
		}
	}
	return out, sc.Err()
}

func label(labels, key string) string {
	for _, part := range strings.Split(labels, ",") {
		k, v, ok := strings.Cut(part, "=")
		if ok && strings.TrimSpace(k) == key {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}

// ParseStats maps app name -> CPU and memory from `docker stats` lines
// "<container> <cpu%> <used> / <limit>" (only FodyCloud app containers are
// kept; if an app briefly has two containers during a deploy, the highest
// values win).
func ParseStats(out string) map[string]AppStats {
	res := map[string]AppStats{}
	for _, line := range dockercli.Lines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		app, ok := AppOfContainer(fields[0])
		if !ok {
			continue
		}
		cpu, err := strconv.ParseFloat(strings.TrimSuffix(fields[1], "%"), 64)
		if err != nil {
			continue
		}
		var mem int64
		if len(fields) >= 3 {
			mem = ParseMemUsage(fields[2])
		}
		cur := res[app]
		res[app] = AppStats{CPUPercent: max(cur.CPUPercent, cpu), MemoryBytes: max(cur.MemoryBytes, mem)}
	}
	return res
}

// ParseMemUsage reads Docker's memory figures ("12.5MiB", "1.2GiB", "980kB",
// "0B") as bytes; unreadable values are 0.
func ParseMemUsage(s string) int64 {
	units := []struct {
		suffix string
		mult   float64
	}{{"KiB", 1 << 10}, {"MiB", 1 << 20}, {"GiB", 1 << 30}, {"TiB", 1 << 40}, {"kB", 1e3}, {"MB", 1e6}, {"GB", 1e9}, {"B", 1}}
	for _, u := range units {
		if num, ok := strings.CutSuffix(s, u.suffix); ok {
			v, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0
			}
			return int64(v * u.mult)
		}
	}
	return 0
}

// AppOfContainer extracts the app name from "fc-app-<app>-<deployment>".
func AppOfContainer(name string) (string, bool) {
	rest, ok := strings.CutPrefix(name, "fc-app-")
	if !ok {
		return "", false
	}
	i := strings.LastIndexByte(rest, '-')
	if i <= 0 {
		return "", false
	}
	if _, err := strconv.ParseInt(rest[i+1:], 10, 64); err != nil {
		return "", false
	}
	return rest[:i], true
}
