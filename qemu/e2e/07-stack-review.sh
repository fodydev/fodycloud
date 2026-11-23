# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Stack review and changes"
build_log() { local dep; dep="$(body_of "$(api GET "/v1/apps/$APP/deployments" "$TOK_A")" | jq -r '.deployments[0].id')"
  body_of "$(api GET "/v1/apps/$APP/deployments/$dep/log" "$TOK_A")" | jq -r .log; }
last_error() { body_of "$(api GET "/v1/apps/$APP/deployments" "$TOK_A")" | jq -r '.deployments[0].error'; }
stack_put() { api PUT "/v1/apps/$APP/stack" "$TOK_A" "$1"; }
check "unknown framework rejected" expect 422 "$(stack_put '{"language":"python","framework":"rails"}')" "unknown framework"
check "custom commands without a run command rejected" expect 422 "$(stack_put '{"language":"node","framework":"custom","build_command":"npm test"}')" "need a run command"
check "working directory outside the repository rejected" expect 422 "$(stack_put '{"language":"node","framework":"node-server","root_dir":"../../etc"}')" "working directory"
check "a confirmed app can't switch folder by re-analysing" expect 422 "$(api POST "/v1/apps/$APP/analyse" "$TOK_A" '{"root_dir":"web"}')" "stack is confirmed"
check "working directory missing from the branch -> build fails clearly" expect 200 "$(stack_put '{"language":"node","framework":"node-server","root_dir":"no-such-folder"}')" '"kind":"build"'
check "...with a clear error" bash -c '[[ "$1" == *"does not exist in this branch"* ]]' _ "$(for _ in $(seq 1 60); do e="$(last_error)"; [[ -n "$e" && "$e" != null ]] && { echo "$e"; break; }; sleep 3; done)"
check "static site with an unsafe publish folder rejected" expect 422 "$(stack_put '{"language":"static","framework":"static","publish_dir":"../etc"}')" "publish folder"
NONCE="$RANDOM$RANDOM" # new command each run, so BuildKit cannot reuse a cached build step
check "change the detected stack: Node.js with custom commands -> rebuild queued" expect 200 \
  "$(stack_put "{\"language\":\"node\",\"framework\":\"custom\",\"build_command\":\"echo build-time-GREETING=\$GREETING run-$NONCE\",\"start_command\":\"node index.js\"}")" '"kind":"build"'
check "custom build reaches running" test "$(wait_deployment "$TOK_A")" = running
check "custom build command ran with the app's env var" bash -c "grep -q 'build-time-GREETING=hello-fodycloud run-$NONCE' <<<\"\$1\"" _ "$(build_log)"
check "container runs the chosen run command" remote "docker inspect $(container_of) --format '{{json .Config.Cmd}}' | grep -q 'node index.js'"
check "app still answers after the custom build" app_ok "$APP.apps.$DOMAIN"
check "stack visible on the app" expect 200 "$(api GET "/v1/apps/$APP" "$TOK_A")" '"framework":"custom"'
stack_put '{"language":"node","framework":"custom","start_command":"echo bye"}' >/dev/null
check "run command that exits -> deployment failed (no fake 'running')" test "$(wait_deployment "$TOK_A")" = failed
check "failure explains the crash" bash -c "grep -q 'during startup' <<<\"\$1\"" _ "$(last_error)"
check "previous version keeps serving after a failed deploy" app_ok "$APP.apps.$DOMAIN"
check "back to the detected stack -> rebuild queued" expect 200 "$(stack_put '{"language":"node","framework":"node-server"}')" '"kind":"build"'
check "rebuild reaches running" test "$(wait_deployment "$TOK_A")" = running

STATIC=e2e-static
check "static repository analysed" expect 202 "$(api POST /v1/apps "$TOK_A" "{\"name\":\"$STATIC\",\"repo_url\":\"https://github.com/octocat/Spoon-Knife\",\"branch\":\"main\"}")"
check "static analysis ends in review" test "$(wait_status "$STATIC" "$TOK_A" review)" = review
SDET="$(detection_of "$STATIC" "$TOK_A")"
check "detected a static website (publish folder .)" bash -c 'jq -e ".detected and .language==\"static\" and .suggested.publish_dir==\".\"" <<<"$1" >/dev/null' _ "$SDET"
check "confirm static website -> build queued" expect 200 "$(api PUT "/v1/apps/$STATIC/stack" "$TOK_A" '{"language":"static","framework":"static","publish_dir":"."}')" '"kind":"build"'
for _ in $(seq 1 $(( BUILD_WAIT / 5 ))); do [[ "$(app_status "$STATIC" "$TOK_A")" =~ ^(running|failed)$ ]] && break; sleep 5; done
check "static site served over HTTPS" app_ok "$STATIC.apps.$DOMAIN" "Fork me? Fork you"
api DELETE "/v1/apps/$STATIC?confirm=$STATIC" "$TOK_A" >/dev/null

EMPTY=e2e-empty
check "repository with nothing to run is analysed" expect 202 "$(api POST /v1/apps "$TOK_A" \
  "{\"name\":\"$EMPTY\",\"repo_url\":\"https://github.com/octocat/Hello-World\",\"branch\":\"master\",\"env\":{\"FOO\":\"bar\"}}")"
check "...and waits for review" test "$(wait_status "$EMPTY" "$TOK_A" review)" = review
check "detection says nothing was found and suggests nothing" bash -c 'jq -e "(.detected|not) and .suggested == null and (.message|test(\"couldn.t detect\"))" <<<"$1" >/dev/null' _ "$(detection_of "$EMPTY" "$TOK_A")"
check "no container started for the undetected app" remote "! docker ps -aq --filter label=fodycloud.app=$EMPTY | grep -q ."
api DELETE "/v1/apps/$EMPTY?confirm=$EMPTY" "$TOK_A" >/dev/null
