# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Build and deploy ($SAMPLE_REPO)"
check "create app -> 202, the repository is analysed first (nothing deployed)" expect 202 \
  "$(api POST /v1/apps "$TOK_A" "{\"name\":\"$APP\",\"repo_url\":\"$SAMPLE_REPO\",\"branch\":\"main\"}")" '"kind":"detect"'
check "analysis ends in review" test "$(wait_status "$APP" "$TOK_A" review)" = review
DET="$(detection_of "$APP" "$TOK_A")"
check "detected Node.js · Node server, with suggested commands" bash -c 'jq -e ".detected and .language==\"node\" and .framework==\"node-server\" and .suggested.start_command != \"\"" <<<"$1" >/dev/null' _ "$DET"
check "no container before the stack is confirmed" remote "! docker ps -aq --filter label=fodycloud.app=$APP | grep -q ."
check "deploying before confirmation is refused" expect 422 "$(api POST "/v1/apps/$APP/deploy" "$TOK_A")" "confirm the detected stack"
check "confirm the suggested stack -> build queued" expect 200 \
  "$(api PUT "/v1/apps/$APP/stack" "$TOK_A" "$(jq -c .suggested <<<"$DET")")" '"kind":"build"'
START=$(date +%s); ST="$(wait_deployment "$TOK_A")"
echo "  build + deploy finished in $(( $(date +%s) - START ))s with status: $ST"
if ! check "deployment reaches running" test "$ST" = running; then
  DEP="$(body_of "$(api GET "/v1/apps/$APP/deployments" "$TOK_A")" | jq -r '.deployments[0].id')"
  body_of "$(api GET "/v1/apps/$APP/deployments/$DEP/log" "$TOK_A")" | jq -r .log | tail -40
fi
check "app answers over HTTPS at $APP.apps.$DOMAIN" app_ok "$APP.apps.$DOMAIN"
if [[ "$NET" == bridge && -s "$CA_ROOT" ]]; then
  check "the new app gets its own valid ACME certificate" valid_cert "$APP.apps.$DOMAIN"
fi
check "free plan limits on the container (0.25 cpu, 256m, 128 pids)" bash -c "[[ '$(limits_of)' == '250000000 268435456 128' ]]"
check "container hardened (cap-drop ALL but NET_BIND_SERVICE, no-new-privileges, 1G disk quota)" remote \
  "docker inspect $(container_of) --format '{{.HostConfig.CapDrop}} {{.HostConfig.CapAdd}} {{.HostConfig.SecurityOpt}} {{index .HostConfig.StorageOpt \"size\"}}' | grep -Eq '^\\[ALL\\] \\[(CAP_)?NET_BIND_SERVICE\\] .*no-new-privileges.*1G'"
check "builds run in the limited builder on the isolated fc-build network" remote \
  "docker inspect buildx_buildkit_fodycloud-builder0 --format '{{range \$k, \$v := .NetworkSettings.Networks}}{{\$k}}{{end}} {{.HostConfig.Memory}}' | grep -qx 'fodycloud-build 4294967296'"
check "runtime logs available" expect 200 "$(api GET "/v1/apps/$APP/logs?tail=20" "$TOK_A")" 'log'
