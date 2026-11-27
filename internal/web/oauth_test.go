// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/fodydev/fodycloud/internal/oauth"
	"github.com/fodydev/fodycloud/internal/service"
)

func forgeRoot(t *testing.T, apps map[string]oauth.App) http.Handler {
	t.Helper()
	reg, err := oauth.NewRegistry(apps)
	if err != nil {
		t.Fatal(err)
	}
	w := &Web{Svc: &service.Service{}, Log: slog.Default(), BaseDomain: "fc.test", SecureCookies: true, Forges: reg}
	if err := w.Init(); err != nil {
		t.Fatal(err)
	}
	return w.Root(http.NotFoundHandler(), http.NotFoundHandler())
}

func forgeGet(h http.Handler, host, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest("GET", "https://"+host+target, nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestForgeButtonsOnlyForEnabledProviders(t *testing.T) {
	// None configured: email and password only, and the routes don't exist.
	off := forgeRoot(t, nil)
	for _, page := range []string{"/login", "/signup"} {
		if body := forgeGet(off, "dashboard.fc.test", page).Body.String(); strings.Contains(body, "Continue with") {
			t.Errorf("%s shows forge buttons with no provider configured", page)
		}
	}
	if code := forgeGet(off, "dashboard.fc.test", "/auth/github/start").Code; code != http.StatusNotFound {
		t.Errorf("disabled provider: start = %d, want 404", code)
	}

	on := forgeRoot(t, map[string]oauth.App{
		oauth.GitHub:   {ClientID: "id", ClientSecret: "s"},
		oauth.Codeberg: {ClientID: "id", ClientSecret: "s", Label: "Campus Git", URL: "https://git.example.edu"},
	})
	for _, page := range []string{"/login", "/signup"} {
		body := forgeGet(on, "dashboard.fc.test", page).Body.String()
		if !strings.Contains(body, `href="/auth/github/start">Continue with GitHub`) || !strings.Contains(body, "Continue with Campus Git") {
			t.Errorf("%s misses the enabled forges", page)
		}
		if strings.Contains(body, "Continue with GitLab") {
			t.Errorf("%s shows GitLab, which isn't configured", page)
		}
	}
	// Admins never sign in with a forge.
	if body := forgeGet(on, "admin.fc.test", "/login").Body.String(); strings.Contains(body, "Continue with") {
		t.Error("the admin sign-in page shows forge buttons")
	}
	for _, target := range []string{"/auth/gitlab/start", "/auth/bitbucket/start", "/auth/gitlab/callback?state=x&code=y"} {
		if code := forgeGet(on, "dashboard.fc.test", target).Code; code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", target, code)
		}
	}
	// Only on the dashboard site.
	if code := forgeGet(on, "fc.test", "/auth/github/start").Code; code != http.StatusNotFound {
		t.Errorf("home site: start = %d, want 404", code)
	}
}

// startFlow follows /auth/<provider>/start and returns the flow cookie and
// the forge's authorize URL.
func startFlow(t *testing.T, h http.Handler, provider string) (*http.Cookie, *url.URL) {
	t.Helper()
	rec := forgeGet(h, "dashboard.fc.test", "/auth/"+provider+"/start")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("start = %d", rec.Code)
	}
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var flow *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "__Host-fc_oauth" {
			flow = c
		}
	}
	if flow == nil {
		t.Fatal("no flow cookie")
	}
	if flow.Domain != "" || flow.Path != "/" || !flow.Secure || !flow.HttpOnly || flow.SameSite != http.SameSiteLaxMode {
		t.Fatalf("the flow cookie must be host-only, Secure, HttpOnly and Lax: %+v", flow)
	}
	return flow, loc
}

func TestForgeStart(t *testing.T) {
	h := forgeRoot(t, map[string]oauth.App{oauth.GitHub: {ClientID: "the-client", ClientSecret: "s"}})
	flow, loc := startFlow(t, h, "github")
	q := loc.Query()
	if loc.Host != "github.com" || loc.Path != "/login/oauth/authorize" {
		t.Errorf("authorize URL = %s", loc)
	}
	if q.Get("client_id") != "the-client" || q.Get("redirect_uri") != "https://dashboard.fc.test/auth/github/callback" ||
		q.Get("code_challenge_method") != "S256" || q.Get("state") == "" {
		t.Errorf("authorize query = %v", q)
	}
	// The PKCE verifier stays in the cookie: never in the URL.
	verifier := strings.Split(flow.Value, ".")[3]
	if strings.Contains(loc.String(), verifier) {
		t.Error("the PKCE verifier leaked into the authorize URL")
	}
	if strings.Contains(loc.String(), "client_secret") || strings.Contains(loc.String(), "=s&") {
		t.Error("the client secret leaked into the authorize URL")
	}
}

func TestForgeCallbackRefusesBadState(t *testing.T) {
	h := forgeRoot(t, map[string]oauth.App{
		oauth.GitHub: {ClientID: "id", ClientSecret: "s"}, oauth.GitLab: {ClientID: "id", ClientSecret: "s"},
	})
	flow, loc := startFlow(t, h, "github")
	state := loc.Query().Get("state")
	cases := []struct {
		name, target string
		cookie       *http.Cookie
	}{
		{"no cookie", "/auth/github/callback?code=c&state=" + state, nil},
		{"wrong state", "/auth/github/callback?code=c&state=nope", flow},
		{"no state", "/auth/github/callback?code=c", flow},
		{"other provider", "/auth/gitlab/callback?code=c&state=" + state, flow},
	}
	for _, c := range cases {
		var cookies []*http.Cookie
		if c.cookie != nil {
			cookies = append(cookies, &http.Cookie{Name: c.cookie.Name, Value: c.cookie.Value})
		}
		rec := forgeGet(h, "dashboard.fc.test", c.target, cookies...)
		if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login?error=") ||
			!strings.Contains(rec.Header().Get("Location"), "invalid") {
			t.Errorf("%s: %d %s, want a redirect to /login with an error", c.name, rec.Code, rec.Header().Get("Location"))
		}
	}
	// Positive control: the right state reaches the provider's answer (here
	// the student cancelled on GitHub), and the flow cookie is cleared.
	rec := forgeGet(h, "dashboard.fc.test", "/auth/github/callback?error=access_denied&state="+state,
		&http.Cookie{Name: flow.Name, Value: flow.Value})
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "cancelled") {
		t.Errorf("cancelled sign-in: Location = %s", loc)
	}
	cleared := false
	for _, c := range rec.Result().Cookies() {
		cleared = cleared || (c.Name == flow.Name && c.MaxAge < 0)
	}
	if !cleared {
		t.Error("the flow cookie must be cleared after the callback")
	}
}

func TestForgeConnectNeedsSession(t *testing.T) {
	h := forgeRoot(t, map[string]oauth.App{oauth.GitHub: {ClientID: "id", ClientSecret: "s"}})
	r := httptest.NewRequest("POST", "https://dashboard.fc.test/account/forges/github/connect", nil)
	r.Header.Set("Origin", "https://dashboard.fc.test")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login" {
		t.Errorf("connect without a session = %d %s, want a redirect to /login", rec.Code, rec.Header().Get("Location"))
	}
}
