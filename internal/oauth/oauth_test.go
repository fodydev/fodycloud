// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

// fakeForge serves a token endpoint and the given API responses (path →
// JSON body), and checks the PKCE verifier and the bearer token.
func fakeForge(t *testing.T, api map[string]string) *httptest.Server {
	t.Helper()
	var challenge string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm() //nolint:errcheck
		if r.Form.Get("code") != "the-code" || oauth2.S256ChallengeFromVerifier(r.Form.Get("code_verifier")) != challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"tok","token_type":"bearer"}`)) //nolint:errcheck
	})
	mux.HandleFunc("GET /challenge", func(w http.ResponseWriter, r *http.Request) { challenge = r.URL.Query().Get("c") })
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		body, ok := api[strings.TrimPrefix(r.URL.Path, "/api")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body)) //nolint:errcheck
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// exchange runs the code flow of p against srv.
func exchange(t *testing.T, p *Provider, srv *httptest.Server) (Identity, error) {
	t.Helper()
	p.cfg.Endpoint = oauth2.Endpoint{AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}
	p.apiBase = srv.URL + "/api"
	verifier := GenerateVerifier()
	auth, err := url.Parse(p.AuthURL("https://dashboard.fodycloud.example/auth/x/callback", "st", verifier))
	if err != nil {
		t.Fatal(err)
	}
	q := auth.Query()
	if q.Get("state") != "st" || q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") == "" {
		t.Fatalf("authorize URL misses state, PKCE or redirect: %s", auth)
	}
	if _, err := http.Get(srv.URL + "/challenge?c=" + url.QueryEscape(q.Get("code_challenge"))); err != nil {
		t.Fatal(err)
	}
	return p.Exchange(context.Background(), "https://dashboard.fodycloud.example/auth/x/callback", "the-code", verifier)
}

func provider(t *testing.T, name string) *Provider {
	t.Helper()
	p, err := New(name, App{ClientID: "id", ClientSecret: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGitHub(t *testing.T) {
	srv := fakeForge(t, map[string]string{
		"/user": `{"id": 583231, "login": "octocat"}`,
		"/user/emails": `[{"email":"old@example.org","primary":false,"verified":true},
			{"email":"Octo@Example.org","primary":true,"verified":true}]`,
	})
	id, err := exchange(t, provider(t, GitHub), srv)
	if err != nil {
		t.Fatal(err)
	}
	if id != (Identity{Subject: "583231", Username: "octocat", Email: "octo@example.org"}) {
		t.Fatalf("identity = %+v", id)
	}
}

func TestUnverifiedEmailRefused(t *testing.T) {
	for name, api := range map[string]map[string]string{
		GitHub: {"/user": `{"id": 1, "login": "a"}`, "/user/emails": `[{"email":"a@example.org","primary":true,"verified":false}]`},
		GitLab: {"/user": `{"id": 1, "username": "a", "email": "a@example.org", "confirmed_at": null, "state": "active"}`},
		Codeberg: {"/user": `{"id": 1, "login": "a"}`, "/user/emails": `[{"email":"a@example.org","primary":true,"verified":false},
			{"email":"b@example.org","primary":false,"verified":true}]`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := exchange(t, provider(t, name), fakeForge(t, api))
			var e *Error
			if !errors.As(err, &e) || !strings.Contains(e.Msg, "no verified primary email") {
				t.Fatalf("err = %v, want the no-verified-email error", err)
			}
		})
	}
}

func TestGitLab(t *testing.T) {
	srv := fakeForge(t, map[string]string{
		"/user": `{"id": 42, "username": "ada", "email": "ada@example.org", "confirmed_at": "2024-01-01T00:00:00Z", "state": "active"}`,
	})
	id, err := exchange(t, provider(t, GitLab), srv)
	if err != nil {
		t.Fatal(err)
	}
	if id != (Identity{Subject: "42", Username: "ada", Email: "ada@example.org"}) {
		t.Fatalf("identity = %+v", id)
	}
	srv = fakeForge(t, map[string]string{"/user": `{"id": 42, "username": "ada", "email": "a@example.org", "confirmed_at": "x", "state": "blocked"}`})
	if _, err := exchange(t, provider(t, GitLab), srv); err == nil || !strings.Contains(err.Error(), "isn't active") {
		t.Fatalf("blocked account: err = %v", err)
	}
}

func TestCodeberg(t *testing.T) {
	srv := fakeForge(t, map[string]string{
		"/user":        `{"id": 7, "login": "bee"}`,
		"/user/emails": `[{"email":"bee@example.org","primary":true,"verified":true}]`,
	})
	id, err := exchange(t, provider(t, Codeberg), srv)
	if err != nil {
		t.Fatal(err)
	}
	if id != (Identity{Subject: "7", Username: "bee", Email: "bee@example.org"}) {
		t.Fatalf("identity = %+v", id)
	}
}

func TestAPIErrorsAndBadCode(t *testing.T) {
	// The API refuses: an error, not an identity.
	srv := fakeForge(t, map[string]string{"/user": `{"id": 1, "login": "a"}`}) // no /user/emails
	if _, err := exchange(t, provider(t, GitHub), srv); err == nil {
		t.Fatal("missing /user/emails: want an error")
	}
	// No account ID.
	srv = fakeForge(t, map[string]string{"/user": `{"login": "a"}`, "/user/emails": `[{"email":"a@example.org","primary":true,"verified":true}]`})
	if _, err := exchange(t, provider(t, Codeberg), srv); err == nil {
		t.Fatal("no account ID: want an error")
	}
	// A wrong PKCE verifier fails the token exchange.
	p := provider(t, GitHub)
	srv = fakeForge(t, map[string]string{})
	p.cfg.Endpoint = oauth2.Endpoint{TokenURL: srv.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}
	if _, err := p.Exchange(context.Background(), "https://x/cb", "the-code", GenerateVerifier()); err == nil {
		t.Fatal("wrong verifier: want an error")
	}
}

func TestRegistry(t *testing.T) {
	r, err := NewRegistry(nil)
	if err != nil || len(r.List()) != 0 {
		t.Fatalf("no apps: %v, %d providers", err, len(r.List()))
	}
	var nilReg *Registry
	if _, ok := nilReg.Get(GitHub); ok || nilReg.List() != nil {
		t.Fatal("a nil registry has no providers")
	}
	if _, err := NewRegistry(map[string]App{GitHub: {ClientID: "id"}}); err == nil {
		t.Fatal("ID without secret: want an error")
	}
	if _, err := NewRegistry(map[string]App{"bitbucket": {ClientID: "id", ClientSecret: "s"}}); err == nil {
		t.Fatal("unknown provider: want an error")
	}
	if _, err := NewRegistry(map[string]App{GitLab: {ClientID: "id", ClientSecret: "s", URL: "ftp://x"}}); err == nil {
		t.Fatal("bad URL: want an error")
	}
	r, err = NewRegistry(map[string]App{
		Codeberg: {ClientID: "id", ClientSecret: "s", URL: "HTTPS://Git.Example.EDU/", Label: "Campus Git"},
		GitHub:   {ClientID: "id", ClientSecret: "s"},
		GitLab:   {}, // disabled
	})
	if err != nil {
		t.Fatal(err)
	}
	if l := r.List(); len(l) != 2 || l[0].Name != GitHub || l[1].Name != Codeberg {
		t.Fatalf("providers = %+v", l)
	}
	cb, _ := r.Get(Codeberg)
	if cb.Issuer != "https://git.example.edu" || cb.Label != "Campus Git" {
		t.Fatalf("codeberg = %q %q", cb.Issuer, cb.Label)
	}
	if _, ok := r.Get(GitLab); ok {
		t.Fatal("GitLab is disabled")
	}
	gl, _ := New(GitLab, App{ClientID: "id", ClientSecret: "s"})
	if gl.Issuer != DefaultGitLabURL || gl.Label != "GitLab" {
		t.Fatalf("gitlab defaults = %q %q", gl.Issuer, gl.Label)
	}
	b, _ := json.Marshal(gl.cfg.Scopes)
	if string(b) != `["read_user"]` {
		t.Fatalf("gitlab scopes = %s", b)
	}
}
