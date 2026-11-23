// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fodydev/fodycloud/internal/appsdb"
	"github.com/fodydev/fodycloud/internal/auth"
	"github.com/fodydev/fodycloud/internal/config"
	"github.com/fodydev/fodycloud/internal/db"
	"github.com/fodydev/fodycloud/internal/deployer"
	"github.com/fodydev/fodycloud/internal/dockercli"
	"github.com/fodydev/fodycloud/internal/mail"
	"github.com/fodydev/fodycloud/internal/plans"
	"github.com/fodydev/fodycloud/internal/ratelimit"
	"github.com/fodydev/fodycloud/internal/repo"
	"github.com/fodydev/fodycloud/internal/service"
)

type app struct {
	cfg  *config.Config
	pool *pgxpool.Pool
	auth *auth.Store
	mail *mail.Mailer
	svc  *service.Service
	dep  *deployer.Deployer
	dock *dockercli.Docker
	git  *repo.Git
}

// withApp loads configuration, connects to the database, applies migrations
// and wires the service.
func withApp(ctx context.Context, log *slog.Logger, fn func(*app) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	catalog, err := plans.Load(cfg.PlansFile)
	if err != nil {
		return err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	dock := &dockercli.Docker{}
	dep := &deployer.Deployer{
		Docker: dock, Network: cfg.AppsNetwork, AppsDomain: cfg.AppsDomain(),
		AppsCertResolver: cfg.AppsCertResolver, CustomCertResolver: cfg.CustomCertResolver,
		AppPort: cfg.AppPort, DiskQuota: cfg.DiskQuota, ProbeImage: cfg.ProbeImage, ValkeyImage: cfg.ValkeyImage,
	}
	var appsDB *appsdb.Server
	if cfg.AppsDBURL != "" {
		appsDB = &appsdb.Server{AdminURL: cfg.AppsDBURL, Host: cfg.AppsDBHost, Port: 5432}
	}
	git := &repo.Git{Timeout: 30 * time.Second}
	mailer := &mail.Mailer{DB: pool, Log: log, Cfg: mail.Config{
		Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername, Password: cfg.SMTPPassword, From: cfg.MailFrom}}
	a := &app{
		cfg: cfg, pool: pool, dock: dock, dep: dep, git: git, mail: mailer,
		auth: &auth.Store{DB: pool},
		svc: &service.Service{AbandonGrace: cfg.AbandonGrace, DB: pool, Plans: catalog, Git: git, Deployer: dep, AppsDB: appsDB, Log: log, PublicIP: cfg.PublicIP,
			Mail: mailer, PublicURL: cfg.PublicURL, RequireVerification: cfg.RequireVerification,
			BuildLimiter: ratelimit.New(20, time.Hour)},
	}
	return fn(a)
}
