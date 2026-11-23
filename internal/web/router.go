// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package web

import (
	"net/http"

	"github.com/fodydev/fodycloud/internal/sites"
)

const (
	dashboardSession sessionSite = iota
	adminSession
)

// Root dispatches each request to the site its host names. apiH serves /v1
// on the API host, mcpH serves /mcp on the MCP host.
func (w *Web) Root(apiH, mcpH http.Handler) http.Handler {
	byHost := map[sites.Site]http.Handler{
		sites.Home:      w.homeHandler(),
		sites.Dashboard: w.dashboardHandler(),
		sites.Admin:     w.adminHandler(),
		sites.API:       w.apiHandler(apiH),
		sites.MCP:       w.mcpHandler(mcpH),
		sites.App:       w.appHostHandler(),
	}
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if h, ok := byHost[w.sites.Of(r.Host)]; ok {
			h.ServeHTTP(rw, r)
			return
		}
		http.Error(rw, "unknown host", http.StatusNotFound)
	})
}

// homeHandler serves the public home page on the apex domain.
func (w *Web) homeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", w.staticHandler())
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(hintCookie); err == nil && c.Value == "1" {
			http.Redirect(rw, r, w.sites.URL(sites.Dashboard, "/apps"), http.StatusSeeOther)
			return
		}
		w.render(rw, r, "home", "Your code. Your deployment. Your cloud.", nil, nil)
	})
	// The guides, public; signed-in visitors read them in the dashboard.
	w.docsRoutes(mux, func(rw http.ResponseWriter, r *http.Request, slug string) {
		if c, err := r.Cookie(hintCookie); err == nil && c.Value == "1" {
			http.Redirect(rw, r, w.sites.URL(sites.Dashboard, r.URL.Path), http.StatusSeeOther)
			return
		}
		w.serveDocs(rw, r, publicDocs(), w.docs, slug, nil)
	})
	mux.HandleFunc("GET /legal/{page}", func(rw http.ResponseWriter, r *http.Request) {
		w.serveLegal(rw, r, "legal/"+r.PathValue("page"))
	})
	w.sourceRoutes(mux)
	mux.HandleFunc("/", w.notFound)
	return mux
}

func (w *Web) notFound(rw http.ResponseWriter, r *http.Request) {
	rw.WriteHeader(http.StatusNotFound)
	w.render(rw, r, "notfound", "Page not found", nil, nil)
}

// docsRoutes registers the public guides under /docs: the index, the user
// and developer pages, and a docs-styled 404 for anything else.
func (w *Web) docsRoutes(mux *http.ServeMux, serve func(rw http.ResponseWriter, r *http.Request, slug string)) {
	mux.HandleFunc("GET /docs", func(rw http.ResponseWriter, r *http.Request) { serve(rw, r, "") })
	mux.HandleFunc("GET /docs/{$}", func(rw http.ResponseWriter, r *http.Request) { serve(rw, r, "") })
	for _, section := range []string{"user", "developer"} {
		mux.HandleFunc("GET /docs/"+section+"/{page}", func(rw http.ResponseWriter, r *http.Request) {
			serve(rw, r, section+"/"+r.PathValue("page"))
		})
	}
	mux.HandleFunc("GET /docs/", func(rw http.ResponseWriter, r *http.Request) { serve(rw, r, "-") })
}

// apiHandler serves the API under /v1 and its home page on api.<domain>.
func (w *Web) apiHandler(apiH http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", w.staticHandler())
	mux.Handle("/v1/", apiH)
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		w.render(rw, r, "apihome", "API", nil, nil)
	})
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusNotFound)
		rw.Write([]byte(`{"error":"not found"}` + "\n")) //nolint:errcheck
	})
	return mux
}

// mcpHandler serves the MCP endpoint and its home page on mcp.<domain>.
func (w *Web) mcpHandler(mcpH http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", w.staticHandler())
	mux.Handle("/mcp", mcpH)
	mux.HandleFunc("GET /{$}", func(rw http.ResponseWriter, r *http.Request) {
		w.render(rw, r, "mcphome", "MCP server", nil, nil)
	})
	mux.HandleFunc("/", w.notFound)
	return mux
}
