// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package web serves the server-rendered dashboard (html/template + htmx).
package web

import (
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fodydev/fodycloud/internal/api"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/config"
	"github.com/fodydev/fodycloud/internal/ratelimit"
	"github.com/fodydev/fodycloud/internal/service"
	"github.com/fodydev/fodycloud/internal/sites"
	"github.com/fodydev/fodycloud/internal/stacks"
)

//go:embed templates/*.html
var templateFS embed.FS

// Web serves the dashboard.
type Web struct {
	Svc           *service.Service
	Auth          *auth.Store
	API           *api.API
	Log           *slog.Logger
	BaseDomain    string
	SecureCookies bool
	pages         map[string]*template.Template
	docs          map[string]*docPage // /docs on the home site and the dashboard
	adminDocs     map[string]*docPage // admin.<domain>/docs
	legalDocs     map[string]*docPage // <domain>/legal
	// Operator names who runs the platform, in the legal policies.
	Operator config.Operator
	sites    sites.Sites

	// The running version and where its source is (<domain>/source).
	Version       string
	SourceURL     string // the public repository
	SourceArchive string // path of the exact source archive ("" or missing = none)

	// Abuse protection (nil disables a limit).
	LoginLimiter  *ratelimit.Limiter // per client IP
	SignupLimiter *ratelimit.Limiter // per client IP (also used for password-reset requests)
	WakeLimiter   *ratelimit.Limiter // per client IP
	Lockout       *ratelimit.Lockout // per account, after failed logins
}

// limited applies an optional per-IP limiter; it writes a 429 when refused.
func limited(l *ratelimit.Limiter, rw http.ResponseWriter, r *http.Request) bool {
	if l == nil {
		return false
	}
	if ok, wait := l.Allow(ratelimit.ClientIP(r)); !ok {
		ratelimit.TooMany(rw, wait, "Too many requests, please slow down.")
		return true
	}
	return false
}

var funcs = template.FuncMap{
	"asset": func(name string) string { return "/static/" + name + "?v=" + assetVersions[name] },
	"dict": func(kv ...any) (map[string]any, error) {
		if len(kv)%2 != 0 {
			return nil, errors.New("dict needs key/value pairs")
		}
		m := make(map[string]any, len(kv)/2)
		for i := 0; i < len(kv); i += 2 {
			k, ok := kv[i].(string)
			if !ok {
				return nil, errors.New("dict keys must be strings")
			}
			m[k] = kv[i+1]
		}
		return m, nil
	},
	"date": func(t any) string {
		switch v := t.(type) {
		case time.Time:
			return v.Format("2006-01-02 15:04")
		case *time.Time:
			if v == nil {
				return "never"
			}
			return v.Format("2006-01-02")
		}
		return ""
	},
	"short": func(s string) string {
		if len(s) > 8 {
			return s[:8]
		}
		return s
	},
	"bytes":    service.HumanBytes,
	"duration": func(secs int64) string { return service.FormatDuration(time.Duration(secs) * time.Second) },
	"active": func(status string) bool {
		return status == "queued" || status == "building" || status == "deploying"
	},
	"stacklabel": func(s stacks.Stack) string {
		l, p, ok := stacks.Find(s.Language, s.Framework)
		switch {
		case !ok:
			return s.Language + " / " + s.Framework
		case l.ID == "static":
			return l.Label
		}
		return l.Label + " · " + p.Label
	},
	"join": strings.Join,
}

// Init parses the templates.
func (w *Web) Init() error {
	w.sites = sites.For(w.BaseDomain)
	if err := w.initDocs(); err != nil {
		return err
	}
	w.pages = map[string]*template.Template{}
	entries, err := fs.Glob(templateFS, "templates/*.html")
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := strings.TrimSuffix(strings.TrimPrefix(e, "templates/"), ".html")
		if name == "layout" || strings.HasPrefix(name, "_") { // _*.html: partials shared by every page
			continue
		}
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/_*.html", e)
		if err != nil {
			return err
		}
		w.pages[name] = t
	}
	return nil
}

type page struct {
	Title      string
	User       *auth.User  // the signed-in student (dashboard pages)
	Admin      *auth.Admin // the signed-in admin (admin site pages)
	BaseDomain string
	OK, Err    string
	Data       any
	// NeedsVerification shows the "confirm your email" banner.
	NeedsVerification bool
	// Nav is the current page (template name), used to highlight the menu.
	Nav string
	// Site is the site serving the page ("admin" switches the menu).
	Site  string
	sites sites.Sites
}

// URL is the absolute address of a path on a site, for links between the
// platform's sites: {{.URL "docs" "/user/stacks"}}. Signed-in students read
// the guides inside the dashboard; everyone else on the home site.
func (p page) URL(site, path string) string {
	s := sites.ByName(site)
	if s == sites.Docs && p.User != nil && p.Site != "admin" {
		return p.sites.URL(sites.Dashboard, strings.TrimSuffix("/docs"+path, "/"))
	}
	return p.sites.URL(s, path)
}

// renderAdmin renders an admin-site page for a signed-in admin.
func (w *Web) renderAdmin(rw http.ResponseWriter, r *http.Request, name, title string, ad *auth.Admin, data any) {
	w.renderPage(rw, r, name, title, nil, ad, data)
}

func (w *Web) render(rw http.ResponseWriter, r *http.Request, name, title string, u *auth.User, data any) {
	w.renderPage(rw, r, name, title, u, nil, data)
}

func (w *Web) renderPage(rw http.ResponseWriter, r *http.Request, name, title string, u *auth.User, ad *auth.Admin, data any) {
	p := page{Admin: ad, Title: title, User: u, BaseDomain: w.BaseDomain, OK: r.URL.Query().Get("ok"), Err: r.URL.Query().Get("error"), Data: data,
		NeedsVerification: u != nil && !u.EmailVerified && w.Svc != nil && w.Svc.RequireVerification, Nav: name, sites: w.sites}
	if w.sites.Of(r.Host) == sites.Admin {
		p.Site = "admin"
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl := w.pages[name]
	target := "layout.html"
	if r.Header.Get("HX-Request") == "true" && tmpl.Lookup("partial") != nil {
		target = "partial"
	}
	if err := tmpl.ExecuteTemplate(rw, target, p); err != nil {
		w.Log.Error("render", "page", name, "err", err)
	}
}

func redirect(rw http.ResponseWriter, r *http.Request, path, key, msg string) {
	if msg != "" {
		path += "?" + key + "=" + url.QueryEscape(msg)
	}
	http.Redirect(rw, r, path, http.StatusSeeOther)
}

func errMsg(err error) string {
	_, msg := api.StatusOf(err)
	return msg
}
