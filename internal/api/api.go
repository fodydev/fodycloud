// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package api exposes the JSON REST API (https://api.<domain>/v1).
// Authentication: a bearer API token.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/ratelimit"
	"github.com/fodydev/fodycloud/internal/service"
	"github.com/fodydev/fodycloud/internal/stacks"
)

// API serves /v1 on the API host.
type API struct {
	Svc  *service.Service
	Auth *auth.Store
	Log  *slog.Logger
	// TokenLimiter bounds requests per credential (nil = unlimited).
	TokenLimiter *ratelimit.Limiter
	// About is returned by GET /v1/about.
	About About
}

// About describes the running platform: its version, and where its source
// code is, which the AGPL-3.0 (section 13) requires offering to every user.
type About struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	License    string `json:"license"`
	Source     string `json:"source"`     // <domain>/source: the exact source archive
	Repository string `json:"repository"` // the public repository
}

type ctxKey struct{}

// UserFrom returns the authenticated user of a request.
func UserFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(ctxKey{}).(*auth.User)
	return u
}

// WithUser stores the user in a context.
func WithUser(ctx context.Context, u *auth.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

func bearer(r *http.Request) string {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return strings.TrimSpace(tok)
}

// errAdminToken refuses admin tokens on students' endpoints.
var errAdminToken = fmt.Errorf("%w: admin tokens only work on /v1/admin", service.ErrForbidden)

// errAdminsOnly refuses students' tokens on admin endpoints.
var errAdminsOnly = fmt.Errorf("%w: admins only", service.ErrForbidden)

// Authenticate resolves a student's bearer token (fc_). Admin tokens (fca_)
// are refused: admins are separate accounts. The API lives on its own host,
// so browser sessions (private to the dashboard host) never apply.
func (a *API) Authenticate(r *http.Request) (*auth.User, error) {
	tok := bearer(r)
	if strings.HasPrefix(tok, auth.AdminTokenPrefix) {
		return nil, errAdminToken
	}
	return a.Auth.TokenUser(r.Context(), tok)
}

// AuthenticateAdmin resolves an admin's bearer token (fca_).
func (a *API) AuthenticateAdmin(r *http.Request) (*auth.Admin, error) {
	tok := bearer(r)
	if strings.HasPrefix(tok, auth.TokenPrefix) {
		return nil, errAdminsOnly
	}
	return a.Auth.AdminTokenAdmin(r.Context(), tok)
}

// respond rate-limits per account (key), runs fn and writes its result:
// 202 for POST (work queued), 204 when there is nothing to return.
func (a *API) respond(w http.ResponseWriter, r *http.Request, key string, fn func(*http.Request) (any, error)) {
	if a.TokenLimiter != nil {
		if ok, wait := a.TokenLimiter.Allow(key); !ok {
			ratelimit.TooMany(w, wait, `{"error":"rate limit exceeded"}`)
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	out, err := fn(r)
	if errors.Is(err, errStreamed) {
		return
	}
	if err != nil {
		writeErr(w, a.Log, err)
		return
	}
	status := http.StatusOK
	if r.Method == http.MethodPost {
		status = http.StatusAccepted
	}
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, status, out)
}

// studentHandler handles a students' endpoint; the result is written as JSON.
type studentHandler func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error)

// adminHandler handles an admin endpoint (/v1/admin/…).
type adminHandler func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error)

// Handler returns the API routes.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	// h registers a students' endpoint, ah an admin endpoint.
	h := func(pattern string, fn studentHandler) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			u, err := a.Authenticate(r)
			if err != nil {
				writeErr(w, a.Log, err)
				return
			}
			a.respond(w, r, fmt.Sprint(u.ID), func(r *http.Request) (any, error) {
				return fn(w, r.WithContext(WithUser(r.Context(), u)), u)
			})
		})
	}
	ah := func(pattern string, fn adminHandler) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			ad, err := a.AuthenticateAdmin(r)
			if err != nil {
				writeErr(w, a.Log, err)
				return
			}
			a.respond(w, r, fmt.Sprintf("admin:%d", ad.ID), func(r *http.Request) (any, error) { return fn(w, r, ad) })
		})
	}

	// Public: no token, so anyone can find the source (AGPL-3.0).
	mux.HandleFunc("GET /v1/about", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, a.About) })
	a.studentRoutes(h)
	a.adminRoutes(ah)
	return mux
}

// SameOrigin reports whether a browser request comes from our own origin.
func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = r.Header.Get("Referer")
	}
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	return normalizeHost(u.Host, u.Scheme) == normalizeHost(r.Host, u.Scheme)
}

// normalizeHost lower-cases a host[:port] and drops the scheme's default port,
// so "dashboard.x:443" (some clients send it) equals "dashboard.x".
func normalizeHost(hostport, scheme string) string {
	h := strings.ToLower(hostport)
	switch {
	case scheme == "https" && strings.HasSuffix(h, ":443"):
		return strings.TrimSuffix(h, ":443")
	case scheme == "http" && strings.HasSuffix(h, ":80"):
		return strings.TrimSuffix(h, ":80")
	}
	return h
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return &service.InvalidError{Msg: "invalid JSON body: " + err.Error()}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// StatusOf maps a service error to an HTTP status and public message.
func StatusOf(err error) (int, string) {
	var inv *service.InvalidError
	var quota *service.QuotaError
	switch {
	case errors.Is(err, auth.ErrUnauthenticated):
		return http.StatusUnauthorized, "authentication required"
	case errors.Is(err, service.ErrNotFound):
		return http.StatusNotFound, "not found"
	case errors.Is(err, service.ErrForbidden):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, service.ErrTooManyStreams):
		return http.StatusTooManyRequests, err.Error()
	case errors.As(err, &inv):
		return http.StatusUnprocessableEntity, inv.Msg
	case errors.As(err, &quota):
		return http.StatusForbidden, quota.Msg
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

func writeErr(w http.ResponseWriter, log *slog.Logger, err error) {
	status, msg := StatusOf(err)
	if status == http.StatusInternalServerError {
		log.Error("request failed", "err", err)
	}
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Bearer realm="fodycloud"`)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

// studentRoutes registers the students' endpoints (fc_ tokens).
func (a *API) studentRoutes(h func(string, studentHandler)) {
	h("GET /v1/me", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		info, err := a.Svc.PlanInfo(r.Context(), u)
		if err != nil {
			return nil, err
		}
		return map[string]any{"email": u.Email, "scopes": u.Scopes, "plan": info}, nil
	})
	h("DELETE /v1/me", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return nil, a.Svc.DeleteAccount(r.Context(), a.Auth, u, r.URL.Query().Get("confirm"))
	})
	h("GET /v1/apps", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		apps, err := a.Svc.ListApps(r.Context(), u)
		if apps == nil {
			apps = []*service.App{}
		}
		return map[string]any{"apps": apps}, err
	})
	h("POST /v1/apps", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		var in service.NewApp
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		app, d, err := a.Svc.CreateApp(r.Context(), u, in)
		if err != nil {
			return nil, err
		}
		return map[string]any{"app": app, "deployment": d}, nil
	})
	h("GET /v1/apps/{name}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.GetApp(r.Context(), u, r.PathValue("name"))
	})
	h("DELETE /v1/apps/{name}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return nil, a.Svc.DeleteApp(r.Context(), u, r.PathValue("name"), r.URL.Query().Get("confirm"))
	})
	h("POST /v1/apps/{name}/deploy", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.Deploy(r.Context(), u, r.PathValue("name"))
	})
	h("GET /v1/stacks", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return map[string]any{"languages": stacks.Catalog}, nil
	})
	h("PUT /v1/apps/{name}/stack", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		var in stacks.Stack
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		d, err := a.Svc.ConfirmStack(r.Context(), u, r.PathValue("name"), in)
		if err != nil {
			return nil, err
		}
		app, err := a.Svc.GetApp(r.Context(), u, r.PathValue("name"))
		return map[string]any{"stack": app.Stack, "deployment": d}, err
	})
	h("POST /v1/apps/{name}/analyse", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		// Optional body {"root_dir": "frontend"}: the folder to analyse.
		var in struct {
			RootDir *string `json:"root_dir"`
		}
		if r.ContentLength != 0 {
			if err := decode(r, &in); err != nil {
				return nil, err
			}
		}
		return a.Svc.Analyse(r.Context(), u, r.PathValue("name"), in.RootDir)
	})
	h("POST /v1/apps/{name}/restart", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.Restart(r.Context(), u, r.PathValue("name"))
	})
	h("POST /v1/apps/{name}/rollback", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.Rollback(r.Context(), u, r.PathValue("name"))
	})
	h("POST /v1/apps/{name}/stop", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return map[string]string{"status": "stopped"}, a.Svc.StopApp(r.Context(), u, r.PathValue("name"))
	})
	h("POST /v1/apps/{name}/start", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return map[string]string{"status": "starting"}, a.Svc.StartApp(r.Context(), u, r.PathValue("name"))
	})
	h("GET /v1/apps/{name}/deployments", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		ds, err := a.Svc.ListDeployments(r.Context(), u, r.PathValue("name"), 20)
		return map[string]any{"deployments": ds}, err
	})
	h("GET /v1/apps/{name}/deployments/{id}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			return nil, service.ErrNotFound
		}
		return a.Svc.GetDeployment(r.Context(), u, r.PathValue("name"), id)
	})
	h("GET /v1/apps/{name}/deployments/{id}/log", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			return nil, service.ErrNotFound
		}
		if WantsStream(r) {
			if err := StreamBuildLog(w, r, a.Svc, u, r.PathValue("name"), id); err != nil {
				return nil, err
			}
			return nil, errStreamed
		}
		log, _, err := a.Svc.BuildLog(r.Context(), u, r.PathValue("name"), id)
		return map[string]string{"log": log}, err
	})
	h("GET /v1/apps/{name}/metrics", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.AppMetrics(r.Context(), u, r.PathValue("name"), r.URL.Query().Get("range"))
	})
	h("GET /v1/apps/{name}/logs", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
		log, err := a.Svc.RuntimeLogs(r.Context(), u, r.PathValue("name"), tail)
		return map[string]string{"log": log}, err
	})
	h("GET /v1/apps/{name}/env", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		env, err := a.Svc.ListEnv(r.Context(), u, r.PathValue("name"), r.URL.Query().Get("reveal") == "true" && u.Can(auth.ScopeWrite))
		return map[string]any{"env": env}, err
	})
	h("PATCH /v1/apps/{name}/env", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		var in map[string]*string
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		d, err := a.Svc.SetEnv(r.Context(), u, r.PathValue("name"), in)
		return map[string]any{"deployment": d}, err
	})
	h("POST /v1/apps/{name}/domains", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		var in struct {
			Hostname string `json:"hostname"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.Svc.AddDomain(r.Context(), u, r.PathValue("name"), in.Hostname)
	})
	h("GET /v1/apps/{name}/domains/{host}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.DomainStatus(r.Context(), u, r.PathValue("name"), r.PathValue("host"))
	})
	h("GET /v1/apps/{name}/services", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		svcs, err := a.Svc.Services(r.Context(), u, r.PathValue("name"))
		return map[string]any{"services": svcs}, err
	})
	h("POST /v1/apps/{name}/services/{kind}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return a.Svc.AddService(r.Context(), u, r.PathValue("name"), r.PathValue("kind"))
	})
	h("DELETE /v1/apps/{name}/services/{kind}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return nil, a.Svc.RemoveService(r.Context(), u, r.PathValue("name"), r.PathValue("kind"), r.URL.Query().Get("confirm"))
	})
	h("PUT /v1/apps/{name}/health-check", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		var in struct {
			Path string `json:"path"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.Svc.SetHealthCheck(r.Context(), u, r.PathValue("name"), in.Path)
	})
	h("DELETE /v1/apps/{name}/domains/{host}", func(w http.ResponseWriter, r *http.Request, u *auth.User) (any, error) {
		return nil, a.Svc.RemoveDomain(r.Context(), u, r.PathValue("name"), r.PathValue("host"))
	})
	// Admin: plan assignment (no online payment yet).
}

// adminRoutes registers the admin endpoints (fca_ tokens).
func (a *API) adminRoutes(ah func(string, adminHandler)) {
	ah("GET /v1/admin/users", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		users, err := a.Svc.ListUsers(r.Context(), ad)
		return map[string]any{"users": users}, err
	})
	ah("POST /v1/admin/users/{email}/ban", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		var in struct {
			Reason string `json:"reason"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return map[string]string{"banned": r.PathValue("email")}, a.Svc.Ban(r.Context(), ad, a.Auth, r.PathValue("email"), in.Reason)
	})
	ah("DELETE /v1/admin/users/{email}/ban", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		return nil, a.Svc.Unban(r.Context(), ad, r.PathValue("email"))
	})
	ah("GET /v1/admin/banned-repos", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		repos, err := a.Svc.ListBannedRepos(r.Context(), ad)
		return map[string]any{"repos": repos}, err
	})
	ah("POST /v1/admin/banned-repos", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		var in struct {
			RepoURL string `json:"repo_url"`
			Reason  string `json:"reason"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		url, err := a.Svc.BanRepo(r.Context(), ad, in.RepoURL, in.Reason)
		return map[string]string{"banned": url}, err
	})
	ah("GET /v1/admin/abuse", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		flags, err := a.Svc.ListAbuseFlags(r.Context(), ad)
		return map[string]any{"flags": flags}, err
	})
	ah("PUT /v1/admin/users/{email}/plan", func(w http.ResponseWriter, r *http.Request, ad *auth.Admin) (any, error) {
		var in struct {
			Plan       string     `json:"plan"`
			ValidUntil *time.Time `json:"valid_until"`
			Note       string     `json:"note"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.Svc.SetPlan(r.Context(), ad, r.PathValue("email"), in.Plan, in.ValidUntil, in.Note)
	})
}
