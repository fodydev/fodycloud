# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Custom-domain limits, account self-service and admin accounts"

# as JAR ARGS…: a dashboard request with the cookie jar JAR, printing
# "<status> <redirect URL>".
as() {
  local jar="$1"; shift
  curl -sk --max-time 60 "${RES[@]}" -b "$jar" -c "$jar" -H "Origin: $D" -o /dev/null -w '%{http_code} %{redirect_url}' "$@"
}
sign_in() { as "$1" --data-urlencode "email=$2" --data-urlencode "password=$3" "$D/login"; }
mail_has() { mailpit_text "$1" "$2" | grep -q "$3"; } # mail_has TO SUBJECT-PART TEXT

# --- Custom domains: per-plan limit and removal (student plan: 5 per app).
S_EMAIL="student-s-$RANDOM@$DOMAIN"
fc admin create-user --email "$S_EMAIL" >/dev/null
fc admin set-plan --email "$S_EMAIL" --plan student --note e2e >/dev/null
TOK_S="$(fc admin create-token --email "$S_EMAIL" --name e2e-s)"
api POST /v1/apps "$TOK_S" "{\"name\":\"e2e-domains\",\"repo_url\":\"$SAMPLE_REPO\"}" >/dev/null
DOMS=()
for i in 1 2 3 4 5; do DOMS+=("d$i-$RANDOM.example.com"); done
for d in "${DOMS[@]}"; do api POST /v1/apps/e2e-domains/domains "$TOK_S" "{\"hostname\":\"$d\"}" >/dev/null; done
check "five custom domains fit the student plan" test "$(body_of "$(api GET /v1/apps/e2e-domains "$TOK_S")" | jq '.custom_domains | length')" = 5
check "a sixth is refused with the plan's limit" expect 403 \
  "$(api POST /v1/apps/e2e-domains/domains "$TOK_S" '{"hostname":"d6.example.com"}')" "allows 5 custom domains per app"
check "remove a domain through the API" expect 204 "$(api DELETE "/v1/apps/e2e-domains/domains/${DOMS[0]}" "$TOK_S")"
check "a removed domain is gone" expect 404 "$(api GET "/v1/apps/e2e-domains/domains/${DOMS[0]}" "$TOK_S")"
check "MCP removal needs the hostname as confirmation" bash -c '[[ "$1" == *"confirmation does not match the hostname"* ]]' _ \
  "$(mcp "$TOK_S" "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"remove_custom_domain\",\"arguments\":{\"app\":\"e2e-domains\",\"hostname\":\"${DOMS[1]}\",\"confirm\":\"wrong\"}}}")"
SJAR="$(mktemp)" SJAR2="$(mktemp)"
sign_in "$SJAR" "$S_EMAIL" "$PASSWORD" >/dev/null
check "remove a domain from the dashboard" bash -c '[[ "$1" == *"removed"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "hostname=${DOMS[1]}" "$D/apps/e2e-domains/domains/remove")"
check "three domains left" test "$(body_of "$(api GET /v1/apps/e2e-domains "$TOK_S")" | jq '.custom_domains | length')" = 3
F_EMAIL="student-f-$RANDOM@$DOMAIN"
fc admin create-user --email "$F_EMAIL" >/dev/null
TOK_F="$(fc admin create-token --email "$F_EMAIL" --name e2e-f)"
api POST /v1/apps "$TOK_F" "{\"name\":\"e2e-free-dom\",\"repo_url\":\"$SAMPLE_REPO\"}" >/dev/null
check "the free plan has no custom domains" expect 403 \
  "$(api POST /v1/apps/e2e-free-dom/domains "$TOK_F" '{"hostname":"free.example.com"}')" "not included in the free plan"

# --- Change password (signed in): other sessions are signed out, tokens stay.
NEW_PW="e2e-new-password-789"
sign_in "$SJAR2" "$S_EMAIL" "$PASSWORD" >/dev/null
check "a wrong current password is refused" bash -c '[[ "$1" == *"current+password+is+wrong"* ]]' _ \
  "$(as "$SJAR" --data-urlencode current=wrong-password-1 --data-urlencode "password=$NEW_PW" --data-urlencode "password2=$NEW_PW" "$D/account/password")"
check "change the password" bash -c '[[ "$1" == *"Password+changed"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "current=$PASSWORD" --data-urlencode "password=$NEW_PW" --data-urlencode "password2=$NEW_PW" "$D/account/password")"
check "this session stays signed in" bash -c '[[ "$1" == "200 " ]]' _ "$(as "$SJAR" "$D/apps")"
check "the other session is signed out" bash -c '[[ "$1" == "303 "*"/login" ]]' _ "$(as "$SJAR2" "$D/apps")"
check "the old password no longer works" bash -c '[[ "$1" == *"Invalid+email+or+password"* ]]' _ "$(sign_in "$(mktemp)" "$S_EMAIL" "$PASSWORD")"
check "the new password works" bash -c '[[ "$1" == "303 "*"/apps" ]]' _ "$(sign_in "$SJAR2" "$S_EMAIL" "$NEW_PW")"
check "API tokens keep working" expect 200 "$(api GET /v1/me "$TOK_S")"
check "the student is told by email" mail_has "$S_EMAIL" "password was changed" "sessions were signed out"

# --- Change email: nothing changes until the link sent to the new address is used.
S_NEW="student-s-new-$RANDOM@$DOMAIN"
check "an address used by a student is refused" bash -c '[[ "$1" == *"already+exists"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "email=$A_EMAIL" --data-urlencode "password=$NEW_PW" "$D/account/email")"
check "an address used by an admin is refused" bash -c '[[ "$1" == *"already+exists"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "email=admin@$DOMAIN" --data-urlencode "password=$NEW_PW" "$D/account/email")"
check "request an email change" bash -c '[[ "$1" == *"confirmation+link+was+sent"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "email=$S_NEW" --data-urlencode "password=$NEW_PW" "$D/account/email")"
check "the email doesn't change before the link is used" bash -c 'jq -e ".email == \"$2\"" <<<"$1" >/dev/null' _ "$(body_of "$(api GET /v1/me "$TOK_S")")" "$S_EMAIL"
LINK="$(mailpit_text "$S_NEW" "Confirm your new FodyCloud email" | grep -oE "https://dashboard\.$DOMAIN/verify-email\?token=[A-Za-z0-9_-]+" | head -1 |
  sed "s#dashboard\.$DOMAIN/#dashboard.$DOMAIN:$HTTPS_PORT/#")"
check "the link is sent to the new address" test -n "$LINK"
check "the link switches the email" bash -c '[[ "$1" == *"Your+email+is+now"* ]]' _ "$(as "$SJAR" "$LINK")"
check "the account now uses the new address" bash -c 'jq -e ".email == \"$2\"" <<<"$1" >/dev/null' _ "$(body_of "$(api GET /v1/me "$TOK_S")")" "$S_NEW"
check "the old address was told about the request" mail_has "$S_EMAIL" "email requested" "$S_NEW"
check "the old address was told about the change" mail_has "$S_EMAIL" "email was changed" "$S_NEW"
check "the link works only once" bash -c '[[ "$1" == *"invalid+or+has+expired"* ]]' _ "$(as "$SJAR" "$LINK")"

# --- Admin accounts: listed in the CLI and the panel, removed by the CLI, never the last one.
fc admin create-admin --email "e2e-admin2@$DOMAIN" >/dev/null 2>&1 || true
TOK_A2="$(fc admin create-admin-token --email "e2e-admin2@$DOMAIN" --name e2e)"
check "list-admins shows the admins" bash -c '[[ "$1" == *"admin@'"$DOMAIN"'"* && "$1" == *"e2e-admin2@'"$DOMAIN"'"* ]]' _ "$(fc admin list-admins)"
A2JAR="$(mktemp)"
curl -sk "${ARES[@]}" -b "$A2JAR" -c "$A2JAR" -H "Origin: $ADM" -o /dev/null --data-urlencode "email=e2e-admin2@$DOMAIN" --data-urlencode "password=$PASSWORD" "$ADM/login"
check "the admin panel lists the admins" bash -c 'grep -q "Admin since" <<<"$1" && grep -q "admin@'"$DOMAIN"'" <<<"$1"' _ "$(curl -sk "${ARES[@]}" -b "$A2JAR" "$ADM/")"
check "the admin token works before the removal" expect 200 "$(api GET /v1/admin/abuse "$TOK_A2")"
check "remove-admin removes an admin" bash -c '[[ "$1" == "removed admin"* ]]' _ "$(fc admin remove-admin --email "e2e-admin2@$DOMAIN" 2>&1)"
check "the removed admin's token stops working" expect 401 "$(api GET /v1/admin/abuse "$TOK_A2")"
check "the removed admin's session ends" bash -c 'grep -q "Sign in to the admin panel" <<<"$1"' _ "$(curl -sk "${ARES[@]}" -b "$A2JAR" "$ADM/")"
fc admin remove-admin --email "e2e-admin@$DOMAIN" >/dev/null 2>&1 || true # leaves admin@ as the only admin
check "the last admin can't be removed" bash -c '[[ "$1" == *"last admin account"* ]]' _ "$(fc admin remove-admin --email "admin@$DOMAIN" 2>&1)"
check "...and stays" bash -c '[[ "$1" == *"admin@'"$DOMAIN"'"* ]]' _ "$(fc admin list-admins)"

# --- Delete account: from the dashboard (S, with a running app), then through the API (F).
check "deploy an app before deleting the account" expect 202 "$(api POST /v1/apps "$TOK_S" \
  '{"name":"e2e-gone","repo_url":"https://github.com/octocat/Spoon-Knife","branch":"main","stack":{"language":"static","framework":"static"}}')"
for _ in $(seq 1 60); do [[ "$(app_status e2e-gone "$TOK_S")" == running ]] && break; sleep 5; done
check "...it runs" test "$(app_status e2e-gone "$TOK_S")" = running
check "a wrong confirmation is refused" bash -c '[[ "$1" == *"confirmation+does+not+match"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "password=$NEW_PW" --data-urlencode "confirm=someone@else.cm" "$D/account/delete")"
check "delete the account from the dashboard" bash -c '[[ "$1" == "303 https://'"$DOMAIN"'/?ok=Your+account"* ]]' _ \
  "$(as "$SJAR" --data-urlencode "password=$NEW_PW" --data-urlencode "confirm=$S_NEW" "$D/account/delete")"
check "its app's containers and images are gone" remote "! docker ps -aq --filter label=fodycloud.app=e2e-gone | grep -q . && ! docker images -q --filter label=fodycloud.app=e2e-gone | grep -q ."
check "its token no longer works" expect 401 "$(api GET /v1/me "$TOK_S")"
check "it can't sign in any more" bash -c '[[ "$1" == *"Invalid+email+or+password"* ]]' _ "$(sign_in "$(mktemp)" "$S_NEW" "$NEW_PW")"
check "the student is told by email" mail_has "$S_NEW" "account was deleted" "were deleted"
check "API deletion needs the email as confirmation" expect 422 "$(api DELETE "/v1/me?confirm=wrong" "$TOK_F")" "confirmation does not match"
check "delete an account through the API" expect 204 "$(api DELETE "/v1/me?confirm=$F_EMAIL" "$TOK_F")"
check "...its apps are gone with it" remote "cd /opt/fodycloud && ! docker compose exec -T postgres psql -U fodycloud -d fodycloud -tAc \"SELECT 1 FROM apps WHERE name='e2e-free-dom'\" | grep -q 1"
rm -f "$SJAR" "$SJAR2" "$A2JAR"
