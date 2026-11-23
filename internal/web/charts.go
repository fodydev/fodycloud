// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/fodydev/fodycloud/internal/service"
)

// metricChart is one small chart of the app page's Metrics card.
type metricChart struct {
	Title  string
	Latest string // last value, formatted
	Limit  string // the plan's limit, formatted ("" = none)
	SVG    template.HTML
}

// Chart size in SVG units; the CSS scales it to the card's width.
const (
	chartW = 300
	chartH = 64
)

// metricCharts turns an app's metrics into the CPU, memory and request charts.
func metricCharts(m *service.Metrics) []metricChart {
	if m == nil {
		return nil
	}
	cpu, mem, req := make([]float64, len(m.Points)), make([]float64, len(m.Points)), make([]float64, len(m.Points))
	for i, p := range m.Points {
		cpu[i], mem[i], req[i] = p.CPUPercent, float64(p.MemoryBytes), float64(p.Requests)
	}
	per := "per minute"
	if m.Step == "hour" {
		per = "per hour"
	}
	last := func(v []float64) float64 {
		if len(v) == 0 {
			return 0
		}
		return v[len(v)-1]
	}
	return []metricChart{
		{Title: "CPU", Latest: fmt.Sprintf("%.1f %%", last(cpu)),
			Limit: fmt.Sprintf("limit %.0f %% (%.2g vCPU)", m.CPULimitPercent, m.CPULimitPercent/100), SVG: sparkline(cpu, m.CPULimitPercent)},
		{Title: "Memory", Latest: humanBytes(last(mem)),
			Limit: "limit " + humanBytes(float64(m.MemoryLimitBytes)), SVG: sparkline(mem, float64(m.MemoryLimitBytes))},
		{Title: "Requests " + per, Latest: fmt.Sprintf("%.0f", last(req)), SVG: sparkline(req, 0)},
	}
}

// sparkline draws values as an area chart. limit (if > 0) is drawn as a
// dashed line and sets the scale, unless a value goes above it. It uses
// only presentation attributes and CSS classes, so the strict CSP allows it.
func sparkline(values []float64, limit float64) template.HTML {
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %d %d" preserveAspectRatio="none" role="img" aria-hidden="true">`, chartW, chartH)
	if len(values) == 0 {
		fmt.Fprintf(&b, `<text class="chart-empty" x="%d" y="%d" text-anchor="middle">No data yet</text></svg>`, chartW/2, chartH/2+4)
		return template.HTML(b.String()) //nolint:gosec // built from numbers only
	}
	top := limit
	for _, v := range values {
		top = max(top, v)
	}
	if top <= 0 {
		top = 1
	}
	y := func(v float64) float64 { return chartH - 2 - (chartH-4)*v/top }
	step := 0.0
	if len(values) > 1 {
		step = float64(chartW) / float64(len(values)-1)
	}
	pts := make([]string, len(values))
	for i, v := range values {
		pts[i] = fmt.Sprintf("%.1f,%.1f", float64(i)*step, y(v))
	}
	if len(values) == 1 { // a single point: draw a flat line
		pts = append(pts, fmt.Sprintf("%d,%.1f", chartW, y(values[0])))
	}
	line := strings.Join(pts, " ")
	fmt.Fprintf(&b, `<polygon class="chart-area" points="0,%d %s %d,%d"/>`, chartH, line, chartW, chartH)
	fmt.Fprintf(&b, `<polyline class="chart-line" points="%s"/>`, line)
	if limit > 0 {
		fmt.Fprintf(&b, `<line class="chart-limit" x1="0" x2="%d" y1="%.1f" y2="%.1f"/>`, chartW, y(limit), y(limit))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) //nolint:gosec // built from numbers only
}

// humanBytes formats a byte count, e.g. 13107200 → "12.5 MB".
func humanBytes(n float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f B", n)
	}
	return fmt.Sprintf("%.1f %s", n, units[i])
}
