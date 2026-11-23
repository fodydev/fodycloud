# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Email: verification and password reset (via Mailpit)"
V_EMAIL="verify-$RANDOM$RANDOM@$DOMAIN" VJAR="$(mktemp)" NEW_PW="new-password-456"
web() { curl -sk --max-time 30 "${RES[@]}" -b "$VJAR" -c "$VJAR" -H "Origin: $D" -o /dev/null -w '%{http_code} %{redirect_url}' "$@"; }
mailpit_text() { # mailpit_text TO SUBJECT-PART -> text of the newest matching message
  local id
  for _ in $(seq 1 20); do
    id="$(remote "curl -s 'http://127.0.0.1:8025/api/v1/search?query=to:$1'" | jq -r --arg s "$2" '[.messages[]? | select(.Subject | contains($s))][0].ID // empty')"
    [[ -n "$id" ]] && { remote "curl -s http://127.0.0.1:8025/api/v1/message/$id" | jq -r .Text; return 0; }
    sleep 2
  done
  return 1
}
local_link() { grep -oE "https://dashboard\.$DOMAIN/[a-z]+\?token=[A-Za-z0-9_-]+" | head -1 | sed "s#dashboard\.$DOMAIN/#dashboard.$DOMAIN:$HTTPS_PORT/#"; }
has() { [[ "$1" == *"$2"* ]] || { echo "got: $1"; return 1; }; }
R="$(web --data-urlencode "email=$V_EMAIL" --data-urlencode "password=$PASSWORD" "$D/signup")"
check "signup without accepting the terms is refused" has "$R" "Accept+the+Terms"
R="$(web --data-urlencode "email=$V_EMAIL" --data-urlencode "password=$PASSWORD" --data accept_terms=1 "$D/signup")"
check "signup through the dashboard (terms accepted)" has "$R" "303 "
check "the acceptance is recorded with the policies' version" remote "cd /opt/fodycloud && docker compose exec -T postgres psql -U fodycloud -d fodycloud -tAc \"SELECT terms_version FROM users WHERE email='$V_EMAIL' AND terms_accepted_at IS NOT NULL\" | grep -q ."
R="$(web --data-urlencode name=e2e-unverified --data-urlencode repo_url=https://github.com/octocat/Hello-World --data-urlencode branch=master "$D/apps")"
check "unverified user cannot create apps" has "$R" "confirm+your+email"
VLINK="$(mailpit_text "$V_EMAIL" "Confirm" | local_link)"
check "verification email received with a link" test -n "$VLINK"
check "verification link confirms the email" has "$(web "$VLINK")" "Email+confirmed"
check "verification link is single-use" has "$(web "$VLINK")" "invalid"
R="$(web --data-urlencode name=e2e-verified --data-urlencode repo_url=https://github.com/octocat/Hello-World --data-urlencode branch=master "$D/apps")"
check "verified user can create apps" has "$R" "App+created"
web --data confirm=e2e-verified "$D/apps/e2e-verified/delete" >/dev/null
check "reset for an unknown email gives the same answer" has "$(web --data-urlencode "email=nobody-$RANDOM@$DOMAIN" "$D/forgot")" "If+an+account+exists"
web --data-urlencode "email=$V_EMAIL" "$D/forgot" >/dev/null
RLINK="$(mailpit_text "$V_EMAIL" "Reset" | local_link)"; RTOKEN="${RLINK##*token=}"
check "password reset email received" test -n "$RTOKEN"
check "password reset succeeds" has "$(web --data-urlencode "token=$RTOKEN" --data-urlencode "password=$NEW_PW" "$D/reset")" "Password+changed"
check "reset signs the user out everywhere" has "$(curl -sk "${RES[@]}" -b "$VJAR" -o /dev/null -w '%{redirect_url}' "$D/apps")" "/login"
check "reset link is single-use" has "$(web --data-urlencode "token=$RTOKEN" --data-urlencode "password=other-password-789" "$D/reset")" "invalid"
check "login with the new password" has "$(web --data-urlencode "email=$V_EMAIL" --data-urlencode "password=$NEW_PW" "$D/login")" "303 https://dashboard.$DOMAIN:$HTTPS_PORT/apps"
rm -f "$VJAR"
