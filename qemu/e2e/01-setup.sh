# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Setup: users and tokens (admin CLI)"
for e in "$A_EMAIL" "$B_EMAIL"; do fc admin create-user --email "$e" >/dev/null 2>&1 || true; done
fc admin revoke-plan --email "$A_EMAIL" >/dev/null 2>&1
TOK_A="$(fc admin create-token --email "$A_EMAIL" --name e2e-write --scopes apps:read,apps:write)"
TOK_RO="$(fc admin create-token --email "$A_EMAIL" --name e2e-readonly --scopes apps:read)"
TOK_B="$(fc admin create-token --email "$B_EMAIL" --name e2e-b)"
check "tokens issued (fc_ prefix)" bash -c "[[ '$TOK_A' == fc_* && '$TOK_RO' == fc_* && '$TOK_B' == fc_* ]]"
api DELETE "/v1/apps/$APP?confirm=$APP" "$TOK_A" >/dev/null # leftover from a previous run
