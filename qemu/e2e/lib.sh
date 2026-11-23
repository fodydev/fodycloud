# shellcheck shell=bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Helpers for the e2e sections (sourced by e2e-test.sh after testlib.sh).

# fc ARGS…: runs the fodycloud CLI inside the control-plane container
# (FC_NEW_PASSWORD = the test password, for create-user / create-admin).
fc() { remote "cd /opt/fodycloud && docker compose exec -T -e FC_NEW_PASSWORD='$PASSWORD' control-plane fodycloud $*"; }
# api METHOD PATH TOKEN [JSON] -> prints "<status>\n<body>"
api() {
  local args=(-sk --max-time 60 -o /dev/stdout -w '\n%{http_code}' -X "$1" --resolve "api.$DOMAIN:$HTTPS_PORT:$HOST"
    -H "Authorization: Bearer $3" "https://api.$DOMAIN:$HTTPS_PORT$2")
  [[ $# -ge 4 ]] && args+=(-H 'Content-Type: application/json' --data "$4")
  curl "${args[@]}"
}
status_of() { tail -1 <<<"$1"; }
body_of() { sed '$d' <<<"$1"; }
expect() { # expect STATUS "<api output>" [grep-pattern]
  local st; st="$(status_of "$2")"
  [[ "$st" == "$1" ]] || { echo "status $st, expected $1: $(body_of "$2")"; return 1; }
  [[ -z "${3:-}" ]] || grep -q -- "$3" <<<"$(body_of "$2")" || { echo "body does not match '$3': $(body_of "$2")"; return 1; }
}
# mcp TOKEN JSON-RPC-BODY -> response body (SSE "data:" lines unwrapped)
mcp() {
  local auth=(); [[ -n "$1" ]] && auth=(-H "Authorization: Bearer $1")
  curl -sk --max-time 60 --resolve "mcp.$DOMAIN:$HTTPS_PORT:$HOST" "${auth[@]}" \
    -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
    -H 'MCP-Protocol-Version: 2025-06-18' \
    --data "$2" "https://mcp.$DOMAIN:$HTTPS_PORT/mcp" | sed -n 's/^data: //p; /^{/p'
}
# app_ok HOST: the sample app itself answers 200 (not the "Waking up"/"No app
# here" pages, which are HTML too).
app_ok() { # app_ok HOST [MARKER]
  local out
  out="$(curl -sk --max-time 15 --resolve "$1:$HTTPS_PORT:$HOST" -w '\n%{http_code}' "https://$1:$HTTPS_PORT/")"
  if [[ "$(tail -1 <<<"$out")" != 200 ]] || ! grep -q "${2:-$SAMPLE_MARKER}" <<<"$out"; then
    echo "HTTP $(tail -1 <<<"$out")"
    return 1
  fi
}
app_status() { body_of "$(api GET "/v1/apps/$1" "$2")" | jq -r .status; }
wait_status() { # wait_status APP TOKEN STATUS -> prints the final status (the analysis clones the repo)
  local st=""
  for _ in $(seq 1 100); do st="$(app_status "$1" "$2")"; [[ "$st" == "$3" ]] && break; sleep 3; done
  echo "$st"
}
detection_of() { body_of "$(api GET "/v1/apps/$1" "$2")" | jq -c .detection; }
# BUILD_WAIT: seconds to wait for a deployment; the first build downloads large
# base images, which takes a while on slow connections.
BUILD_WAIT="${BUILD_WAIT:-1800}"
wait_deployment() { # wait_deployment TOKEN -> prints final status
  local st=""
  for _ in $(seq 1 $(( BUILD_WAIT / 5 ))); do
    st="$(body_of "$(api GET "/v1/apps/$APP/deployments" "$1")" | jq -r '.deployments[0].status')"
    case "$st" in running|failed|cancelled) break ;; esac
    sleep 5
  done
  echo "$st"
}
container_of() { remote "docker ps --filter label=fodycloud.app=$APP --format '{{.Names}}' | head -1"; }
limits_of() { remote "docker inspect $(container_of) --format '{{.HostConfig.NanoCpus}} {{.HostConfig.Memory}} {{.HostConfig.PidsLimit}}'"; }
