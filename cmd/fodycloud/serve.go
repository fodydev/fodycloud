// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/fodydev/fodycloud/internal/api"
	"github.com/fodydev/fodycloud/internal/builder"
	"github.com/fodydev/fodycloud/internal/mcpserver"
	"github.com/fodydev/fodycloud/internal/monitor"
	"github.com/fodydev/fodycloud/internal/oauth"
	"github.com/fodydev/fodycloud/internal/ratelimit"
	"github.com/fodydev/fodycloud/internal/sites"
	"github.com/fodydev/fodycloud/internal/web"
	"github.com/fodydev/fodycloud/internal/worker"
)

func serve(ctx context.Context, log *slog.Logger) error {
	return withApp(ctx, log, func(a *app) error {
		if err := os.MkdirAll(a.cfg.WorkDir, 0o700); err != nil {
			return err
		}
		b := &builder.Builder{
			Docker: a.dock, Git: a.git, Name: a.cfg.BuilderName, Network: a.cfg.BuildNetwork,
			CPUs: a.cfg.BuilderCPUs, Memory: a.cfg.BuilderMemory, Frontend: a.cfg.RailpackFrontend,
			WorkDir: a.cfg.WorkDir, MaxRepoSizeMB: a.cfg.MaxRepoSizeMB,
		}
		w := &worker.Worker{Svc: a.svc, Builder: b, Deployer: a.dep, Concurrency: a.cfg.BuildConcurrency,
			BuildTimeout: a.cfg.BuildTimeout, Log: log}
		// The builder's first start pulls the BuildKit image, which can take
		// minutes on a slow link: the sites must not wait for it (the
		// container's health check would fail), only the builds.
		go func() {
			if err := b.EnsureBuilder(ctx); err != nil {
				log.Error("buildx builder not ready (builds will fail until it is)", "err", err)
			}
			w.Run(ctx)
		}()
		go w.RunExpiry(ctx, 10*time.Minute)
		go a.mail.Run(ctx)
		mon := &monitor.Monitor{Svc: a.svc, Docker: a.dock, Log: log, MetricsURL: a.cfg.TraefikMetricsURL, CPUMinutes: a.cfg.AbuseCPUMinutes,
			AbandonInterval: a.cfg.AbandonCheckInterval}
		go mon.Run(ctx)
		log.Info("email", "delivery", a.mail.Enabled(), "verification_required", a.cfg.RequireVerification)

		tokenLimiter := ratelimit.New(120, time.Minute) // per user, shared by the API and MCP
		// The source of this version (AGPL-3.0): <domain>/source, /v1/about, MCP server info.
		sourcePage := sites.For(a.cfg.BaseDomain).URL(sites.Home, "/source")
		apiH := &api.API{Svc: a.svc, Auth: a.auth, Log: log, TokenLimiter: tokenLimiter, About: api.About{
			Name: "FodyCloud", Version: Version, License: "AGPL-3.0-or-later", Source: sourcePage, Repository: a.cfg.SourceURL}}
		forges, err := oauth.NewRegistry(a.cfg.OAuth)
		if err != nil {
			return err
		}
		for _, p := range forges.List() {
			log.Info("sign-in with a forge enabled", "provider", p.Name, "issuer", p.Issuer)
		}
		webH := &web.Web{Svc: a.svc, Auth: a.auth, API: apiH, Forges: forges, Log: log, BaseDomain: a.cfg.BaseDomain, SecureCookies: a.cfg.SecureCookies,
			Operator: a.cfg.Operator, Version: Version, SourceURL: a.cfg.SourceURL, SourceArchive: a.cfg.SourceArchive,
			LoginLimiter: ratelimit.New(10, time.Minute), SignupLimiter: ratelimit.New(a.cfg.SignupPerHour, time.Hour),
			WakeLimiter: ratelimit.New(30, time.Minute), Lockout: ratelimit.NewLockout(5, 15*time.Minute)}
		if err := webH.Init(); err != nil {
			return err
		}
		mcpH := &mcpserver.Server{Svc: a.svc, Auth: a.auth, Log: log, Version: Version, SourcePage: sourcePage, TokenLimiter: tokenLimiter}

		// Each host has one role (internal/sites): home, dashboard, admin, docs,
		// api, mcp, and <app>.apps.<domain> when no app container serves it.
		sitesH := webH.Root(apiH.Handler(), mcpH.Handler())
		root := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				if err := a.pool.Ping(r.Context()); err != nil {
					http.Error(w, "database unavailable", http.StatusServiceUnavailable)
					return
				}
				w.Write([]byte("ok")) //nolint:errcheck
				return
			}
			sitesH.ServeHTTP(w, r)
		})
		srv := &http.Server{
			Addr: a.cfg.ListenAddr, Handler: securityHeaders(root),
			ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second,
		}
		srv.RegisterOnShutdown(api.StopStreams) // live log streams would hold the shutdown
		go func() {
			<-ctx.Done()
			shut, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			srv.Shutdown(shut) //nolint:errcheck
		}()
		log.Info("fodycloud control plane listening", "addr", a.cfg.ListenAddr, "version", Version, "domain", a.cfg.BaseDomain)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
}

func securityHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'")
		h.ServeHTTP(w, r)
	})
}
