// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"strings"
	"testing"
	"time"

	"github.com/fodydev/fodycloud/internal/service"
)

func TestSparkline(t *testing.T) {
	if s := string(sparkline(nil, 100)); !strings.Contains(s, "No data yet") || strings.Contains(s, "polyline") {
		t.Errorf("empty series: %s", s)
	}
	one := string(sparkline([]float64{5}, 0))
	if strings.Count(one, ",") < 2 || strings.Contains(one, "chart-limit") {
		t.Errorf("single point must draw a flat line, no limit: %s", one)
	}
	s := string(sparkline([]float64{0, 50, 25}, 100))
	for _, want := range []string{`class="chart-line"`, `class="chart-limit"`, "0.0,62.0", "150.0,32.0"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
	if strings.Contains(s, "style=") {
		t.Error("inline styles are blocked by the CSP")
	}
	// A value above the limit sets the scale, so it stays inside the chart.
	if over := string(sparkline([]float64{200}, 100)); strings.Contains(over, ",-") {
		t.Errorf("value above the limit drawn outside: %s", over)
	}
}

func TestMetricCharts(t *testing.T) {
	m := &service.Metrics{Step: "minute", CPULimitPercent: 25, MemoryLimitBytes: 256 << 20, Points: []service.MetricPoint{
		{T: time.Now(), CPUPercent: 3.2, MemoryBytes: 12 << 20, Requests: 4},
	}}
	c := metricCharts(m)
	if len(c) != 3 || c[0].Latest != "3.2 %" || c[1].Latest != "12.0 MB" || c[1].Limit != "limit 256.0 MB" || c[2].Title != "Requests per minute" {
		t.Errorf("got %+v", c)
	}
	if humanBytes(512) != "512 B" || humanBytes(1.5*(1<<30)) != "1.5 GB" {
		t.Error("humanBytes")
	}
}
