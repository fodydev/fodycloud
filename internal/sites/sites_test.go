// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package sites

import "testing"

func TestOf(t *testing.T) {
	s := For("FodyCloud.example")
	cases := map[string]Site{
		"fodycloud.example": Home, "fodycloud.example:443": Home, "DASHBOARD.fodycloud.example": Dashboard,
		"admin.fodycloud.example": Admin, "docs.fodycloud.example": Unknown, "api.fodycloud.example:8443": API, "mcp.fodycloud.example.": MCP,
		"blog.apps.fodycloud.example": App, "a.b.apps.fodycloud.example": Unknown, "apps.fodycloud.example": Unknown,
		"evil.com": Unknown, "203.0.113.10": Unknown, "dashboard.fodycloud.example.evil.com": Unknown,
	}
	for host, want := range cases {
		if got := s.Of(host); got != want {
			t.Errorf("Of(%q) = %v, want %v", host, got, want)
		}
	}
	if got := s.URL(Docs, "/user/stacks"); got != "https://fodycloud.example/docs/user/stacks" {
		t.Errorf("URL: %s", got)
	}
	if got := s.URL(Docs, "/"); got != "https://fodycloud.example/docs" {
		t.Errorf("URL docs home: %s", got)
	}
	if got := s.URL(Home, ""); got != "https://fodycloud.example/" {
		t.Errorf("URL home: %s", got)
	}
}
