# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A or
# BUILD_WAIT come from e2e-test.sh and earlier parts (hence SC2153 is off).

section "More stacks: Go, PHP, Laravel, Spring Boot, Bun (real builds)"

# A research-plan student: 10 apps, and memory enough for Java.
M_EMAIL="student-m-$RANDOM@$DOMAIN"
fc admin create-user --email "$M_EMAIL" >/dev/null
fc admin set-plan --email "$M_EMAIL" --plan research --note e2e >/dev/null
TOK_M="$(fc admin create-token --email "$M_EMAIL" --name e2e-m)"

suggested() { body_of "$(api GET "/v1/apps/$1" "$TOK_M")" | jq -c .detection.suggested; }
# wait_latest APP: waits for the app's latest deployment to finish; prints
# its status.
wait_latest() {
  local st=""
  for _ in $(seq 1 $(( BUILD_WAIT / 5 ))); do
    st="$(body_of "$(api GET "/v1/apps/$1/deployments" "$TOK_M")" | jq -r '.deployments[0].status')"
    case "$st" in running|failed|cancelled) break ;; esac
    sleep 5
  done
  echo "$st"
}
# page APP PATH MARKER: the app answers 200 on PATH with MARKER in the body.
page() {
  local host="$1.apps.$DOMAIN" out
  out="$(curl -sk --max-time 30 --resolve "$host:$HTTPS_PORT:$HOST" -w '\n%{http_code}' "https://$host:$HTTPS_PORT$2")"
  if [[ "$(tail -1 <<<"$out")" != 200 ]] || ! grep -q -- "$3" <<<"$out"; then
    echo "HTTP $(tail -1 <<<"$out"): $(head -c 300 <<<"$out")"
    return 1
  fi
}
# failed_log APP: the end of the latest deployment's log, shown when a build fails.
failed_log() {
  local id; id="$(body_of "$(api GET "/v1/apps/$1/deployments" "$TOK_M")" | jq -r '.deployments[0].id')"
  body_of "$(api GET "/v1/apps/$1/deployments/$id/log" "$TOK_M")" | jq -r .log | tail -25
}
confirm() { # confirm APP: deploys the suggested stack as is
  api PUT "/v1/apps/$1/stack" "$TOK_M" "$(suggested "$1")" >/dev/null
}

# --- Refusals (no build).
check "a PHP stack with a run command is refused" expect 422 "$(api POST /v1/apps "$TOK_M" \
  '{"name":"e2e-php-bad","repo_url":"https://github.com/heroku/php-getting-started","stack":{"language":"php","framework":"php","start_command":"php -S 0.0.0.0:$PORT"}}')" "served by FrankenPHP"
check "an unsupported Go version is refused" expect 422 "$(api POST /v1/apps "$TOK_M" \
  '{"name":"e2e-go-bad","repo_url":"https://github.com/heroku/go-getting-started","stack":{"language":"go","framework":"go-server","version":"1.10"}}')" "unsupported Go version"

# --- Analyses: four repositories, analysed in parallel.
APP_KEY="base64:$(head -c 32 /dev/urandom | base64)"
api POST /v1/apps "$TOK_M" '{"name":"e2e-go","repo_url":"https://github.com/heroku/go-getting-started"}' >/dev/null
api POST /v1/apps "$TOK_M" '{"name":"e2e-php","repo_url":"https://github.com/heroku/php-getting-started"}' >/dev/null
api POST /v1/apps "$TOK_M" "{\"name\":\"e2e-laravel\",\"repo_url\":\"https://github.com/laravel/laravel\",\"branch\":\"12.x\",\"env\":{\"APP_KEY\":\"$APP_KEY\"}}" >/dev/null
api POST /v1/apps "$TOK_M" '{"name":"e2e-spring","repo_url":"https://github.com/spring-guides/gs-rest-service","stack":{"root_dir":"complete"}}' >/dev/null
for a in e2e-go e2e-php e2e-laravel e2e-spring; do
  check "$a: the analysis ends in review" test "$(wait_status "$a" "$TOK_M" review)" = review
done
check "Go is detected, run as ./out (not the Procfile's command)" bash -c \
  'jq -e ".language==\"go\" and .framework==\"go-server\" and .start_command==\"./out\"" <<<"$1" >/dev/null' _ "$(suggested e2e-go)"
check "plain PHP is detected, with the web/ folder as document root" bash -c \
  'jq -e ".language==\"php\" and .framework==\"php\" and .publish_dir==\"web\" and (.start_command // \"\")==\"\"" <<<"$1" >/dev/null' _ "$(suggested e2e-php)"
check "Laravel is detected" bash -c 'jq -e ".language==\"php\" and .framework==\"laravel\"" <<<"$1" >/dev/null' _ "$(suggested e2e-laravel)"
check "Spring Boot is detected in the complete/ folder, with the Java version of its toolchain" bash -c \
  'jq -e ".language==\"java\" and .framework==\"spring-boot\" and .root_dir==\"complete\" and .version==\"17\" and (.start_command|contains(\"-Dserver.port\"))" <<<"$1" >/dev/null' _ "$(suggested e2e-spring)"

# --- Builds: the suggested stacks as they are, and Bun on the Node sample.
for a in e2e-go e2e-php e2e-laravel e2e-spring; do confirm "$a"; done
api POST /v1/apps "$TOK_M" "{\"name\":\"e2e-bun\",\"repo_url\":\"$SAMPLE_REPO\",\"stack\":{\"language\":\"node\",\"framework\":\"bun\",\"start_command\":\"bun index.js\"}}" >/dev/null
for a in e2e-go e2e-php e2e-laravel e2e-spring e2e-bun; do
  st="$(wait_latest "$a")"
  [[ "$st" == running ]] || failed_log "$a"
  check "$a: built and deployed" test "$st" = running
done
check "the Go app answers" page e2e-go / "Heroku"
check "the PHP app answers from web/ (the Procfile's server was replaced)" page e2e-php / "Heroku"
check "the Laravel app answers its welcome page" page e2e-laravel / "Laravel"
check "the Spring Boot app answers /greeting" page e2e-spring /greeting '"content":"Hello, World!"'
check "the Bun app answers" page e2e-bun / "$SAMPLE_MARKER"
check "...and runs on Bun" remote "docker top \$(docker ps -q --filter label=fodycloud.app=e2e-bun | head -1) -o pid,comm | awk '{print \$2}' | grep -qx bun"
check "Java gets its default memory setting" remote \
  "docker inspect \$(docker ps -q --filter label=fodycloud.app=e2e-spring | head -1) --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -qx 'JAVA_OPTS=-XX:MaxRAMPercentage=75.0'"

# Free the VM's memory and disk for the next sections.
for a in e2e-go e2e-php e2e-laravel e2e-spring e2e-bun; do
  api DELETE "/v1/apps/$a?confirm=$a" "$TOK_M" >/dev/null
done
