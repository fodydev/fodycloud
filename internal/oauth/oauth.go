// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package oauth signs students in with a forge account: GitHub, GitLab
// (gitlab.com or self-hosted) and Codeberg (or another Forgejo/Gitea).
// Every provider is optional: the operator enables one by giving the client
// ID and secret of an OAuth app they registered. The provider's access token
// is used once, to read the account's ID, username and verified primary
// email, and is never stored.
package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Provider names, used in URLs (/auth/<name>/…) and stored with identities.
const (
	GitHub   = "github"
	GitLab   = "gitlab"
	Codeberg = "codeberg"
)

// Default instances.
const (
	DefaultGitLabURL   = "https://gitlab.com"
	DefaultCodebergURL = "https://codeberg.org"
)

// Identity is a forge account, as the provider reports it.
type Identity struct {
	Subject  string // the provider's numeric account ID (stable, unlike the username)
	Username string
	Email    string // the verified primary email, lower-cased
}

// Error is a failure the student can act on; its message is shown as is.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }

// App is an operator's OAuth app on one forge.
type App struct {
	ClientID, ClientSecret string
	URL                    string // instance base URL (GitLab, Codeberg); "" = the default
	Label                  string // button label; "" = the forge's name
}

// Enabled reports whether the app is configured.
func (a App) Enabled() bool { return a.ClientID != "" && a.ClientSecret != "" }

// Provider is one enabled forge.
type Provider struct {
	Name  string // GitHub, GitLab or Codeberg
	Label string // shown on buttons: "GitHub", "GitLab", "Codeberg", or the operator's label
	// Issuer identifies the instance (its base URL). Identities are keyed by
	// issuer and subject, so account 42 of one GitLab never matches account
	// 42 of another.
	Issuer string

	cfg     oauth2.Config
	apiBase string
	client  *http.Client
}

// New returns the provider for a forge, or an error for an unknown name or
// an invalid instance URL.
func New(name string, app App) (*Provider, error) {
	p := &Provider{Name: name, Label: app.Label, client: &http.Client{Timeout: 15 * time.Second}}
	p.cfg = oauth2.Config{ClientID: app.ClientID, ClientSecret: app.ClientSecret}
	switch name {
	case GitHub:
		p.Issuer, p.apiBase = "https://github.com", "https://api.github.com"
		p.cfg.Endpoint = oauth2.Endpoint{AuthURL: p.Issuer + "/login/oauth/authorize", TokenURL: p.Issuer + "/login/oauth/access_token"}
		p.cfg.Scopes = []string{"read:user", "user:email"}
		if p.Label == "" {
			p.Label = "GitHub"
		}
	case GitLab:
		base, err := baseURL(app.URL, DefaultGitLabURL)
		if err != nil {
			return nil, fmt.Errorf("gitlab URL: %w", err)
		}
		p.Issuer, p.apiBase = base, base+"/api/v4"
		p.cfg.Endpoint = oauth2.Endpoint{AuthURL: base + "/oauth/authorize", TokenURL: base + "/oauth/token"}
		p.cfg.Scopes = []string{"read_user"}
		if p.Label == "" {
			p.Label = "GitLab"
		}
	case Codeberg:
		base, err := baseURL(app.URL, DefaultCodebergURL)
		if err != nil {
			return nil, fmt.Errorf("codeberg URL: %w", err)
		}
		p.Issuer, p.apiBase = base, base+"/api/v1"
		p.cfg.Endpoint = oauth2.Endpoint{AuthURL: base + "/login/oauth/authorize", TokenURL: base + "/login/oauth/access_token"}
		p.cfg.Scopes = []string{"read:user"}
		if p.Label == "" {
			p.Label = "Codeberg"
		}
	default:
		return nil, fmt.Errorf("unknown OAuth provider %q", name)
	}
	return p, nil
}

// baseURL validates an instance URL: http(s), a host, no query; the trailing
// slash is dropped and the scheme and host lower-cased, so the issuer is stable.
func baseURL(raw, def string) (string, error) {
	if raw == "" {
		raw = def
	}
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(raw), "/"))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("%q is not a valid http(s) base URL", raw)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.EscapedPath(), nil
}

// AuthURL is where the browser goes to sign in: state ties the callback to
// this browser, and verifier is the PKCE secret (oauth2.GenerateVerifier).
func (p *Provider) AuthURL(redirectURL, state, verifier string) string {
	cfg := p.cfg
	cfg.RedirectURL = redirectURL
	return cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
}

// Exchange trades the callback's code for a token, reads the account with
// it, and drops the token.
func (p *Provider) Exchange(ctx context.Context, redirectURL, code, verifier string) (Identity, error) {
	cfg := p.cfg
	cfg.RedirectURL = redirectURL
	ctx = context.WithValue(ctx, oauth2.HTTPClient, p.client)
	tok, err := cfg.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, fmt.Errorf("%s token exchange: %w", p.Name, err)
	}
	c := cfg.Client(ctx, tok)
	c.Timeout = p.client.Timeout
	var id Identity
	switch p.Name {
	case GitHub:
		id, err = p.github(ctx, c)
	case GitLab:
		id, err = p.gitlab(ctx, c)
	default:
		id, err = p.forgejo(ctx, c)
	}
	if err != nil {
		return Identity{}, err
	}
	id.Email = strings.ToLower(strings.TrimSpace(id.Email))
	if id.Subject == "" || id.Subject == "0" {
		return Identity{}, fmt.Errorf("%s returned no account ID", p.Name)
	}
	if id.Email == "" {
		return Identity{}, &Error{Msg: fmt.Sprintf("Your %s account has no verified primary email address. Verify one on %s, then try again.", p.Label, p.Label)}
	}
	return id, nil
}

type email struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func primaryVerified(emails []email) string {
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email
		}
	}
	return ""
}

func (p *Provider) github(ctx context.Context, c *http.Client) (Identity, error) {
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := p.get(ctx, c, "/user", &u); err != nil {
		return Identity{}, err
	}
	var emails []email
	if err := p.get(ctx, c, "/user/emails", &emails); err != nil {
		return Identity{}, err
	}
	return Identity{Subject: strconv.FormatInt(u.ID, 10), Username: u.Login, Email: primaryVerified(emails)}, nil
}

func (p *Provider) gitlab(ctx context.Context, c *http.Client) (Identity, error) {
	// The user's email is the primary one; confirmed_at says whether it is
	// confirmed (absent or null: not confirmed, or not visible to us).
	var u struct {
		ID          int64   `json:"id"`
		Username    string  `json:"username"`
		Email       string  `json:"email"`
		ConfirmedAt *string `json:"confirmed_at"`
		State       string  `json:"state"`
	}
	if err := p.get(ctx, c, "/user", &u); err != nil {
		return Identity{}, err
	}
	if u.State != "" && u.State != "active" {
		return Identity{}, &Error{Msg: fmt.Sprintf("Your %s account isn't active.", p.Label)}
	}
	id := Identity{Subject: strconv.FormatInt(u.ID, 10), Username: u.Username}
	if u.ConfirmedAt != nil && *u.ConfirmedAt != "" {
		id.Email = u.Email
	}
	return id, nil
}

func (p *Provider) forgejo(ctx context.Context, c *http.Client) (Identity, error) {
	var u struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	}
	if err := p.get(ctx, c, "/user", &u); err != nil {
		return Identity{}, err
	}
	var emails []email
	if err := p.get(ctx, c, "/user/emails", &emails); err != nil {
		return Identity{}, err
	}
	return Identity{Subject: strconv.FormatInt(u.ID, 10), Username: u.Login, Email: primaryVerified(emails)}, nil
}

// maxBody bounds the provider's API responses.
const maxBody = 1 << 20

func (p *Provider) get(ctx context.Context, c *http.Client, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.apiBase+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "FodyCloud")
	if p.Name == GitHub {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("%s API %s: %w", p.Name, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody)) //nolint:errcheck
		return fmt.Errorf("%s API %s: HTTP %d", p.Name, path, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return fmt.Errorf("%s API %s: %w", p.Name, path, err)
	}
	return nil
}

// Registry holds the enabled providers, in display order.
type Registry struct{ list []*Provider }

// NewRegistry builds the providers whose apps are configured. An app with
// only one of its client ID and secret is a configuration error.
func NewRegistry(apps map[string]App) (*Registry, error) {
	r := &Registry{}
	for _, name := range []string{GitHub, GitLab, Codeberg} {
		app := apps[name]
		if (app.ClientID == "") != (app.ClientSecret == "") {
			return nil, fmt.Errorf("OAuth %s: set both the client ID and the client secret, or neither", name)
		}
		if !app.Enabled() {
			continue
		}
		p, err := New(name, app)
		if err != nil {
			return nil, err
		}
		r.list = append(r.list, p)
	}
	for name := range apps {
		if name != GitHub && name != GitLab && name != Codeberg {
			return nil, errors.New("unknown OAuth provider " + strconv.Quote(name))
		}
	}
	return r, nil
}

// List returns the enabled providers (none when r is nil).
func (r *Registry) List() []*Provider {
	if r == nil {
		return nil
	}
	return r.list
}

// Get returns an enabled provider by name.
func (r *Registry) Get(name string) (*Provider, bool) {
	for _, p := range r.List() {
		if p.Name == name {
			return p, true
		}
	}
	return nil, false
}

// GenerateVerifier returns a new PKCE verifier.
func GenerateVerifier() string { return oauth2.GenerateVerifier() }
