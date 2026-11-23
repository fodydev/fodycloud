-- SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
-- SPDX-License-Identifier: AGPL-3.0-or-later

-- FodyCloud schema. Pre-production: edit this file directly and recreate
-- the database. Once FodyCloud runs in production, each change goes in a new
-- numbered file (0002_…) and applied files are never edited.

-- ---------------------------------------------------------------------------
-- Students
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id                BIGSERIAL PRIMARY KEY,
    email             TEXT NOT NULL UNIQUE,
    password_hash     TEXT NOT NULL,
    email_verified_at TIMESTAMPTZ,
    banned_at         TIMESTAMPTZ,
    ban_reason        TEXT NOT NULL DEFAULT '',
    -- Acceptance of the terms of service and acceptable-use policy at
    -- sign-up (NULL for accounts created through the admin CLI).
    terms_accepted_at TIMESTAMPTZ,
    terms_version     TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Dashboard sessions; only hashes are stored.
CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions(user_id);

-- Students' API tokens (fc_…); only hashes are stored.
CREATE TABLE api_tokens (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    prefix       TEXT NOT NULL,           -- first characters, shown in the UI
    scopes       TEXT[] NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX api_tokens_user_idx ON api_tokens(user_id);

-- Single-use email links (verification, password reset, email change); only
-- hashes are stored.
CREATE TABLE email_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    TEXT NOT NULL CHECK (purpose IN ('verify', 'reset', 'email')),
    new_email  TEXT NOT NULL DEFAULT '',  -- purpose 'email': the address to switch to
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_tokens_user_idx ON email_tokens(user_id);

-- ---------------------------------------------------------------------------
-- Admins: separate accounts, never students (an email is one or the other,
-- enforced by internal/auth).
-- ---------------------------------------------------------------------------
CREATE TABLE admins (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE admin_sessions (
    token_hash TEXT PRIMARY KEY,
    admin_id   BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX admin_sessions_admin_idx ON admin_sessions(admin_id);

-- Admin API tokens (fca_…), only valid on /v1/admin.
CREATE TABLE admin_tokens (
    id           BIGSERIAL PRIMARY KEY,
    admin_id     BIGINT NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    prefix       TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX admin_tokens_admin_idx ON admin_tokens(admin_id);

-- ---------------------------------------------------------------------------
-- Plans: assigned by admins only (no online payment yet).
-- ---------------------------------------------------------------------------
CREATE TABLE subscriptions (
    user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    plan        TEXT NOT NULL,
    valid_until TIMESTAMPTZ,              -- NULL = never expires
    assigned_by BIGINT REFERENCES admins(id) ON DELETE SET NULL, -- NULL = CLI or system
    note        TEXT NOT NULL DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Audit log of every plan change.
CREATE TABLE plan_changes (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    old_plan    TEXT NOT NULL,
    new_plan    TEXT NOT NULL,
    valid_until TIMESTAMPTZ,
    changed_by  BIGINT REFERENCES admins(id) ON DELETE SET NULL, -- NULL = CLI or system (expiry)
    note        TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One plan-expiry reminder of each kind per subscription period.
CREATE TABLE reminders_sent (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    valid_until TIMESTAMPTZ NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, valid_until)
);

-- ---------------------------------------------------------------------------
-- Apps and deployments
-- ---------------------------------------------------------------------------
CREATE TABLE apps (
    id                    BIGSERIAL PRIMARY KEY,
    user_id               BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                  TEXT NOT NULL UNIQUE,  -- also the subdomain
    repo_url              TEXT NOT NULL,
    branch                TEXT NOT NULL,
    -- detecting|review|building|running|sleeping|stopped|failed
    status                TEXT NOT NULL DEFAULT 'detecting',
    -- The stack, confirmed by the student after the analysis (or given
    -- explicitly through the API/MCP). See internal/stacks.
    stack_language        TEXT NOT NULL DEFAULT '',
    stack_framework       TEXT NOT NULL DEFAULT '',
    stack_version         TEXT NOT NULL DEFAULT '',  -- '' = automatic
    build_command         TEXT NOT NULL DEFAULT '',
    start_command         TEXT NOT NULL DEFAULT '',
    publish_dir           TEXT NOT NULL DEFAULT '',  -- static stacks, relative to root_dir
    root_dir              TEXT NOT NULL DEFAULT '',  -- working directory; '' = repository root
    health_path           TEXT NOT NULL DEFAULT '',  -- HTTP path checked before a deploy goes live; '' = off
    stack_confirmed       BOOLEAN NOT NULL DEFAULT FALSE,
    detection             JSONB,                     -- last analysis result
    last_request_at       TIMESTAMPTZ,               -- last visit seen by the monitor
    abandon_warned_at     TIMESTAMPTZ,               -- "will be stopped" email sent (abandoned apps)
    current_deployment_id BIGINT,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX apps_user_idx ON apps(user_id);

-- Deployments double as the job queue (status = 'queued').
CREATE TABLE deployments (
    id          BIGSERIAL PRIMARY KEY,
    app_id      BIGINT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,            -- detect | build | restart | rollback
    -- queued|building|deploying|running|superseded|done|failed|cancelled
    status      TEXT NOT NULL DEFAULT 'queued',
    commit_sha  TEXT NOT NULL DEFAULT '',
    image       TEXT NOT NULL DEFAULT '', -- set before deploy; preset for restart/rollback
    log         TEXT NOT NULL DEFAULT '',  -- the last 1 MiB of output
    log_size    BIGINT NOT NULL DEFAULT 0, -- characters ever written (offsets of live followers)
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at  TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);
CREATE INDEX deployments_app_idx ON deployments(app_id, id DESC);
CREATE INDEX deployments_queue_idx ON deployments(id) WHERE status = 'queued';

ALTER TABLE apps ADD CONSTRAINT apps_current_deployment_fk
    FOREIGN KEY (current_deployment_id) REFERENCES deployments(id) ON DELETE SET NULL;

-- Per-app metrics sampled by the monitor: one row per minute (kept 7 days),
-- rolled up into one row per hour (kept 30 days).
CREATE TABLE app_metrics (
    app_id       BIGINT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    resolution   TEXT NOT NULL CHECK (resolution IN ('minute', 'hour')),
    ts           TIMESTAMPTZ NOT NULL,     -- start of the minute or hour
    cpu_percent  REAL NOT NULL,            -- of one CPU (100 = one full core)
    memory_bytes BIGINT NOT NULL,
    requests     INT NOT NULL,             -- requests during the period
    PRIMARY KEY (app_id, resolution, ts)
);

CREATE TABLE env_vars (
    app_id BIGINT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    key    TEXT NOT NULL,
    value  TEXT NOT NULL,
    PRIMARY KEY (app_id, key)
);

CREATE TABLE domains (
    id         BIGSERIAL PRIMARY KEY,
    app_id     BIGINT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    hostname   TEXT NOT NULL UNIQUE,
    verified   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Storage and databases of an app: a persistent volume (/data), a managed
-- PostgreSQL database and a Valkey cache, at most one of each. The secret is
-- the database or cache password, injected into the app's environment.
CREATE TABLE app_services (
    app_id          BIGINT NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('volume', 'postgres', 'valkey')),
    secret          TEXT NOT NULL DEFAULT '',
    size_bytes      BIGINT NOT NULL DEFAULT 0,   -- last measured usage
    measured_at     TIMESTAMPTZ,
    read_only_since TIMESTAMPTZ,                 -- postgres over its size limit
    blocked_since   TIMESTAMPTZ,                 -- postgres far over the limit: connections refused
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (app_id, kind)
);

-- ---------------------------------------------------------------------------
-- Abuse protection and email
-- ---------------------------------------------------------------------------
CREATE TABLE banned_repos (
    repo_url   TEXT PRIMARY KEY,           -- canonical https://host/path.git
    reason     TEXT NOT NULL DEFAULT '',
    banned_by  BIGINT REFERENCES admins(id) ON DELETE SET NULL, -- NULL = CLI
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE abuse_flags (
    id          BIGSERIAL PRIMARY KEY,
    app_id      BIGINT REFERENCES apps(id) ON DELETE SET NULL,
    app_name    TEXT NOT NULL,
    user_id     BIGINT REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,             -- cpu
    detail      TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL DEFAULT '',  -- e.g. 'stopped'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ
);
CREATE INDEX abuse_flags_open_idx ON abuse_flags(created_at DESC) WHERE resolved_at IS NULL;

-- Outbound email queue: requests never wait for SMTP.
CREATE TABLE email_outbox (
    id              BIGSERIAL PRIMARY KEY,
    to_addr         TEXT NOT NULL,
    subject         TEXT NOT NULL,
    body            TEXT NOT NULL,
    attempts        INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ,
    last_error      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_outbox_pending_idx ON email_outbox(next_attempt_at) WHERE sent_at IS NULL;
