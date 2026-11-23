# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Bans"
fc admin ban-repo --repo "$SAMPLE_REPO" --reason e2e >/dev/null
check "deploying a banned repository is refused" expect 403 "$(api POST "/v1/apps/$APP/deploy" "$TOK_A")" "not allowed"
fc admin unban-repo --repo "$SAMPLE_REPO" >/dev/null
check "non-admin cannot ban users" expect 403 "$(api POST "/v1/admin/users/$B_EMAIL/ban" "$TOK_A" '{"reason":"x"}')"
check "admin suspends user A" expect 202 "$(api POST "/v1/admin/users/$A_EMAIL/ban" "$TOK_ADMIN" '{"reason":"e2e test"}')"
check "suspended user's apps are stopped" remote "! docker ps -q --filter label=fodycloud.app=$APP | grep -q ."
check "suspended user's token is rejected" expect 401 "$(api GET /v1/apps "$TOK_A")"
check "suspended user is emailed" mailpit_text "$A_EMAIL" "suspended"
check "admin reinstates user A" expect 204 "$(api DELETE "/v1/admin/users/$A_EMAIL/ban" "$TOK_ADMIN")"
TOK_A="$(fc admin create-token --email "$A_EMAIL" --name e2e-after-ban)" # old tokens stay revoked
