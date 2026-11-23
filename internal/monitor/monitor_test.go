// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package monitor

import (
	"strings"
	"testing"
)

func TestParseServiceRequests(t *testing.T) {
	in := `# HELP traefik_service_requests_total How many HTTP requests processed on a service.
# TYPE traefik_service_requests_total counter
traefik_service_requests_total{code="200",method="GET",protocol="http",service="app-blog@docker"} 12
traefik_service_requests_total{code="404",method="GET",protocol="http",service="app-blog@docker"} 3
traefik_service_requests_total{code="200",method="GET",protocol="http",service="control-plane@docker"} 40
traefik_service_request_duration_seconds_count{service="app-blog@docker"} 99
`
	got, err := ParseServiceRequests(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got["app-blog@docker"] != 15 || got["control-plane@docker"] != 40 || len(got) != 2 {
		t.Fatalf("got %v", got)
	}
}

func TestParseStats(t *testing.T) {
	got := ParseStats("fc-app-my-app-12 23.50% 120MiB / 256MiB\nfc-app-my-app-13 91.00% 12.5MiB / 256MiB\n" +
		"fodycloud-traefik-1 3.00% 40MiB / 7.6GiB\nfc-app-x-y 5% 1MiB / 1GiB\nfc-app-old-4 1.00%\n")
	want := map[string]AppStats{"my-app": {CPUPercent: 91, MemoryBytes: 120 << 20}, "old": {CPUPercent: 1}}
	if len(got) != len(want) || got["my-app"] != want["my-app"] || got["old"] != want["old"] {
		t.Fatalf("got %v", got)
	}
}

func TestParseMemUsage(t *testing.T) {
	cases := map[string]int64{"0B": 0, "512B": 512, "980kB": 980000, "12.5MiB": 13107200, "1.5GiB": 1610612736, "bogus": 0, "1.2.3MiB": 0}
	for in, want := range cases {
		if got := ParseMemUsage(in); got != want {
			t.Errorf("ParseMemUsage(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRequestDelta(t *testing.T) {
	if RequestDelta(10, 15) != 5 || RequestDelta(10, 10) != 0 {
		t.Error("normal growth")
	}
	if RequestDelta(100, 3) != 3 { // Traefik restarted: the counter started again
		t.Error("counter reset")
	}
}

func TestAppOfContainer(t *testing.T) {
	cases := map[string]string{"fc-app-blog-3": "blog", "fc-app-my-api-2-41": "my-api-2"}
	for in, want := range cases {
		if got, ok := AppOfContainer(in); !ok || got != want {
			t.Errorf("AppOfContainer(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"traefik", "fc-app-", "fc-app-blog", "fc-app-blog-x"} {
		if _, ok := AppOfContainer(in); ok {
			t.Errorf("AppOfContainer(%q) accepted", in)
		}
	}
}
