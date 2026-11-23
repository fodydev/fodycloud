// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/docs"
	"github.com/fodydev/fodycloud/internal/api"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/service"
)

// adminHandler serves the admin panel and the admin guide on admin.<domain>,
// behind their own sign-in. Only admin accounts (a separate kind of account
// from students) can sign in; sessions last 8 hours.
func (w *Web) adminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", w.staticHandler())
	loginPage := func(rw http.ResponseWriter, r *http.Request) {
		w.render(rw, r, "login", "Admin sign-in", nil, map[string]any{"Admin": true})
	}
	// admin wraps pages that need an admin session; others see the sign-in form.
	admin := func(fn func(http.ResponseWriter, *http.Request, *auth.Admin)) http.HandlerFunc {
		return func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Set("Cache-Control", "no-store")
			ad, err := w.sessionAdmin(r)
			if err != nil {
				if r.Method != http.MethodGet {
					http.Redirect(rw, r, "/login", http.StatusSeeOther)
					return
				}
				loginPage(rw, r)
				return
			}
			if r.Method == http.MethodPost && !api.SameOrigin(r) {
				http.Error(rw, "cross-origin request refused", http.StatusForbidden)
				return
			}
			fn(rw, r, ad)
		}
	}
	mux.HandleFunc("GET /login", loginPage)
	mux.HandleFunc("POST /login", func(rw http.ResponseWriter, r *http.Request) {
		var ad *auth.Admin
		ok := w.checkLogin(rw, r, "admin:", func(email, password string) (err error) {
			ad, err = w.Auth.AuthenticateAdmin(r.Context(), email, password)
			return err
		})
		if !ok {
			return
		}
		tok, err := w.Auth.CreateAdminSession(r.Context(), ad.ID)
		if err != nil {
			redirect(rw, r, "/login", "error", "could not start a session")
			return
		}
		w.setSessionCookie(rw, adminSession, tok)
		http.Redirect(rw, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /logout", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) {
		if c, err := r.Cookie(w.cookieName(adminSession)); err == nil {
			w.Auth.DeleteAdminSession(r.Context(), c.Value) //nolint:errcheck
		}
		w.clearSessionCookie(rw, adminSession)
		redirect(rw, r, "/login", "ok", "Signed out of the admin panel.")
	}))
	panel := func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin, newToken string) {
		users, err := w.Svc.ListUsers(r.Context(), ad)
		if err != nil {
			http.Error(rw, errMsg(err), http.StatusInternalServerError)
			return
		}
		repos, _ := w.Svc.ListBannedRepos(r.Context(), ad)
		flags, _ := w.Svc.ListAbuseFlags(r.Context(), ad)
		tokens, _ := w.Auth.ListAdminTokens(r.Context(), ad.ID)
		admins, _ := w.Auth.ListAdmins(r.Context())
		storage, _ := w.Svc.StorageTotals(r.Context(), ad)
		w.renderAdmin(rw, r, "admin", "Admin", ad, map[string]any{"Users": users, "Plans": w.Svc.Plans.Names(), "Repos": repos,
			"Flags": flags, "Tokens": tokens, "NewToken": newToken, "Admins": admins, "Storage": storage})
	}
	mux.HandleFunc("GET /{$}", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) { panel(rw, r, ad, "") }))
	mux.HandleFunc("POST /tokens", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) {
		tok, err := w.Auth.CreateAdminToken(r.Context(), ad.ID, r.FormValue("name"))
		if err != nil {
			redirect(rw, r, "/", "error", err.Error())
			return
		}
		panel(rw, r, ad, tok) // shown once, never put in a URL
	}))
	mux.HandleFunc("POST /tokens/{id}/revoke", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := w.Auth.RevokeAdminToken(r.Context(), ad.ID, id); err != nil {
			redirect(rw, r, "/", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/", "ok", "Admin token revoked.")
	}))
	mux.HandleFunc("GET /docs", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) {
		w.serveAdminDocs(rw, r, docs.AdminNav[0].Pages[0].Slug, ad)
	}))
	mux.HandleFunc("GET /docs/{page}", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) {
		w.serveAdminDocs(rw, r, "admin/"+r.PathValue("page"), ad)
	}))
	action := func(path string, fn func(r *http.Request, ad *auth.Admin) (string, error)) {
		mux.HandleFunc("POST /"+path, admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) {
			msg, err := fn(r, ad)
			if err != nil {
				redirect(rw, r, "/", "error", errMsg(err))
				return
			}
			redirect(rw, r, "/", "ok", msg)
		}))
	}
	action("ban", func(r *http.Request, ad *auth.Admin) (string, error) {
		return r.FormValue("email") + " suspended.", w.Svc.Ban(r.Context(), ad, w.Auth, r.FormValue("email"), r.FormValue("reason"))
	})
	action("unban", func(r *http.Request, ad *auth.Admin) (string, error) {
		return r.FormValue("email") + " reinstated.", w.Svc.Unban(r.Context(), ad, r.FormValue("email"))
	})
	action("repos/ban", func(r *http.Request, ad *auth.Admin) (string, error) {
		url, err := w.Svc.BanRepo(r.Context(), ad, r.FormValue("repo_url"), r.FormValue("reason"))
		return url + " banned.", err
	})
	action("repos/unban", func(r *http.Request, ad *auth.Admin) (string, error) {
		return "Repository allowed again.", w.Svc.UnbanRepo(r.Context(), ad, r.FormValue("repo_url"))
	})
	action("abuse/resolve", func(r *http.Request, ad *auth.Admin) (string, error) {
		id, _ := strconv.ParseInt(r.FormValue("id"), 10, 64)
		return "Flag resolved.", w.Svc.ResolveAbuseFlag(r.Context(), ad, id)
	})
	action("plan", func(r *http.Request, ad *auth.Admin) (string, error) {
		var until *time.Time
		if v := strings.TrimSpace(r.FormValue("valid_until")); v != "" {
			t, err := time.Parse("2006-01-02", v)
			if err != nil {
				return "", &service.InvalidError{Msg: "invalid date"}
			}
			t = t.Add(24*time.Hour - time.Second) // valid through the end of that day
			until = &t
		}
		email := r.FormValue("email")
		_, err := w.Svc.SetPlan(r.Context(), ad, email, r.FormValue("plan"), until, r.FormValue("note"))
		return "Plan updated for " + email + ".", err
	})
	mux.HandleFunc("/", admin(func(rw http.ResponseWriter, r *http.Request, ad *auth.Admin) { w.notFound(rw, r) }))
	return mux
}
