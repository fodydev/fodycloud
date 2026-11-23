# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Metrics, legal pages and unused apps (about 6 minutes)"

# --- Metrics: the sample app has been running for many minutes by now.
METRICS="$(body_of "$(api GET "/v1/apps/$APP/metrics" "$TOK_A")")"
check "metrics: per-minute points with CPU, memory and the plan's limits" bash -c \
  'jq -e ".step == \"minute\" and (.points | length) >= 2 and .points[-1].memory_bytes > 0 and .cpu_limit_percent > 0 and .memory_limit_bytes > 0" <<<"$1" >/dev/null' _ "$METRICS"
check "metrics: the requests made by earlier tests were counted" bash -c 'jq -e "[.points[].requests] | add > 0" <<<"$1" >/dev/null' _ "$METRICS"
check "metrics: the 7-day range is hourly" bash -c 'jq -e ".step == \"hour\" and (.points | length) >= 1" <<<"$1" >/dev/null' _ \
  "$(body_of "$(api GET "/v1/apps/$APP/metrics?range=7d" "$TOK_A")")"
check "metrics: an unknown range is refused" expect 422 "$(api GET "/v1/apps/$APP/metrics?range=1y" "$TOK_A")" "unknown range"
check "metrics: another student's app is not found" expect 404 "$(api GET "/v1/apps/$APP/metrics" "$TOK_B")"
check "metrics: the app page shows the Metrics card" bash -c "curl -sk -b $JAR ${RES[*]} $D/apps/$APP | grep -q 'class=\"chart\"'"
check "metrics: MCP get_app_metrics" bash -c 'jq -e "(.points | length) >= 2" <<<"$1" >/dev/null' _ "$(mcp_call get_app_metrics "{\"app\":\"$APP\"}")"

# --- Legal pages, filled with the test operator (qemu/test.tfvars).
legal_page() { curl -sk "${HOME_RES[@]}" "https://$DOMAIN:$HTTPS_PORT/legal/$1"; }
check "terms of service name the operator" bash -c 'grep -q "FodyCloud Test Operator" <<<"$1" && grep -q "Cameroon" <<<"$1"' _ "$(legal_page terms)"
check "privacy policy gives the contact address" bash -c 'grep -q "privacy@fodycloud.internal" <<<"$1"' _ "$(legal_page privacy)"
check "acceptable-use policy served" bash -c 'grep -q "Mine cryptocurrency" <<<"$1"' _ "$(legal_page acceptable-use)"

# --- Unused apps. Plan "unused": warned after ~3 minutes without visits or
# deploys, stopped ~2 minutes after the warning. Two static apps: "kept" is
# visited after the warning (which cancels it), "idle" never is (stopped).
# Nothing may visit them before the warning: that would count as activity.
U_EMAIL="student-u@$DOMAIN"
fc admin create-user --email "$U_EMAIL" >/dev/null 2>&1 || true
fc admin set-plan --email "$U_EMAIL" --plan unused --note e2e >/dev/null
TOK_U="$(fc admin create-token --email "$U_EMAIL" --name e2e-u)"
for name in e2e-kept e2e-idle; do
  api DELETE "/v1/apps/$name?confirm=$name" "$TOK_U" >/dev/null
  check "deploy $name on the 'unused' plan" expect 202 "$(api POST /v1/apps "$TOK_U" \
    "{\"name\":\"$name\",\"repo_url\":\"https://github.com/octocat/Spoon-Knife\",\"branch\":\"main\",\"stack\":{\"language\":\"static\",\"framework\":\"static\"}}")"
done
stops_on() { body_of "$(api GET "/v1/apps/$1" "$TOK_U")" | jq -r '.stops_on // empty'; }
mail_has() { mailpit_text "$1" "$2" | grep -q "$3"; } # mail_has TO SUBJECT-PART TEXT
for _ in $(seq 1 60); do [[ -n "$(stops_on e2e-kept)" && -n "$(stops_on e2e-idle)" ]] && break; sleep 6; done
check "unused apps get a stop date" bash -c '[[ -n "$1" && -n "$2" ]]' _ "$(stops_on e2e-kept)" "$(stops_on e2e-idle)"
check "the owner is warned by email" mail_has "$U_EMAIL" "will be stopped on" "no visits and no deployments"
curl -sk --max-time 10 --resolve "e2e-kept.apps.$DOMAIN:$HTTPS_PORT:$HOST" -o /dev/null "https://e2e-kept.apps.$DOMAIN:$HTTPS_PORT/"
for _ in $(seq 1 30); do [[ -z "$(stops_on e2e-kept)" ]] && break; sleep 6; done
check "a visit after the warning cancels it" test -z "$(stops_on e2e-kept)"
for _ in $(seq 1 40); do [[ "$(app_status e2e-idle "$TOK_U")" == stopped ]] && break; sleep 6; done
check "an app still unused after the grace period is stopped" test "$(app_status e2e-idle "$TOK_U")" = stopped
check "...and its owner is told" mail_has "$U_EMAIL" "was stopped (unused)" "stopped as announced"
check "the visited app keeps running" test "$(app_status e2e-kept "$TOK_U")" = running
for name in e2e-kept e2e-idle; do api DELETE "/v1/apps/$name?confirm=$name" "$TOK_U" >/dev/null; done
