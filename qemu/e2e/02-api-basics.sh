# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "API basics"
check "unauthenticated request -> 401" expect 401 "$(api GET /v1/apps nope)"
check "GET /me shows the free plan" expect 200 "$(api GET /v1/me "$TOK_A")" '"plan":"free"'
