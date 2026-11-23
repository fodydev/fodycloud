# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Cleanup"
check "delete without confirmation refused" expect 422 "$(api DELETE "/v1/apps/$APP?confirm=wrong" "$TOK_A")"
check "delete app" expect 204 "$(api DELETE "/v1/apps/$APP?confirm=$APP" "$TOK_A")"
check "containers and images removed" remote "! docker ps -a --filter label=fodycloud.app=$APP -q | grep -q . && ! docker images -q --filter label=fodycloud.app=$APP | grep -q ."
