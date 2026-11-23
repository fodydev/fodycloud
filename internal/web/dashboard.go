// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/fodydev/fodycloud/docs"

	"github.com/fodydev/fodycloud/internal/api"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/ratelimit"
	"github.com/fodydev/fodycloud/internal/service"
	"github.com/fodydev/fodycloud/internal/sites"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// dashboardHandler serves dashboard.<domain>: sign-in and sign-up, the
// students' apps and account pages, and the guides for signed-in students.
func (w *Web) dashboardHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", w.staticHandler())
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		if _, err := w.sessionUser(r); err == nil {
			http.Redirect(rw, r, "/apps", http.StatusSeeOther)
			return
		}
		http.Redirect(rw, r, "/login", http.StatusSeeOther)
	})
	// The guides inside the dashboard; signed-out visitors read the public copy.
	w.docsRoutes(mux, func(rw http.ResponseWriter, r *http.Request, slug string) {
		u, err := w.sessionUser(r)
		if err != nil {
			w.clearSessionCookie(rw, dashboardSession)
			http.Redirect(rw, r, w.sites.URL(sites.Home, r.URL.Path), http.StatusSeeOther)
			return
		}
		w.serveDocs(rw, r, publicDocs(), w.docs, slug, u)
	})
	w.signInRoutes(mux)
	w.appRoutes(mux)
	w.accountRoutes(mux)
	mux.HandleFunc("/", w.notFound)
	return mux
}

// authed wraps dashboard pages that need a signed-in student. Visitors
// without a session go to the sign-in page; POSTs must be same-origin (CSRF).
func (w *Web) authed(fn func(http.ResponseWriter, *http.Request, *auth.User)) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		u, err := w.sessionUser(r)
		if err != nil {
			w.clearSessionCookie(rw, dashboardSession) // also drops a stale "signed in" hint
			http.Redirect(rw, r, "/login", http.StatusSeeOther)
			return
		}
		if r.Method == http.MethodPost && !api.SameOrigin(r) {
			http.Error(rw, "cross-origin request refused", http.StatusForbidden)
			return
		}
		fn(rw, r, u)
	}
}

// passwordForm wraps the account forms that ask for the student's password.
// A wrong password counts toward the account's sign-in lockout (so a stolen
// session can't be used to guess it); while locked, the form is refused.
// fn's error is shown on the Account page.
func (w *Web) passwordForm(fn func(http.ResponseWriter, *http.Request, *auth.User) error) http.HandlerFunc {
	return w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		if w.Lockout != nil {
			if locked, wait := w.Lockout.Locked(u.Email); locked {
				ratelimit.TooMany(rw, wait, "Too many wrong passwords for this account. Try again later.")
				return
			}
		}
		err := fn(rw, r, u)
		if err == nil {
			return
		}
		if errors.Is(err, service.ErrWrongPassword) && w.Lockout != nil {
			w.Lockout.Fail(u.Email)
		}
		redirect(rw, r, "/account", "error", errMsg(err))
	})
}

// publicForm wraps the forms visitors post without a session (sign-up,
// password reset): same-origin only, and limited per IP.
func (w *Web) publicForm(fn http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if !api.SameOrigin(r) {
			http.Error(rw, "cross-origin request refused", http.StatusForbidden)
			return
		}
		if limited(w.SignupLimiter, rw, r) {
			return
		}
		fn(rw, r)
	}
}

// signInRoutes: sign-in, sign-up, email verification, password reset, sign-out.
func (w *Web) signInRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", func(rw http.ResponseWriter, r *http.Request) { w.render(rw, r, "login", "Log in", nil, nil) })
	mux.HandleFunc("GET /signup", func(rw http.ResponseWriter, r *http.Request) { w.render(rw, r, "signup", "Sign up", nil, nil) })
	mux.HandleFunc("POST /login", func(rw http.ResponseWriter, r *http.Request) {
		var u *auth.User
		ok := w.checkLogin(rw, r, "", func(email, password string) (err error) {
			u, err = w.Auth.Authenticate(r.Context(), email, password)
			return err
		})
		if ok {
			w.startSession(rw, r, u)
		}
	})
	mux.HandleFunc("POST /signup", w.publicForm(func(rw http.ResponseWriter, r *http.Request) {
		if r.FormValue("accept_terms") != "1" {
			redirect(rw, r, "/signup", "error", "Accept the Terms of service and the Acceptable-use policy to create an account.")
			return
		}
		u, err := w.Auth.CreateUser(r.Context(), r.FormValue("email"), r.FormValue("password"), !w.Svc.RequireVerification)
		if err != nil {
			redirect(rw, r, "/signup", "error", err.Error())
			return
		}
		if err := w.Auth.AcceptTerms(r.Context(), u.ID, docs.LegalVersion); err != nil {
			w.Log.Error("record terms acceptance", "err", err)
		}
		if err := w.Svc.SendVerification(r.Context(), w.Auth, u); err != nil {
			w.Log.Error("send verification", "err", err)
		}
		w.startSession(rw, r, u)
	}))
	mux.HandleFunc("GET /verify", func(rw http.ResponseWriter, r *http.Request) {
		if err := w.Svc.VerifyEmail(r.Context(), w.Auth, r.URL.Query().Get("token")); err != nil {
			redirect(rw, r, "/apps", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/apps", "ok", "Email confirmed. You can deploy your apps now.")
	})
	// Link from the "confirm your new email" message (works signed in or not).
	mux.HandleFunc("GET /verify-email", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Referrer-Policy", "no-referrer") // keep the token out of Referer headers
		email, err := w.Svc.ConfirmEmailChange(r.Context(), w.Auth, r.URL.Query().Get("token"))
		if err != nil {
			redirect(rw, r, "/login", "error", errMsg(err))
			return
		}
		target := "/login" // the student may open the link on another device
		if _, err := w.sessionUser(r); err == nil {
			target = "/account"
		}
		redirect(rw, r, target, "ok", "Your email is now "+email+".")
	})
	mux.HandleFunc("POST /verify/resend", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		if limited(w.SignupLimiter, rw, r) {
			return
		}
		if err := w.Svc.SendVerification(r.Context(), w.Auth, u); err != nil {
			redirect(rw, r, "/apps", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/apps", "ok", "A new confirmation link was sent to "+u.Email+".")
	}))
	mux.HandleFunc("GET /forgot", func(rw http.ResponseWriter, r *http.Request) { w.render(rw, r, "forgot", "Reset password", nil, nil) })
	mux.HandleFunc("POST /forgot", w.publicForm(func(rw http.ResponseWriter, r *http.Request) {
		if err := w.Svc.RequestPasswordReset(r.Context(), w.Auth, r.FormValue("email")); err != nil {
			w.Log.Error("password reset request", "err", err)
		}
		// Same answer whether or not the account exists.
		redirect(rw, r, "/login", "ok", "If an account exists for this email, a reset link is on its way.")
	}))
	mux.HandleFunc("GET /reset", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Referrer-Policy", "no-referrer") // keep the token out of Referer headers
		w.render(rw, r, "reset", "Choose a new password", nil, map[string]string{"Token": r.URL.Query().Get("token")})
	})
	mux.HandleFunc("POST /reset", w.publicForm(func(rw http.ResponseWriter, r *http.Request) {
		if err := w.Svc.ResetPassword(r.Context(), w.Auth, r.FormValue("token"), r.FormValue("password")); err != nil {
			redirect(rw, r, "/forgot", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/login", "ok", "Password changed. Log in with your new password.")
	}))
	mux.HandleFunc("POST /logout", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		if c, err := r.Cookie(w.cookieName(dashboardSession)); err == nil {
			w.Auth.DeleteSession(r.Context(), c.Value) //nolint:errcheck
		}
		w.clearSessionCookie(rw, dashboardSession)
		http.Redirect(rw, r, w.sites.URL(sites.Home, "/"), http.StatusSeeOther)
	}))
}

// appRoutes: the apps list, an app's page and its actions (deploy, stack,
// env vars, domains…), logs.
func (w *Web) appRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /apps", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		apps, err := w.Svc.ListApps(r.Context(), u)
		if err != nil {
			w.Log.Error("list apps", "err", err)
		}
		info, _ := w.Svc.PlanInfo(r.Context(), u)
		w.render(rw, r, "apps", "Your apps", u, map[string]any{"Apps": apps, "Plan": info})
	}))
	mux.HandleFunc("POST /apps", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		env, err := service.ParseDotEnv(r.FormValue("env"))
		if err != nil {
			redirect(rw, r, "/apps", "error", errMsg(err))
			return
		}
		// No stack: FodyCloud analyses the repository and the student reviews the result.
		n := service.NewApp{Name: r.FormValue("name"), RepoURL: r.FormValue("repo_url"), Branch: r.FormValue("branch"), Env: env}
		if dir := strings.TrimSpace(r.FormValue("root_dir")); dir != "" {
			n.Stack = &stacks.Stack{RootDir: dir} // analyse this folder
		}
		app, _, err := w.Svc.CreateApp(r.Context(), u, n)
		if err != nil {
			redirect(rw, r, "/apps", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/apps/"+app.Name, "ok", "App created. FodyCloud is analysing your repository: review the detected stack before the first deploy.")
	}))
	mux.HandleFunc("GET /apps/{name}", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		name := r.PathValue("name")
		app, err := w.Svc.GetApp(r.Context(), u, name)
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		deps, _ := w.Svc.ListDeployments(r.Context(), u, name, 10)
		env, _ := w.Svc.ListEnv(r.Context(), u, name, false)
		info, _ := w.Svc.PlanInfo(r.Context(), u)
		var latest *service.Deployment
		if len(deps) > 0 {
			latest = deps[0]
		}
		rng := r.URL.Query().Get("range")
		if !slices.Contains(service.MetricRanges, rng) {
			rng = "24h"
		}
		metrics, err := w.Svc.AppMetrics(r.Context(), u, name, rng)
		if err != nil {
			w.Log.Error("app metrics", "app", name, "err", err)
		}
		services, err := w.Svc.Services(r.Context(), u, name)
		if err != nil {
			w.Log.Error("app services", "app", name, "err", err)
		}
		rw.Header().Set("Cache-Control", "no-store") // connection URLs contain passwords
		w.render(rw, r, "app", app.Name, u, map[string]any{"App": app, "Deployments": deps, "Latest": latest, "Env": env, "Plan": info,
			"Services": storageRows(services, info.Limits),
			"Form":     newStackForm(app, "", "", nil), "Charts": metricCharts(metrics), "Range": rng, "Ranges": service.MetricRanges})
	}))
	mux.HandleFunc("GET /apps/{name}/deployments/{id}/log", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		d, err := w.Svc.GetDeployment(r.Context(), u, r.PathValue("name"), id)
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		log, offset, _ := w.Svc.BuildLog(r.Context(), u, r.PathValue("name"), id)
		if r.FormValue("raw") != "" {
			plainText(rw, fmt.Sprintf("%s-deployment-%d.log", r.PathValue("name"), id), log)
			return
		}
		w.render(rw, r, "log", "Deployment log", u, map[string]any{"App": r.PathValue("name"), "Deployment": d, "Log": log, "Offset": offset,
			"Full": r.Header.Get("HX-Request") == "" || r.FormValue("full") != ""})
	}))
	// Live log as server-sent events (static/app.js follows it).
	mux.HandleFunc("GET /apps/{name}/deployments/{id}/log/stream", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := api.StreamBuildLog(rw, r, w.Svc, u, r.PathValue("name"), id); err != nil {
			status, msg := api.StatusOf(err)
			http.Error(rw, msg, status)
		}
	}))
	mux.HandleFunc("GET /apps/{name}/runtime-log", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		log, err := w.Svc.RuntimeLogs(r.Context(), u, r.PathValue("name"), 200)
		if err != nil {
			log = errMsg(err)
		}
		if r.FormValue("download") != "" {
			plainText(rw, r.PathValue("name")+"-runtime.log", log)
			return
		}
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		rw.Write([]byte(log)) //nolint:errcheck
	}))

	action := func(path string, fn func(r *http.Request, u *auth.User, name string) (string, error)) {
		mux.HandleFunc("POST /apps/{name}/"+path, w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
			name := r.PathValue("name")
			msg, err := fn(r, u, name)
			if err != nil {
				redirect(rw, r, "/apps/"+name, "error", errMsg(err))
				return
			}
			if path == "delete" {
				redirect(rw, r, "/apps", "ok", msg)
				return
			}
			redirect(rw, r, "/apps/"+name, "ok", msg)
		}))
	}
	action("deploy", func(r *http.Request, u *auth.User, n string) (string, error) {
		_, err := w.Svc.Deploy(r.Context(), u, n)
		return "Build queued.", err
	})
	action("restart", func(r *http.Request, u *auth.User, n string) (string, error) {
		_, err := w.Svc.Restart(r.Context(), u, n)
		return "Restart queued.", err
	})
	action("rollback", func(r *http.Request, u *auth.User, n string) (string, error) {
		_, err := w.Svc.Rollback(r.Context(), u, n)
		return "Rollback queued.", err
	})
	action("stop", func(r *http.Request, u *auth.User, n string) (string, error) {
		return "App stopped.", w.Svc.StopApp(r.Context(), u, n)
	})
	action("start", func(r *http.Request, u *auth.User, n string) (string, error) {
		return "App started.", w.Svc.StartApp(r.Context(), u, n)
	})
	action("delete", func(r *http.Request, u *auth.User, n string) (string, error) {
		return "App " + n + " deleted.", w.Svc.DeleteApp(r.Context(), u, n, r.FormValue("confirm"))
	})
	mux.HandleFunc("GET /apps/{name}/stack-form", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		app, err := w.Svc.GetApp(r.Context(), u, r.PathValue("name"))
		if err != nil {
			http.NotFound(rw, r)
			return
		}
		w.render(rw, r, "stackform", "Stack", u, newStackForm(app, r.FormValue("language"), r.FormValue("framework"), formPtr(r, "root_dir")))
	}))
	action("stack", func(r *http.Request, u *auth.User, n string) (string, error) {
		_, err := w.Svc.ConfirmStack(r.Context(), u, n, stacks.Stack{
			Language: r.FormValue("language"), Framework: r.FormValue("framework"), Version: r.FormValue("version"),
			BuildCommand: r.FormValue("build_command"), StartCommand: r.FormValue("start_command"), PublishDir: r.FormValue("publish_dir"),
			RootDir: r.FormValue("root_dir"),
		})
		return "Stack confirmed, the build is queued.", err
	})
	action("analyse", func(r *http.Request, u *auth.User, n string) (string, error) {
		_, err := w.Svc.Analyse(r.Context(), u, n, formPtr(r, "root_dir"))
		return "Analysing the repository again.", err
	})
	action("env", func(r *http.Request, u *auth.User, n string) (string, error) {
		key := strings.TrimSpace(r.FormValue("key"))
		changes := map[string]*string{}
		if r.FormValue("delete") == "1" {
			changes[key] = nil
		} else {
			v := r.FormValue("value")
			changes[key] = &v
		}
		_, err := w.Svc.SetEnv(r.Context(), u, n, changes)
		return "Environment updated.", err
	})
	action("domains", func(r *http.Request, u *auth.User, n string) (string, error) {
		st, err := w.Svc.AddDomain(r.Context(), u, n, r.FormValue("hostname"))
		if err != nil {
			return "", err
		}
		return st.Instructions, nil
	})
	action("domains/check", func(r *http.Request, u *auth.User, n string) (string, error) {
		st, err := w.Svc.DomainStatus(r.Context(), u, n, r.FormValue("hostname"))
		if err != nil {
			return "", err
		}
		if st.Verified {
			return st.Hostname + " is verified and routed.", nil
		}
		return st.Hostname + " is not pointing to FodyCloud yet. " + st.Instructions, nil
	})
	action("health-check", func(r *http.Request, u *auth.User, n string) (string, error) {
		a, err := w.Svc.SetHealthCheck(r.Context(), u, n, r.FormValue("path"))
		if err != nil {
			return "", err
		}
		if a.HealthCheckPath == "" {
			return "Health check turned off. It applies from the next deployment.", nil
		}
		return "Health check saved: " + a.HealthCheckPath + ". It applies from the next deployment.", nil
	})
	for _, kind := range service.ServiceKinds {
		action("services/"+kind, func(r *http.Request, u *auth.User, n string) (string, error) {
			if _, err := w.Svc.AddService(r.Context(), u, n, kind); err != nil {
				return "", err
			}
			return storageNames[kind] + " added. The app restarts to use it.", nil
		})
		action("services/"+kind+"/delete", func(r *http.Request, u *auth.User, n string) (string, error) {
			if err := w.Svc.RemoveService(r.Context(), u, n, kind, r.FormValue("confirm")); err != nil {
				return "", err
			}
			return storageNames[kind] + " deleted with its data.", nil
		})
	}
	action("domains/remove", func(r *http.Request, u *auth.User, n string) (string, error) {
		host := r.FormValue("hostname")
		return host + " removed.", w.Svc.RemoveDomain(r.Context(), u, n, host)
	})
}

// accountRoutes: plan and API tokens.
func (w *Web) accountRoutes(mux *http.ServeMux) {
	account := func(rw http.ResponseWriter, r *http.Request, u *auth.User, newToken string) {
		info, _ := w.Svc.PlanInfo(r.Context(), u)
		toks, _ := w.Auth.ListTokens(r.Context(), u.ID)
		rw.Header().Set("Cache-Control", "no-store")
		w.render(rw, r, "account", "Account", u, map[string]any{"Plan": info, "Tokens": toks, "NewToken": newToken})
	}
	mux.HandleFunc("GET /account", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		account(rw, r, u, "")
	}))
	mux.HandleFunc("POST /account/tokens", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		scopes := []string{auth.ScopeRead}
		if r.FormValue("write") == "1" {
			scopes = append(scopes, auth.ScopeWrite)
		}
		tok, err := w.Auth.CreateToken(r.Context(), u.ID, r.FormValue("name"), scopes)
		if err != nil {
			redirect(rw, r, "/account", "error", err.Error())
			return
		}
		// Shown once, rendered directly (never put in a URL: it would end up in access logs and history).
		account(rw, r, u, tok)
	}))
	mux.HandleFunc("POST /account/tokens/{id}/revoke", w.authed(func(rw http.ResponseWriter, r *http.Request, u *auth.User) {
		id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err := w.Auth.RevokeToken(r.Context(), u.ID, id); err != nil {
			redirect(rw, r, "/account", "error", errMsg(err))
			return
		}
		redirect(rw, r, "/account", "ok", "Token revoked.")
	}))
	// The forms below ask for the password: wrong answers count toward the
	// account's sign-in lockout (passwordForm).
	mux.HandleFunc("POST /account/password", w.passwordForm(func(rw http.ResponseWriter, r *http.Request, u *auth.User) error {
		if r.FormValue("password") != r.FormValue("password2") {
			redirect(rw, r, "/account", "error", "The two new passwords are different.")
			return nil
		}
		c, _ := r.Cookie(w.cookieName(dashboardSession))
		if err := w.Svc.ChangePassword(r.Context(), w.Auth, u, c.Value, r.FormValue("current"), r.FormValue("password")); err != nil {
			return err
		}
		redirect(rw, r, "/account", "ok", "Password changed. Your other sessions were signed out.")
		return nil
	}))
	mux.HandleFunc("POST /account/email", w.passwordForm(func(rw http.ResponseWriter, r *http.Request, u *auth.User) error {
		email := r.FormValue("email")
		if err := w.Svc.RequestEmailChange(r.Context(), w.Auth, u, email, r.FormValue("password")); err != nil {
			return err
		}
		redirect(rw, r, "/account", "ok", "A confirmation link was sent to "+strings.ToLower(strings.TrimSpace(email))+". Your email changes when you open it.")
		return nil
	}))
	mux.HandleFunc("POST /account/delete", w.passwordForm(func(rw http.ResponseWriter, r *http.Request, u *auth.User) error {
		if _, err := w.Auth.Authenticate(r.Context(), u.Email, r.FormValue("password")); err != nil {
			return service.ErrWrongPassword
		}
		if err := w.Svc.DeleteAccount(r.Context(), w.Auth, u, r.FormValue("confirm")); err != nil {
			return err
		}
		w.clearSessionCookie(rw, dashboardSession)
		http.Redirect(rw, r, w.sites.URL(sites.Home, "/")+"?ok="+url.QueryEscape("Your account and its apps were deleted."), http.StatusSeeOther)
		return nil
	}))
}

// plainText sends a log as a file download. The name is built from a
// validated app name and a number, so it needs no escaping.
func plainText(rw http.ResponseWriter, filename, body string) {
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	rw.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	rw.Header().Set("X-Content-Type-Options", "nosniff")
	rw.Write([]byte(body)) //nolint:errcheck
}
