// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/docs"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/oauth"
	"github.com/fodydev/fodycloud/internal/sites"
)

// Sign-in with a forge (GitHub, GitLab, Codeberg), on the dashboard site.
//
//   - GET /auth/{provider}/start (log in or sign up) and POST
//     /account/forges/{provider}/connect (link to the signed-in account) send
//     the browser to the forge, with a state and a PKCE verifier kept in a
//     short-lived, host-only cookie.
//   - GET /auth/{provider}/callback checks the state, exchanges the code and
//     signs in, links the account, or starts a sign-up.
//   - A sign-up waits on /auth/confirm for the terms; its token is in a
//     host-only cookie, never in a URL.

const (
	intentLogin = "login"
	intentLink  = "link"
	// flowTTL bounds the time spent on the forge's pages.
	flowTTL = 10 * time.Minute
)

func (w *Web) flowCookie() string   { return w.hostCookie("fc_oauth") }
func (w *Web) signupCookie() string { return w.hostCookie("fc_signup") }

// hostCookie names a host-only cookie: with Secure cookies, the __Host-
// prefix stops a student app under *.apps.<domain> from planting it.
func (w *Web) hostCookie(name string) string {
	if w.SecureCookies {
		return "__Host-" + name
	}
	return name
}

func (w *Web) setHostCookie(rw http.ResponseWriter, name, value string, ttl time.Duration) {
	http.SetCookie(rw, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: w.SecureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
}

func (w *Web) clearHostCookie(rw http.ResponseWriter, name string) {
	http.SetCookie(rw, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: w.SecureCookies, MaxAge: -1})
}

func (w *Web) callbackURL(p *oauth.Provider) string {
	return w.sites.URL(sites.Dashboard, "/auth/"+p.Name+"/callback")
}

// startForge sends the browser to the forge's sign-in page.
func (w *Web) startForge(rw http.ResponseWriter, r *http.Request, p *oauth.Provider, intent string) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		http.Error(rw, "internal error", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(b)
	verifier := oauth.GenerateVerifier()
	// Every part is URL-safe base64 or a fixed word, so "." separates them.
	w.setHostCookie(rw, w.flowCookie(), strings.Join([]string{p.Name, intent, state, verifier}, "."), flowTTL)
	rw.Header().Set("Cache-Control", "no-store")
	http.Redirect(rw, r, p.AuthURL(w.callbackURL(p), state, verifier), http.StatusSeeOther)
}

// forgeRoutes registers the sign-in routes; the Account page's connect and
// disconnect forms are in accountRoutes.
func (w *Web) forgeRoutes(mux *http.ServeMux) {
	provider := func(rw http.ResponseWriter, r *http.Request) (*oauth.Provider, bool) {
		p, ok := w.Forges.Get(r.PathValue("provider"))
		if !ok {
			w.notFound(rw, r)
		}
		return p, ok
	}
	mux.HandleFunc("GET /auth/{provider}/start", func(rw http.ResponseWriter, r *http.Request) {
		if p, ok := provider(rw, r); ok {
			w.startForge(rw, r, p, intentLogin)
		}
	})
	mux.HandleFunc("GET /auth/{provider}/callback", func(rw http.ResponseWriter, r *http.Request) {
		p, ok := provider(rw, r)
		if !ok || limited(w.LoginLimiter, rw, r) {
			return
		}
		rw.Header().Set("Cache-Control", "no-store")
		intent, verifier, ok := w.checkFlow(rw, r, p)
		back := "/login"
		if intent == intentLink {
			back = "/account"
		}
		if !ok {
			redirect(rw, r, back, "error", "This sign-in link is invalid or has expired. Try again.")
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			msg := "Sign-in with " + p.Label + " was cancelled."
			if e != "access_denied" {
				msg = p.Label + " refused the sign-in. Try again."
			}
			redirect(rw, r, back, "error", msg)
			return
		}
		id, err := p.Exchange(r.Context(), w.callbackURL(p), r.URL.Query().Get("code"), verifier)
		if err != nil {
			var oe *oauth.Error
			if errors.As(err, &oe) {
				redirect(rw, r, back, "error", oe.Msg)
				return
			}
			w.Log.Warn("forge sign-in failed", "provider", p.Name, "err", err)
			redirect(rw, r, back, "error", "Sign-in with "+p.Label+" failed. Try again.")
			return
		}
		if intent == intentLink {
			u, err := w.sessionUser(r)
			if err != nil {
				http.Redirect(rw, r, "/login", http.StatusSeeOther)
				return
			}
			if err := w.Svc.ConnectForge(r.Context(), w.Auth, u, p, id); err != nil {
				redirect(rw, r, "/account", "error", errMsg(err))
				return
			}
			redirect(rw, r, "/account", "ok", p.Label+" connected. You can now log in with it.")
			return
		}
		res, err := w.Svc.SignInWithForge(r.Context(), w.Auth, p, id)
		switch {
		case errors.Is(err, auth.ErrBanned):
			redirect(rw, r, "/login", "error", "This account is suspended.")
		case err != nil:
			redirect(rw, r, "/login", "error", errMsg(err))
		case res.User != nil:
			w.startSession(rw, r, res.User)
		default:
			w.setHostCookie(rw, w.signupCookie(), res.PendingSignup, 15*time.Minute)
			http.Redirect(rw, r, "/auth/confirm", http.StatusSeeOther)
		}
	})
	mux.HandleFunc("GET /auth/confirm", func(rw http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie(w.signupCookie())
		var tok string
		if c != nil {
			tok = c.Value
		}
		email, id, err := w.Svc.PendingForgeSignup(r.Context(), w.Auth, tok)
		if err != nil {
			redirect(rw, r, "/signup", "error", errMsg(err))
			return
		}
		label := id.Provider
		if p, ok := w.Forges.Get(id.Provider); ok {
			label = p.Label
		}
		rw.Header().Set("Cache-Control", "no-store")
		w.render(rw, r, "forgeconfirm", "Create your account", nil, map[string]any{"Email": email, "Username": id.Username, "Provider": label})
	})
	mux.HandleFunc("POST /auth/confirm", w.publicForm(func(rw http.ResponseWriter, r *http.Request) {
		c, _ := r.Cookie(w.signupCookie())
		var tok string
		if c != nil {
			tok = c.Value
		}
		u, err := w.Svc.CompleteForgeSignup(r.Context(), w.Auth, tok, r.FormValue("accept_terms") == "1", docs.LegalVersion)
		if err != nil {
			target := "/signup"
			if r.FormValue("accept_terms") != "1" {
				target = "/auth/confirm"
			}
			redirect(rw, r, target, "error", errMsg(err))
			return
		}
		w.clearHostCookie(rw, w.signupCookie())
		w.startSession(rw, r, u)
	}))
}

// checkFlow reads and clears the flow cookie, and checks that it belongs to
// this provider and that the callback's state matches it.
func (w *Web) checkFlow(rw http.ResponseWriter, r *http.Request, p *oauth.Provider) (intent, verifier string, ok bool) {
	c, err := r.Cookie(w.flowCookie())
	if err != nil {
		return "", "", false
	}
	w.clearHostCookie(rw, w.flowCookie())
	parts := strings.Split(c.Value, ".")
	if len(parts) != 4 || parts[0] != p.Name || (parts[1] != intentLogin && parts[1] != intentLink) {
		return "", "", false
	}
	state := r.URL.Query().Get("state")
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(parts[2])) != 1 {
		return parts[1], "", false
	}
	return parts[1], parts[3], true
}

// forgeAccountRoutes: the Account page's sign-in methods.
func (w *Web) forgeAccountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /account/forges/{provider}/connect", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		p, ok := w.Forges.Get(r.PathValue("provider"))
		if !ok {
			w.notFound(rw, r)
			return
		}
		w.startForge(rw, r, p, intentLink)
	}))
	mux.HandleFunc("POST /account/forges/{provider}/disconnect", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		// A provider the operator turned off can still be disconnected.
		name := r.PathValue("provider")
		label := name
		if p, ok := w.Forges.Get(name); ok {
			label = p.Label
		} else if l, ok := forgeNames[name]; ok {
			label = l
		}
		if err := w.Svc.DisconnectForge(r.Context(), w.Auth, u, name, label); err != nil {
			redirect(rw, r, "/account", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/account", "ok", label+" disconnected.")
	}))
	mux.HandleFunc("POST /account/set-password", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		if r.FormValue("password") != r.FormValue("password2") {
			redirect(rw, r, "/account", "error", "The two new passwords are different.")
			return
		}
		c, _ := r.Cookie(w.cookieName(dashboardSession))
		if err := w.Svc.SetFirstPassword(r.Context(), w.Auth, u, c.Value, r.FormValue("password")); err != nil {
			redirect(rw, r, "/account", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/account", "ok", "Password set. You can now log in with your email and password too.")
	}))
}

// forgeNames labels providers the operator turned off.
var forgeNames = map[string]string{oauth.GitHub: "GitHub", oauth.GitLab: "GitLab", oauth.Codeberg: "Codeberg"}

// forgeMethod is one row of the Account page's sign-in methods.
type forgeMethod struct {
	Name, Label, Username string
	Linked, Enabled       bool
}

// forgeMethods lists the enabled forges and those linked to the student.
func (w *Web) forgeMethods(ids []auth.Identity) []forgeMethod {
	var out []forgeMethod
	linked := map[string]auth.Identity{}
	for _, i := range ids {
		linked[i.Provider] = i
	}
	for _, p := range w.Forges.List() {
		i, ok := linked[p.Name]
		out = append(out, forgeMethod{Name: p.Name, Label: p.Label, Username: i.Username, Linked: ok, Enabled: true})
		delete(linked, p.Name)
	}
	for _, name := range []string{oauth.GitHub, oauth.GitLab, oauth.Codeberg} { // linked, but turned off by the operator
		if i, ok := linked[name]; ok {
			out = append(out, forgeMethod{Name: name, Label: forgeNames[name], Username: i.Username, Linked: true})
		}
	}
	return out
}
