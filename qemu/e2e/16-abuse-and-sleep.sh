# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "CPU abuse detection and idle sleep (about 5 minutes)"
C_EMAIL="student-c@$DOMAIN"
fc admin create-user --email "$C_EMAIL" >/dev/null 2>&1 || true
fc admin set-plan --email "$C_EMAIL" --plan sleepy --note e2e >/dev/null
TOK_C="$(fc admin create-token --email "$C_EMAIL" --name e2e-c)"
TOK_ADMIN="$(fc admin create-admin-token --email "admin@$DOMAIN" --name e2e-admin)"
api DELETE "/v1/apps/e2e-sleepy?confirm=e2e-sleepy" "$TOK_C" >/dev/null
api DELETE "/v1/apps/e2e-burn?confirm=e2e-burn" "$TOK_A" >/dev/null
BURN_JSON="$(jq -cn --arg r "$SAMPLE_REPO" '{name:"e2e-burn", repo_url:$r, stack:{language:"node", framework:"custom", start_command:"node -e \"for(;;){}\""}}')"
check "deploy a CPU-burning app on the free plan" expect 202 "$(api POST /v1/apps "$TOK_A" "$BURN_JSON")"
check "deploy an app on the 'sleepy' plan (sleeps after 1 idle minute)" expect 202 \
  "$(api POST /v1/apps "$TOK_C" "{\"name\":\"e2e-sleepy\",\"repo_url\":\"$SAMPLE_REPO\",\"stack\":{\"language\":\"node\",\"framework\":\"node-server\"}}")"
open_flags() { body_of "$(api GET /v1/admin/abuse "$TOK_ADMIN")" | jq '[.flags[] | select(.app=="e2e-burn")] | length'; }
START=$(date +%s)
for _ in $(seq 1 100); do
  [[ "$(app_status e2e-sleepy "$TOK_C")" == sleeping && "$(app_status e2e-burn "$TOK_A")" == stopped ]] && break
  sleep 6
done
echo "  waited $(( $(date +%s) - START ))s"
check "idle app was put to sleep" test "$(app_status e2e-sleepy "$TOK_C")" = sleeping
check "the sleeping app's container is stopped" remote "! docker ps -q --filter label=fodycloud.app=e2e-sleepy | grep -q ."
WAKE="$(curl -sk --max-time 15 --resolve "e2e-sleepy.apps.$DOMAIN:$HTTPS_PORT:$HOST" -w '\n%{http_code}' "https://e2e-sleepy.apps.$DOMAIN:$HTTPS_PORT/")"
check "first request shows the wake page (503 + refresh)" bash -c '[[ "$(tail -1 <<<"$1")" == 503 ]] && grep -q "Waking up" <<<"$1" && grep -q "http-equiv=\"refresh\"" <<<"$1"' _ "$WAKE"
for _ in $(seq 1 15); do
  [[ "$(curl -sk --max-time 10 -o /dev/null -w '%{http_code}' --resolve "e2e-sleepy.apps.$DOMAIN:$HTTPS_PORT:$HOST" "https://e2e-sleepy.apps.$DOMAIN:$HTTPS_PORT/")" == 200 ]] && break
  sleep 2
done
check "the app answers again after waking" app_ok "e2e-sleepy.apps.$DOMAIN"
check "woken app is running again" test "$(app_status e2e-sleepy "$TOK_C")" = running
check "CPU abuse flagged twice" test "$(open_flags)" -ge 2
check "abusive free-plan app stopped automatically" test "$(app_status e2e-burn "$TOK_A")" = stopped
check "owner emailed about the stop" mailpit_text "$A_EMAIL" "was stopped"
check "admins alerted" mailpit_text "admin@$DOMAIN" "cpu abuse"
api DELETE "/v1/apps/e2e-sleepy?confirm=e2e-sleepy" "$TOK_C" >/dev/null
api DELETE "/v1/apps/e2e-burn?confirm=e2e-burn" "$TOK_A" >/dev/null
