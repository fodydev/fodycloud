-- SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
-- SPDX-License-Identifier: AGPL-3.0-or-later

-- Sign-in with a forge account (GitHub, GitLab, Codeberg). Additive only:
-- platforms upgraded from v0.1.0 keep every row, and the v0.1.0 binary
-- still runs on this schema (it ignores the new tables). Accounts created
-- through a forge have no password: users.password_hash = '' never matches.

-- Forge accounts linked to students. issuer is the forge instance's base URL
-- (GitLab and Codeberg can be self-hosted), subject its numeric account ID.
CREATE TABLE user_identities (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider   TEXT NOT NULL,              -- github | gitlab | codeberg
    issuer     TEXT NOT NULL,
    subject    TEXT NOT NULL,
    username   TEXT NOT NULL DEFAULT '',   -- at the last sign-in, for display
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (issuer, subject),
    UNIQUE (user_id, provider)
);

-- Sign-ups through a forge that wait for the student to accept the terms;
-- only the hash of the browser's token is stored. Short-lived.
CREATE TABLE oauth_signups (
    token_hash TEXT PRIMARY KEY,
    provider   TEXT NOT NULL,
    issuer     TEXT NOT NULL,
    subject    TEXT NOT NULL,
    username   TEXT NOT NULL DEFAULT '',
    email      TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
