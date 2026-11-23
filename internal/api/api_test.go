// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package api

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		host, origin, referer string
		want                  bool
	}{
		{"dashboard.x", "https://dashboard.x", "", true},
		{"dashboard.x", "https://dashboard.x:443", "", true},       // explicit default port
		{"dashboard.x:443", "https://dashboard.x", "", true},       // Host with default port
		{"dashboard.x:8443", "https://dashboard.x:8443", "", true}, // non-default port must match
		{"dashboard.x", "https://dashboard.x:8443", "", false},
		{"Dashboard.X", "https://dashboard.x", "", true},        // case-insensitive
		{"dashboard.x", "", "https://dashboard.x/apps/a", true}, // Referer fallback
		{"dashboard.x", "https://evil.example", "", false},
		{"dashboard.x", "https://dashboard.x.evil.example", "", false}, // suffix trick
		{"dashboard.x", "null", "", false},                             // sandboxed iframes send "null"
		{"dashboard.x", "", "", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/login", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.referer != "" {
			r.Header.Set("Referer", c.referer)
		}
		if got := SameOrigin(r); got != c.want {
			t.Errorf("host=%q origin=%q referer=%q: got %v, want %v", c.host, c.origin, c.referer, got, c.want)
		}
	}
}

// Students' and admins' tokens never cross: the prefix is checked before any
// database lookup (so this test needs no database).
func TestTokenKindsDontCross(t *testing.T) {
	a := &API{Log: slog.Default()}
	h := a.Handler()
	cases := []struct{ path, token, want string }{
		{"/v1/apps", "fca_abc", "admin tokens only work on /v1/admin"},
		{"/v1/me", "fca_abc", "admin tokens only work on /v1/admin"},
		{"/v1/admin/users", "fc_abc", "admins only"},
		{"/v1/admin/abuse", "fc_abc", "admins only"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "https://api.fc.test"+c.path, nil)
		r.Header.Set("Authorization", "Bearer "+c.token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s with %s: %d %s", c.path, c.token, rec.Code, rec.Body.String())
		}
	}
}

// GET /v1/about is public: anyone can find the source (AGPL-3.0).
func TestAboutIsPublic(t *testing.T) {
	a := &API{About: About{Name: "FodyCloud", Version: "v1", License: "AGPL-3.0-or-later", Source: "https://fc.test/source"}}
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/v1/about", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"source":"https://fc.test/source"`) || !strings.Contains(rec.Body.String(), `"license":"AGPL-3.0-or-later"`) {
		t.Errorf("about: %d %s", rec.Code, rec.Body.String())
	}
}
