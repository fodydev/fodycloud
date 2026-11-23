# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Environment variables"
check "set env var -> restart queued" expect 200 "$(api PATCH "/v1/apps/$APP/env" "$TOK_A" '{"GREETING":"hello-fodycloud"}')" '"kind":"restart"'
check "restart deployment reaches running" test "$(wait_deployment "$TOK_A")" = running
check "env var present in the new container" remote "docker inspect $(container_of) --format '{{.Config.Env}}' | grep -q GREETING=hello-fodycloud"
check "env values are masked in the API" expect 200 "$(api GET "/v1/apps/$APP/env" "$TOK_A")" '"GREETING":"he\*'
