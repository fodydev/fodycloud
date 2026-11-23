# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Isolation between users"
check "user B cannot see user A's app (404)" expect 404 "$(api GET "/v1/apps/$APP" "$TOK_B")"
check "user B cannot deploy user A's app (404)" expect 404 "$(api POST "/v1/apps/$APP/deploy" "$TOK_B")"
check "user B's app list is empty" expect 200 "$(api GET /v1/apps "$TOK_B")" '"apps":\[\]'
