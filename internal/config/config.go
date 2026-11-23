// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package config loads the control-plane configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds every setting of the control plane.
type Config struct {
	ListenAddr  string // HTTP listen address of the dashboard/API/MCP server
	DatabaseURL string
	BaseDomain  string // e.g. fodycloud.example
	PublicURL   string // e.g. https://dashboard.fodycloud.example

	// Traefik certificate resolvers ("" means: no ACME, self-signed).
	AppsCertResolver   string // for <app>.apps.<base>
	CustomCertResolver string // for student custom domains (HTTP-01)

	PlansFile        string
	AppsNetwork      string // shared student-app network
	BuildNetwork     string // network of the buildx builder container
	BuilderName      string
	BuilderCPUs      float64
	BuilderMemory    string
	RailpackFrontend string
	WorkDir          string // scratch space for clones
	BuildConcurrency int
	BuildTimeout     time.Duration
	MaxRepoSizeMB    int64
	DiskQuota        bool   // whether --storage-opt size= is supported on this host
	AppPort          int    // port student apps must listen on ($PORT)
	ProbeImage       string // runs the HTTP health checks of apps (busybox)
	ValkeyImage      string // the apps' caches
	// The apps' PostgreSQL server (empty AppsDBURL = no managed databases):
	// the superuser connection, and the address apps connect to.
	AppsDBURL     string
	AppsDBHost    string
	SecureCookies bool
	PublicIP      string // shown in custom-domain DNS instructions

	// Email (empty SMTPHost = no delivery; verification is then not required).
	SMTPHost, SMTPUsername, SMTPPassword, MailFrom string
	SMTPPort                                       int
	RequireVerification                            bool

	// Monitoring.
	TraefikMetricsURL string // internal Prometheus endpoint, used for idle sleep
	AbuseCPUMinutes   int    // consecutive minutes at the CPU limit before a flag
	SignupPerHour     int    // signups and password-reset requests per client IP

	// Abandoned apps: the grace period between the warning email and the
	// stop, and how often the rule is checked (plans set the inactivity delay).
	AbandonGrace         time.Duration
	AbandonCheckInterval time.Duration

	// The platform's operator, named in the legal policies.
	Operator Operator

	// Where users get the platform's source code (AGPL-3.0 section 13): the
	// public repository, and the archive of the exact source the image was
	// built from (absent in development builds).
	SourceURL     string
	SourceArchive string
}

// Load reads the configuration from environment variables.
func Load() (*Config, error) {
	c := &Config{
		ListenAddr:         env("FC_LISTEN_ADDR", ":8080"),
		DatabaseURL:        os.Getenv("FC_DATABASE_URL"),
		BaseDomain:         os.Getenv("FC_BASE_DOMAIN"),
		AppsCertResolver:   os.Getenv("FC_APPS_CERT_RESOLVER"),
		CustomCertResolver: os.Getenv("FC_CUSTOM_CERT_RESOLVER"),
		PlansFile:          env("FC_PLANS_FILE", "/etc/fodycloud/plans.json"),
		AppsNetwork:        env("FC_APPS_NETWORK", "fodycloud-apps"),
		BuildNetwork:       env("FC_BUILD_NETWORK", "fodycloud-build"),
		BuilderName:        env("FC_BUILDER_NAME", "fodycloud-builder"),
		BuilderMemory:      env("FC_BUILDER_MEMORY", "4g"),
		RailpackFrontend:   env("FC_RAILPACK_FRONTEND", "ghcr.io/railwayapp/railpack-frontend:v0.43.0"),
		WorkDir:            env("FC_WORK_DIR", "/var/lib/fodycloud/work"),
		PublicIP:           os.Getenv("FC_PUBLIC_IP"),
	}
	var err error
	if c.BuilderCPUs, err = strconv.ParseFloat(env("FC_BUILDER_CPUS", "2"), 64); err != nil {
		return nil, fmt.Errorf("FC_BUILDER_CPUS: %w", err)
	}
	if c.BuildConcurrency, err = strconv.Atoi(env("FC_BUILD_CONCURRENCY", "2")); err != nil {
		return nil, fmt.Errorf("FC_BUILD_CONCURRENCY: %w", err)
	}
	if c.BuildTimeout, err = time.ParseDuration(env("FC_BUILD_TIMEOUT", "15m")); err != nil {
		return nil, fmt.Errorf("FC_BUILD_TIMEOUT: %w", err)
	}
	if c.MaxRepoSizeMB, err = strconv.ParseInt(env("FC_MAX_REPO_SIZE_MB", "500"), 10, 64); err != nil {
		return nil, fmt.Errorf("FC_MAX_REPO_SIZE_MB: %w", err)
	}
	if c.AppPort, err = strconv.Atoi(env("FC_APP_PORT", "8080")); err != nil {
		return nil, fmt.Errorf("FC_APP_PORT: %w", err)
	}
	c.ProbeImage = env("FC_PROBE_IMAGE", "busybox:1.37.0")
	c.ValkeyImage = env("FC_VALKEY_IMAGE", "valkey/valkey:8.1-alpine")
	c.AppsDBURL, c.AppsDBHost = os.Getenv("FC_APPS_DB_URL"), os.Getenv("FC_APPS_DB_HOST")
	c.SMTPHost, c.SMTPUsername, c.SMTPPassword = os.Getenv("FC_SMTP_HOST"), os.Getenv("FC_SMTP_USERNAME"), os.Getenv("FC_SMTP_PASSWORD")
	if c.SMTPPort, err = strconv.Atoi(env("FC_SMTP_PORT", "587")); err != nil {
		return nil, fmt.Errorf("FC_SMTP_PORT: %w", err)
	}
	c.RequireVerification = c.SMTPHost != "" && env("FC_REQUIRE_EMAIL_VERIFICATION", "true") == "true"
	c.TraefikMetricsURL = env("FC_TRAEFIK_METRICS_URL", "http://traefik:8082/metrics")
	if c.AbuseCPUMinutes, err = strconv.Atoi(env("FC_ABUSE_CPU_MINUTES", "30")); err != nil {
		return nil, fmt.Errorf("FC_ABUSE_CPU_MINUTES: %w", err)
	}
	if c.SignupPerHour, err = strconv.Atoi(env("FC_SIGNUP_LIMIT_PER_HOUR", "5")); err != nil {
		return nil, fmt.Errorf("FC_SIGNUP_LIMIT_PER_HOUR: %w", err)
	}
	graceDays, err := strconv.ParseFloat(env("FC_ABANDON_GRACE_DAYS", "7"), 64)
	if err != nil {
		return nil, fmt.Errorf("FC_ABANDON_GRACE_DAYS: %w", err)
	}
	c.AbandonGrace = time.Duration(graceDays * 24 * float64(time.Hour))
	if c.AbandonCheckInterval, err = time.ParseDuration(env("FC_ABANDON_CHECK_INTERVAL", "1h")); err != nil {
		return nil, fmt.Errorf("FC_ABANDON_CHECK_INTERVAL: %w", err)
	}
	c.Operator = Operator{
		Name:         env("FC_OPERATOR_NAME", "the operator of this FodyCloud platform"),
		Country:      env("FC_OPERATOR_COUNTRY", "the operator's country"),
		Email:        env("FC_OPERATOR_EMAIL", "the operator's contact address"),
		DataLocation: env("FC_OPERATOR_DATA_LOCATION", "the operator's servers"),
	}
	c.SourceURL = env("FC_SOURCE_URL", "https://github.com/fodydev/fodycloud")
	c.SourceArchive = env("FC_SOURCE_ARCHIVE", "/usr/share/fodycloud/source.tar.gz")
	c.DiskQuota = env("FC_DISK_QUOTA", "false") == "true"
	c.SecureCookies = env("FC_SECURE_COOKIES", "true") == "true"
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("FC_DATABASE_URL is required")
	}
	if c.BaseDomain == "" {
		return nil, fmt.Errorf("FC_BASE_DOMAIN is required")
	}
	c.PublicURL = env("FC_PUBLIC_URL", "https://dashboard."+c.BaseDomain)
	c.MailFrom = env("FC_MAIL_FROM", "FodyCloud <no-reply@"+c.BaseDomain+">")
	return c, nil
}

// Operator identifies who runs the platform, for the legal policies.
type Operator struct {
	Name         string // legal name, e.g. "FodyDev SARL"
	Country      string // governing law, e.g. "Cameroon"
	Email        string // contact for privacy and abuse
	DataLocation string // where data is stored, e.g. "Cameroon (VPS at …)"
}

// AppsDomain is the parent domain of free student subdomains.
func (c *Config) AppsDomain() string { return "apps." + c.BaseDomain }

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
