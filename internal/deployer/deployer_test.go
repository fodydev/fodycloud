// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package deployer

import (
	"slices"
	"strings"
	"testing"

	"github.com/fodydev/fodycloud/internal/plans"
)

func TestLabels(t *testing.T) {
	d := &Deployer{Network: "fodycloud-apps", AppsDomain: "apps.fodycloud.internal", AppPort: 8080,
		AppsCertResolver: "dns", CustomCertResolver: "http"}
	l := d.Labels(Spec{AppName: "blog", UserID: 7, DeploymentID: 42, CustomDomains: []string{"www.ex.cm", "ex.cm"}})
	want := map[string]string{
		"fodycloud.app":                                           "blog",
		"fodycloud.user":                                          "7",
		"fodycloud.deployment":                                    "42",
		"traefik.enable":                                          "true",
		"traefik.docker.network":                                  "fodycloud-apps",
		"traefik.http.routers.app-blog.rule":                      "Host(`blog.apps.fodycloud.internal`)",
		"traefik.http.routers.app-blog.tls.certresolver":          "dns",
		"traefik.http.routers.app-blog-custom.rule":               "Host(`www.ex.cm`) || Host(`ex.cm`)",
		"traefik.http.routers.app-blog-custom.tls.certresolver":   "http",
		"traefik.http.routers.app-blog-custom.service":            "app-blog",
		"traefik.http.services.app-blog.loadbalancer.server.port": "8080",
	}
	for k, v := range want {
		if l[k] != v {
			t.Errorf("label %s = %q, want %q", k, l[k], v)
		}
	}
	// No ACME (local tests): no certresolver labels at all.
	d.AppsCertResolver, d.CustomCertResolver = "", ""
	l = d.Labels(Spec{AppName: "blog"})
	if _, ok := l["traefik.http.routers.app-blog.tls.certresolver"]; ok {
		t.Error("certresolver label set without ACME")
	}
	if _, ok := l["traefik.http.routers.app-blog-custom.rule"]; ok {
		t.Error("custom router set without custom domains")
	}
}

func TestLimitArgs(t *testing.T) {
	p := plans.Plan{CPUs: 0.25, Memory: "256m", Disk: "1G", Pids: 128}
	d := &Deployer{DiskQuota: true}
	run := d.LimitArgs(p, false)
	for _, want := range []string{"0.25", "256m", "128", "size=1G"} {
		if !slices.Contains(run, want) {
			t.Errorf("run args %v missing %s", run, want)
		}
	}
	if slices.Contains(d.LimitArgs(p, true), "size=1G") {
		t.Error("docker update cannot change storage size")
	}
	d.DiskQuota = false
	if slices.Contains(d.LimitArgs(p, false), "--storage-opt") {
		t.Error("storage-opt used without quota support")
	}
}

func TestStateHealthy(t *testing.T) {
	cases := []struct {
		st   state
		want bool
	}{
		{state{Running: true}, true},
		{state{Running: true, RestartCount: 3}, false}, // crash loop caught while briefly running
		{state{Running: true, Restarting: true}, false},
		{state{Running: false, ExitCode: 0}, false},
	}
	for _, c := range cases {
		if got := c.st.Healthy(); got != c.want {
			t.Errorf("%+v.Healthy() = %v, want %v", c.st, got, c.want)
		}
	}
}

func TestParseStatus(t *testing.T) {
	cases := map[string]int{
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain": 200,
		"HTTP/1.0 302 Found":                          302,
		"HTTP/1.1 503 Service Unavailable":            503,
	}
	for in, want := range cases {
		if got, ok := ParseStatus(in); !ok || got != want {
			t.Errorf("ParseStatus(%q) = %d, %v; want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "hello", "HTTP/1.1 abc", "SSH-2.0-OpenSSH", "HTTP/1.1 999 x"} {
		if _, ok := ParseStatus(bad); ok {
			t.Errorf("ParseStatus(%q) accepted", bad)
		}
	}
}

func TestHealthArgsUseOnlyTheMarker(t *testing.T) {
	args := strings.Join(healthArgs(), " ")
	if !strings.Contains(args, "[ -e "+readyMarker+" ]") || !strings.Contains(args, "--health-start-interval 1s") {
		t.Fatalf("health args = %q", args)
	}
}
