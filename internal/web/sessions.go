// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fodydev/fodycloud/internal/api"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/ratelimit"
)

// hintCookie tells the public home page that the visitor is signed in, so it
// can send them to the dashboard. It holds no secret: the dashboard still
// checks the real session, and a forged hint only costs a redirect.
const hintCookie = "fc_signed_in"

// sessionSite says which site a browser session belongs to. Students sign
// in to the dashboard, admins to the admin panel: separate accounts, tables
// and cookies.
type sessionSite int

// cookieName is the session cookie of a site. With Secure cookies the
// __Host- prefix makes the browser enforce host-only, path=/ cookies, so a
// student app under *.apps.<domain> can't plant one.
func (w *Web) cookieName(site sessionSite) string {
	name := "fc_session"
	if site == adminSession {
		name = "fc_admin"
	}
	if w.SecureCookies {
		return "__Host-" + name
	}
	return name
}

// sessionUser returns the student signed in to the dashboard.
func (w *Web) sessionUser(r *http.Request) (*auth.User, error) {
	c, err := r.Cookie(w.cookieName(dashboardSession))
	if err != nil {
		return nil, auth.ErrUnauthenticated
	}
	return w.Auth.SessionUser(r.Context(), c.Value)
}

// sessionAdmin returns the admin signed in to the admin panel.
func (w *Web) sessionAdmin(r *http.Request) (*auth.Admin, error) {
	c, err := r.Cookie(w.cookieName(adminSession))
	if err != nil {
		return nil, auth.ErrUnauthenticated
	}
	return w.Auth.AdminSession(r.Context(), c.Value)
}

func (w *Web) setSessionCookie(rw http.ResponseWriter, site sessionSite, tok string) {
	ttl := auth.SessionTTL
	if site == adminSession {
		ttl = auth.AdminSessionTTL
	}
	http.SetCookie(rw, &http.Cookie{Name: w.cookieName(site), Value: tok, Path: "/", HttpOnly: true, Secure: w.SecureCookies,
		SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
	if site == dashboardSession {
		http.SetCookie(rw, &http.Cookie{Name: hintCookie, Value: "1", Domain: w.BaseDomain, Path: "/", Secure: w.SecureCookies,
			SameSite: http.SameSiteLaxMode, MaxAge: int(ttl.Seconds())})
	}
}

func (w *Web) clearSessionCookie(rw http.ResponseWriter, site sessionSite) {
	http.SetCookie(rw, &http.Cookie{Name: w.cookieName(site), Value: "", Path: "/", Secure: w.SecureCookies, MaxAge: -1})
	if site == dashboardSession {
		http.SetCookie(rw, &http.Cookie{Name: hintCookie, Value: "", Domain: w.BaseDomain, Path: "/", Secure: w.SecureCookies, MaxAge: -1})
	}
}

func (w *Web) startSession(rw http.ResponseWriter, r *http.Request, u *auth.User) {
	tok, err := w.Auth.CreateSession(r.Context(), u.ID)
	if err != nil {
		redirect(rw, r, "/login", "error", "could not start a session")
		return
	}
	w.setSessionCookie(rw, dashboardSession, tok)
	http.Redirect(rw, r, "/apps", http.StatusSeeOther)
}

// checkLogin verifies a sign-in form: same-origin, per-IP rate limit, and the
// per-account lockout (keys prefixed per kind of account, e.g. "admin:").
// authenticate checks the credentials. On failure it writes the response
// itself and returns false.
func (w *Web) checkLogin(rw http.ResponseWriter, r *http.Request, lockoutPrefix string, authenticate func(email, password string) error) bool {
	if !api.SameOrigin(r) {
		http.Error(rw, "cross-origin request refused", http.StatusForbidden)
		return false
	}
	if limited(w.LoginLimiter, rw, r) {
		return false
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	key := lockoutPrefix + email
	if w.Lockout != nil {
		if locked, wait := w.Lockout.Locked(key); locked {
			ratelimit.TooMany(rw, wait, "Too many failed logins for this account. Try again later or reset your password.")
			return false
		}
	}
	err := authenticate(email, r.FormValue("password"))
	if errors.Is(err, auth.ErrBanned) {
		redirect(rw, r, "/login", "error", "This account is suspended.")
		return false
	}
	if err != nil {
		if w.Lockout != nil {
			w.Lockout.Fail(key)
		}
		redirect(rw, r, "/login", "error", "Invalid email or password")
		return false
	}
	if w.Lockout != nil {
		w.Lockout.Reset(key)
	}
	return true
}
