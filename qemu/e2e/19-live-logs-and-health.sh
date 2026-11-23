# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here);
# as and sign_in come from 18-account-and-domains.sh.

section "Live build logs, build durations and health checks"

H_EMAIL="student-h-$RANDOM@$DOMAIN"
fc admin create-user --email "$H_EMAIL" >/dev/null
TOK_H="$(fc admin create-token --email "$H_EMAIL" --name e2e-h)"
HAPP="e2e-health"
HHOST="$HAPP.apps.$DOMAIN"
STREAM_DIR="$(mktemp -d)"

# follow_log ID OUT: follows a deployment's log through the API's event
# stream until its "end" event, reconnecting from the last offset like a
# client would (each connection lasts at most ~50 s). Everything received is
# appended to OUT; prints the number of connections.
follow_log() {
  local from=0 n=0 chunk="$STREAM_DIR/chunk" id
  : >"$2"
  while (( n < 60 )); do
    n=$((n + 1))
    curl -sk -N --max-time 70 --resolve "api.$DOMAIN:$HTTPS_PORT:$HOST" -H "Authorization: Bearer $TOK_H" \
      -H 'Accept: text/event-stream' "https://api.$DOMAIN:$HTTPS_PORT/v1/apps/$HAPP/deployments/$1/log?from=$from" >"$chunk"
    cat "$chunk" >>"$2"
    grep -q '^event: end' "$chunk" && break
    id="$(sed -n 's/^id: //p' "$chunk" | tail -1)"
    from="${id:-$from}"
  done
  echo "$n"
}
# wait_depl ID: waits for a deployment of HAPP to finish; prints its status.
wait_depl() {
  local st=""
  for _ in $(seq 1 $(( BUILD_WAIT / 5 ))); do
    st="$(body_of "$(api GET "/v1/apps/$HAPP/deployments/$1" "$TOK_H")" | jq -r .status)"
    case "$st" in running|failed|cancelled) break ;; esac
    sleep 5
  done
  echo "$st"
}
# sse_text FILE: the text of the "log" events in a recorded stream, put back
# together as a client does (an event's data lines are joined with \n; a
# line of output may be split across two events).
sse_text() {
  awk 'BEGIN { RS = ""; FS = "\n" }
    { ev = ""; txt = ""; n = 0
      for (i = 1; i <= NF; i++) {
        if ($i ~ /^event: /) ev = substr($i, 8)
        else if ($i ~ /^data: /) { txt = txt (n++ ? "\n" : "") substr($i, 7) }
      }
      if (ev == "log") printf "%s", txt }' "$1"
}
build_log() { body_of "$(api GET "/v1/apps/$HAPP/deployments/$1/log" "$TOK_H")" | jq -r .log; }
restart() { body_of "$(api POST "/v1/apps/$HAPP/restart" "$TOK_H")" | jq -r .id; }
app_status_code() { curl -sk --max-time 15 --resolve "$HHOST:$HTTPS_PORT:$HOST" -o /dev/null -w '%{http_code}' "https://$HHOST:$HTTPS_PORT/"; }

# --- Live log stream (SSE), followed from the start of a real build.
OUT="$(api POST /v1/apps "$TOK_H" "{\"name\":\"$HAPP\",\"repo_url\":\"https://github.com/octocat/Spoon-Knife\",\"branch\":\"main\",\"stack\":{\"language\":\"static\",\"framework\":\"static\"}}")"
check "create an app with a stack (build starts)" expect 202 "$OUT"
D1="$(body_of "$OUT" | jq -r .deployment.id)"
CONNS="$(follow_log "$D1" "$STREAM_DIR/d1")"
check "the stream pushes output while the build runs" grep -q '^event: log' "$STREAM_DIR/d1"
check "...and status changes as they happen" grep -qx 'data: deploying' "$STREAM_DIR/d1"
check "...and ends with the final status" bash -c 'grep -A1 "^event: end" "$1" | grep -qx "data: running"' _ "$STREAM_DIR/d1"
sse_text "$STREAM_DIR/d1" >"$STREAM_DIR/d1.txt"
check "resuming after a reconnection repeats nothing ($CONNS connection(s))" \
  test "$(grep -c '^==> Cloning' "$STREAM_DIR/d1.txt")" = 1
check "the streamed text matches the stored log" bash -c 'diff <(grep "^==>" "$1") <(grep "^==>" <<<"$2")' _ "$STREAM_DIR/d1.txt" "$(build_log "$D1")"
LAST_ID="$(sed -n 's/^id: //p' "$STREAM_DIR/d1" | tail -1)"
RESUMED="$(curl -sk -N --max-time 20 --resolve "api.$DOMAIN:$HTTPS_PORT:$HOST" -H "Authorization: Bearer $TOK_H" \
  -H 'Accept: text/event-stream' -H "Last-Event-ID: $LAST_ID" "https://api.$DOMAIN:$HTTPS_PORT/v1/apps/$HAPP/deployments/$D1/log")"
check "Last-Event-ID at the end sends no text again" bash -c '! grep -q "^event: log" <<<"$1" && grep -q "^event: end" <<<"$1"' _ "$RESUMED"
check "...while from=0 sends the whole log (positive control)" bash -c 'grep -q "^event: log" <<<"$1"' _ \
  "$(curl -sk -N --max-time 20 --resolve "api.$DOMAIN:$HTTPS_PORT:$HOST" -H "Authorization: Bearer $TOK_H" \
    -H 'Accept: text/event-stream' "https://api.$DOMAIN:$HTTPS_PORT/v1/apps/$HAPP/deployments/$D1/log?from=0")"
check "without the header the API still answers JSON" expect 200 "$(api GET "/v1/apps/$HAPP/deployments/$D1/log" "$TOK_H")" '"log"'
HJAR="$(mktemp)"
sign_in "$HJAR" "$H_EMAIL" "$PASSWORD" >/dev/null
check "the dashboard streams the log as text/event-stream" bash -c 'grep -qi "^content-type: text/event-stream" <<<"$1"' _ \
  "$(curl -sk --max-time 20 "${RES[@]}" -b "$HJAR" -D - -o /dev/null "$D/apps/$HAPP/deployments/$D1/log/stream")"
check "another student can't follow it" bash -c '[[ "$1" == 404 ]]' _ \
  "$(curl -sk --max-time 20 "${RES[@]}" -b "$JAR" -o /dev/null -w '%{http_code}' "$D/apps/$HAPP/deployments/$D1/log/stream")"
check "...nor through the API" expect 404 "$(api GET "/v1/apps/$HAPP/deployments/$D1/log" "$TOK_A")"

# --- Build durations.
check "the log shows the build duration" bash -c 'grep -q "^==> Build finished in [0-9]" <<<"$1"' _ "$(build_log "$D1")"
check "...and the total" bash -c 'grep -q "^==> Deployment succeeded in [0-9]" <<<"$1"' _ "$(build_log "$D1")"
check "the API gives duration_seconds" bash -c 'jq -e ".duration_seconds > 0" <<<"$1" >/dev/null' _ "$(body_of "$(api GET "/v1/apps/$HAPP/deployments/$D1" "$TOK_H")")"
check "the deployments table has a Duration column" bash -c 'grep -q "<th>Duration</th>" <<<"$1"' _ "$(curl -sk "${RES[@]}" -b "$HJAR" "$D/apps/$HAPP")"

# --- Health check before going live.
check "an invalid health-check path is refused" expect 422 "$(api PUT "/v1/apps/$HAPP/health-check" "$TOK_H" '{"path":"healthz now"}')" "must start with /"
check "set the health check to /" expect 200 "$(api PUT "/v1/apps/$HAPP/health-check" "$TOK_H" '{"path":"/"}')" '"health_check_path":"/"'
D2="$(restart)"
check "a deployment that answers the path goes live" test "$(wait_depl "$D2")" = running
check "...after the check passed (positive control for the probe)" bash -c 'grep -q "==> Health check passed (200)" <<<"$1"' _ "$(build_log "$D2")"
check "set the health check from the dashboard" bash -c '[[ "$1" == *"Health+check+saved"* ]]' _ \
  "$(as "$HJAR" --data-urlencode "path=/no-such-page" "$D/apps/$HAPP/health-check")"
D3="$(restart)"
for _ in $(seq 1 40); do grep -q "GET /no-such-page" <<<"$(build_log "$D3")" && break; sleep 1; done
check "a version still being checked gets no visitors (not ready for routing)" remote \
  "test \"\$(docker inspect -f '{{.State.Health.Status}}' fc-app-$HAPP-$D3)\" = starting"
check "...while the live version is ready (positive control)" remote \
  "test \"\$(docker inspect -f '{{.State.Health.Status}}' fc-app-$HAPP-$D2)\" = healthy"
check "a deployment that doesn't answer the path fails" test "$(wait_depl "$D3")" = failed
check "...with the reason" bash -c 'jq -e ".error | contains(\"didn'"'"'t answer /no-such-page with a 2xx or 3xx status\")" <<<"$1" >/dev/null' _ \
  "$(body_of "$(api GET "/v1/apps/$HAPP/deployments/$D3" "$TOK_H")")"
check "...after showing each attempt" bash -c 'grep -q "GET /no-such-page → 404" <<<"$1"' _ "$(build_log "$D3")"
check "the previous version keeps serving" test "$(app_status_code)" = 200
check "...and stays the current deployment" bash -c 'jq -e ".current_deployment.id == $2" <<<"$1" >/dev/null' _ "$(body_of "$(api GET "/v1/apps/$HAPP" "$TOK_H")")" "$D2"
check "set_health_check over MCP turns it off" bash -c '[[ "$1" != *isError*true* ]]' _ \
  "$(mcp "$TOK_H" "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"set_health_check\",\"arguments\":{\"app\":\"$HAPP\",\"path\":\"\"}}}")"
check "...for the app" bash -c 'jq -e "has(\"health_check_path\") | not" <<<"$1" >/dev/null' _ "$(body_of "$(api GET "/v1/apps/$HAPP" "$TOK_H")")"
D4="$(restart)"
check "without a health check, deployments use the start-up wait again" test "$(wait_depl "$D4")" = running
check "...as the log shows" bash -c 'grep -q "==> Waiting .* for the app to stay up" <<<"$1" && ! grep -q "Checking http" <<<"$1"' _ "$(build_log "$D4")"
rm -rf "$STREAM_DIR" "$HJAR"
