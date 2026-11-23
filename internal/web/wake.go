// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/fodydev/fodycloud/internal/service"
)

// appHostHandler answers <app>.apps.<domain> when no app container serves
// it (Traefik's low-priority catch-all): the wake page and its assets.
func (w *Web) appHostHandler() http.Handler {
	static := w.staticHandler()
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/static/") {
			static.ServeHTTP(rw, r)
			return
		}
		w.ServeWake(rw, r)
	})
}

// ServeWake handles requests for <app>.apps.<domain> that reach the control
// plane: Traefik routes them here (low-priority catch-all) only when no app
// container serves the host, i.e. the app is sleeping, stopped or unknown.
func (w *Web) ServeWake(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Cache-Control", "no-store")
	if limited(w.WakeLimiter, rw, r) {
		return
	}
	host := strings.ToLower(r.Host)
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	name, _, _ := strings.Cut(host, ".")
	status, err := w.Svc.WakeApp(r.Context(), name)
	data := map[string]string{"App": name, "Status": status}
	switch {
	case errors.Is(err, service.ErrNotFound):
		rw.WriteHeader(http.StatusNotFound)
		w.renderBare(rw, "wake", "No app here", data)
	case err != nil:
		w.Log.Error("wake app", "app", name, "err", err)
		rw.WriteHeader(http.StatusServiceUnavailable)
		w.renderBare(rw, "wake", "Try again", data)
	case status == "waking" || status == "running" || status == "building":
		// 503 + Retry-After tells clients and crawlers the app is coming back.
		rw.Header().Set("Retry-After", "3")
		rw.WriteHeader(http.StatusServiceUnavailable)
		data["Status"] = "waking"
		w.renderBare(rw, "wake", "Waking up "+name, data)
	default: // stopped or failed
		rw.WriteHeader(http.StatusServiceUnavailable)
		w.renderBare(rw, "wake", name+" is offline", data)
	}
}

func (w *Web) renderBare(rw http.ResponseWriter, name, title string, data any) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := w.pages[name].ExecuteTemplate(rw, "layout.html", page{Title: title, BaseDomain: w.BaseDomain, Data: data, sites: w.sites}); err != nil {
		w.Log.Error("render", "page", name, "err", err)
	}
}
