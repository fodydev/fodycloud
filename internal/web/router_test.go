// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fodydev/fodycloud/internal/service"
)

func testRoot(t *testing.T) http.Handler {
	t.Helper()
	w := &Web{Svc: &service.Service{}, Log: slog.Default(), BaseDomain: "fc.test", SecureCookies: true}
	if err := w.Init(); err != nil {
		t.Fatal(err)
	}
	stub := func(name string) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.Write([]byte(name + " " + r.URL.Path)) }) //nolint:errcheck
	}
	return w.Root(stub("API"), stub("MCP"))
}

func TestSiteDispatch(t *testing.T) {
	root := testRoot(t)
	cases := []struct {
		method, host, target string
		cookie               *http.Cookie
		code                 int
		want                 string // Location for redirects, else a body substring
	}{
		// home
		{"GET", "fc.test", "/", nil, 200, "Your code. Your deployment. Your cloud."},
		{"GET", "fc.test", "/", &http.Cookie{Name: hintCookie, Value: "1"}, 303, "https://dashboard.fc.test/apps"},
		{"GET", "fc.test", "/login", nil, 404, "Page not found"}, // sign-in lives on the dashboard only
		{"GET", "fc.test", "/nope", nil, 404, "Page not found"},
		// dashboard
		{"GET", "dashboard.fc.test", "/", nil, 303, "/login"},
		{"GET", "dashboard.fc.test", "/apps", nil, 303, "/login"},
		{"GET", "dashboard.fc.test", "/login", nil, 200, "Log in to FodyCloud"},
		{"GET", "dashboard.fc.test", "/nope", nil, 404, "Page not found"},
		{"GET", "dashboard.fc.test", "/docs", nil, 303, "https://fc.test/docs"},
		{"GET", "dashboard.fc.test", "/docs/user/stacks", nil, 303, "https://fc.test/docs/user/stacks"},
		{"GET", "dashboard.fc.test", "/admin", nil, 404, "Page not found"}, // the admin panel is on admin.<domain>
		{"POST", "dashboard.fc.test", "/mcp", nil, 404, ""},
		// docs: public on the home site, signed-in users read them in the dashboard
		{"GET", "fc.test", "/docs", nil, 200, "User guide"},
		{"GET", "fc.test", "/docs/", nil, 200, "User guide"},
		{"GET", "fc.test", "/docs/user/stacks", nil, 200, "Choose your stack"},
		{"GET", "fc.test", "/docs/user/stacks", &http.Cookie{Name: hintCookie, Value: "1"}, 303, "https://dashboard.fc.test/docs/user/stacks"},
		{"GET", "fc.test", "/docs/admin/overview", nil, 404, "Page not found"},
		{"GET", "fc.test", "/docs/developer/nope", nil, 404, "Page not found"},
		{"GET", "docs.fc.test", "/", nil, 404, "unknown host"},
		// legal policies: public, on the home site
		{"GET", "fc.test", "/legal/terms", nil, 200, "Terms of service"},
		{"GET", "fc.test", "/legal/privacy", nil, 200, "Privacy policy"},
		{"GET", "fc.test", "/legal/nope", nil, 404, "Page not found"},
		// api and mcp
		{"GET", "api.fc.test", "/", nil, 200, "FodyCloud API"},
		{"GET", "api.fc.test", "/v1/me", nil, 200, "API /v1/me"},
		{"GET", "api.fc.test", "/nope", nil, 404, `{"error":"not found"}`},
		{"GET", "mcp.fc.test", "/", nil, 200, "https://mcp.fc.test/mcp"},
		{"POST", "mcp.fc.test", "/mcp", nil, 200, "MCP /mcp"},
		{"GET", "dashboard.fc.test", "/v1/me", nil, 404, ""},
		// admin: everything but /login needs an admin session
		{"GET", "admin.fc.test", "/", nil, 200, "Sign in to the admin panel"},
		{"GET", "admin.fc.test", "/docs/plans", nil, 200, "Sign in to the admin panel"},
		{"POST", "admin.fc.test", "/plan", nil, 303, "/login"},
		// apps: wake page assets are served, not the wake page
		{"GET", "blog.apps.fc.test", "/static/app.css", nil, 200, ".topbar"},
		{"GET", "fc.test", "/static/app.js", nil, 200, "htmx:beforeSwap"},
		// unknown hosts
		{"GET", "203.0.113.10", "/", nil, 404, "unknown host"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, "https://"+c.host+c.target, nil)
		if c.cookie != nil {
			r.AddCookie(c.cookie)
		}
		rec := httptest.NewRecorder()
		root.ServeHTTP(rec, r)
		name := c.method + " " + c.host + c.target
		if rec.Code != c.code {
			t.Errorf("%s: status %d, want %d", name, rec.Code, c.code)
			continue
		}
		if c.code >= 300 && c.code < 400 {
			if got := rec.Header().Get("Location"); got != c.want {
				t.Errorf("%s: Location %q, want %q", name, got, c.want)
			}
		} else if !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: body lacks %q", name, c.want)
		}
	}
}

func TestStaticContentTypeOnAppHosts(t *testing.T) {
	rec := httptest.NewRecorder()
	testRoot(t).ServeHTTP(rec, httptest.NewRequest("GET", "https://unknown.apps.fc.test/static/brand/emblem.svg", nil))
	if ct := rec.Header().Get("Content-Type"); rec.Code != 200 || !strings.HasPrefix(ct, "image/svg+xml") {
		t.Errorf("emblem: %d %q", rec.Code, ct)
	}
}

func TestCookieNames(t *testing.T) {
	w := &Web{SecureCookies: true}
	if w.cookieName(dashboardSession) != "__Host-fc_session" || w.cookieName(adminSession) != "__Host-fc_admin" {
		t.Error("secure cookies need the __Host- prefix")
	}
	w.SecureCookies = false
	if w.cookieName(dashboardSession) != "fc_session" {
		t.Error("plain cookie name")
	}
}

func TestLinksBetweenSites(t *testing.T) {
	root := testRoot(t)
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, httptest.NewRequest("GET", "https://fc.test/", nil))
	body := rec.Body.String()
	for _, want := range []string{`href="https://dashboard.fc.test/login"`, `href="https://fc.test/docs"`, `href="https://api.fc.test/"`, `href="https://fc.test/docs/user/getting-started"`} {
		if !strings.Contains(body, want) {
			t.Errorf("home page lacks %s", want)
		}
	}
}

// Sign-up requires accepting the terms; the check happens before any account
// is created (so no database is needed here).
func TestSignupRequiresTerms(t *testing.T) {
	root := testRoot(t)
	r := httptest.NewRequest("POST", "https://dashboard.fc.test/signup", strings.NewReader("email=a%40b.cm&password=long-enough-pw"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://dashboard.fc.test")
	rec := httptest.NewRecorder()
	root.ServeHTTP(rec, r)
	if rec.Code != 303 || !strings.Contains(rec.Header().Get("Location"), "Accept+the+Terms") {
		t.Errorf("got %d %s", rec.Code, rec.Header().Get("Location"))
	}
	page := httptest.NewRecorder()
	root.ServeHTTP(page, httptest.NewRequest("GET", "https://dashboard.fc.test/signup", nil))
	if !strings.Contains(page.Body.String(), `name="accept_terms"`) || !strings.Contains(page.Body.String(), `href="https://fc.test/legal/terms"`) {
		t.Error("sign-up form lacks the terms checkbox or links")
	}
}
