# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).
#
# Sign-in with a forge. test.tfvars sets a dummy GitHub app (the button and
# the redirect to github.com, through OpenTofu's oauth.env). The full flow
# runs against a local Forgejo (compose.test.yaml), the software behind
# Codeberg, enabled here as the "Codeberg" provider through
# config/forge-test.env.

section "Sign-in with a forge"
FG="http://git.$DOMAIN:3000"
# forgejo CMD…: the Forgejo CLI, as its git user.
forgejo() { remote "cd /opt/fodycloud && docker compose exec -T -u git forgejo forgejo $*"; }
# forgejo_authorize AUTHORIZE-URL USER PASSWORD -> the dashboard callback URL
# Forgejo redirects to: logs in to Forgejo and grants access, like a browser
# (from the VM, where Forgejo listens on 127.0.0.1:3000).
forgejo_authorize() {
  # shellcheck disable=SC2016 # expanded on the VM
  remote "bash -s $(printf '%q ' "$1" "$2" "$3" "$DOMAIN")" <<'SH' | sed "s#^https://dashboard\.$DOMAIN/#$D/#"
set -eu
auth="$1" user="$2" pw="$3" domain="$4"
F="http://git.$domain:3000" jar="$(mktemp)"
trap 'rm -f "$jar"' EXIT
c() { curl -s --max-time 30 --resolve "git.$domain:3000:127.0.0.1" -b "$jar" -c "$jar" "$@"; }
field() { grep -o "name=\"$1\" value=\"[^\"]*\"" | head -1 | sed 's/.*value="//; s/"$//'; }
loc="$(c -o /dev/null -w '%{redirect_url}' "$auth")"
if [[ "$loc" == *"/user/login"* ]]; then
  csrf="$(c "$F/user/login" | field _csrf)"
  c -o /dev/null --data-urlencode "_csrf=$csrf" --data-urlencode "user_name=$user" --data-urlencode "password=$pw" "$F/user/login"
  loc="$(c -o /dev/null -w '%{redirect_url}' "$auth")"
fi
if [[ "$loc" != *"/auth/codeberg/callback"* ]]; then # the consent page, the first time
  page="$(c "$auth")"
  args=()
  for f in _csrf client_id state scope nonce redirect_uri; do args+=(--data-urlencode "$f=$(field "$f" <<<"$page")"); done
  loc="$(c -o /dev/null -w '%{redirect_url}' "${args[@]}" --data-urlencode granted=true "$F/login/oauth/grant")"
fi
echo "$loc"
SH
}
# forge_flow JAR START-PATH FORGE-USER -> "<status> <redirect>" of the dashboard's callback
forge_flow() {
  local auth cb
  auth="$(as "$1" -X "${4:-GET}" "$D$2" | cut -d' ' -f2)"
  [[ "$auth" == "$FG/login/oauth/authorize?"* ]] || { echo "start went to: $auth"; return 1; }
  cb="$(forgejo_authorize "$auth" "$3" "$PASSWORD")"
  [[ "$cb" == "$D/auth/codeberg/callback?"* ]] || { echo "Forgejo went to: $cb"; return 1; }
  as "$1" "$cb"
}
psql_fc() { remote "cd /opt/fodycloud && docker compose exec -T postgres psql -U fodycloud -d fodycloud -tAc \"$1\""; }

# --- Through OpenTofu: GitHub (dummy app) shows, Codeberg not configured yet.
LOGIN_PAGE="$(curl -sk "${RES[@]}" "$D/login")"
check "the login page offers GitHub (oauth_github in test.tfvars)" has "$LOGIN_PAGE" 'href="/auth/github/start">Continue with GitHub'
check "no Codeberg button before it is configured" bash -c '[[ "$1" != *"Continue with Codeberg"* ]]' _ "$LOGIN_PAGE"
check "a provider that isn't configured has no routes" bash -c "[[ \$(curl -sk ${RES[*]} -o /dev/null -w '%{http_code}' $D/auth/codeberg/start) == 404 ]]"
GH="$(as "$(mktemp)" "$D/auth/github/start")"
check "GitHub start goes to github.com with PKCE and the callback" bash -c '[[ "$1" == "303 https://github.com/login/oauth/authorize?"* &&
  "$1" == *"code_challenge_method=S256"* && "$1" == *"redirect_uri=https%3A%2F%2Fdashboard.'"$DOMAIN"'%2Fauth%2Fgithub%2Fcallback"* ]]' _ "$GH"
check "the client secret stays on the server" bash -c '[[ "$1" != *client_secret* ]]' _ "$GH"

# --- Enable the local Forgejo as "Codeberg": a forge user, an OAuth app.
G1="e2e-g1-$RANDOM" G2="e2e-g2-$RANDOM"
G1_EMAIL="$G1@$DOMAIN" C_EMAIL="student-c-$RANDOM@$DOMAIN"
check "Forgejo is up" remote "for _ in \$(seq 1 60); do curl -sf http://127.0.0.1:3000/api/healthz >/dev/null && exit 0; sleep 2; done; exit 1"
forgejo admin user create --username "$G1" --password "'$PASSWORD'" --email "$G1_EMAIL" --must-change-password=false >/dev/null
# G2's email belongs to an existing FodyCloud account (password sign-up).
forgejo admin user create --username "$G2" --password "'$PASSWORD'" --email "$C_EMAIL" --must-change-password=false >/dev/null
fc admin create-user --email "$C_EMAIL" >/dev/null
OAPP="$(remote "curl -s -u '$G1:$PASSWORD' -H 'Content-Type: application/json' http://127.0.0.1:3000/api/v1/user/applications/oauth2 \
  -d '{\"name\":\"fodycloud-e2e\",\"confidential_client\":true,\"redirect_uris\":[\"https://dashboard.$DOMAIN/auth/codeberg/callback\"]}'")"
check "OAuth app created on Forgejo" bash -c '[[ -n "$(jq -r ".client_secret // empty" <<<"$1")" ]]' _ "$OAPP"
remote "umask 077; printf 'FC_OAUTH_CODEBERG_CLIENT_ID=%s\nFC_OAUTH_CODEBERG_CLIENT_SECRET=%s\nFC_OAUTH_CODEBERG_URL=%s\n' \
  '$(jq -r .client_id <<<"$OAPP")' '$(jq -r .client_secret <<<"$OAPP")' '$FG' > /opt/fodycloud/config/forge-test.env"
check "the control plane restarts with Codeberg enabled" remote "cd /opt/fodycloud && docker compose --progress quiet up -d --wait --wait-timeout 120 control-plane"
check "the login page offers Codeberg" has "$(curl -sk "${RES[@]}" "$D/login")" "Continue with Codeberg"

# --- Sign up with Codeberg: terms first, then a verified account without a password.
GJ="$(mktemp)"
check "a bad state is refused" has "$(as "$GJ" "$D/auth/codeberg/callback?code=x&state=forged")" "invalid+or+has+expired"
check "sign up: Codeberg sends a new student to the terms" has "$(forge_flow "$GJ" /auth/codeberg/start "$G1")" "303 $D/auth/confirm"
check "the terms page shows the forge's email" has "$(curl -sk "${RES[@]}" -b "$GJ" "$D/auth/confirm")" "$G1_EMAIL"
check "the terms must be accepted" has "$(as "$GJ" -X POST "$D/auth/confirm")" "Accept+the+Terms"
check "accepting the terms creates the account and signs in" has "$(as "$GJ" --data accept_terms=1 "$D/auth/confirm")" "303 $D/apps"
check "the account is verified, has no password and recorded the terms" test \
  "$(psql_fc "SELECT email_verified_at IS NOT NULL, password_hash = '', terms_version <> '' FROM users WHERE email='$G1_EMAIL'")" = "t|t|t"
check "the forge account is linked by its ID, not its name" test \
  "$(psql_fc "SELECT i.provider, i.issuer, i.username FROM user_identities i JOIN users u ON u.id=i.user_id WHERE u.email='$G1_EMAIL'")" = "codeberg|$FG|$G1"
check "a password login fails for a forge-only account" has "$(sign_in "$(mktemp)" "$G1_EMAIL" "$PASSWORD")" "Invalid+email+or+password"
GJ2="$(mktemp)"
check "signing in again uses the same account (no terms page)" has "$(forge_flow "$GJ2" /auth/codeberg/start "$G1")" "303 $D/apps"
check "still one account for this forge user" test "$(psql_fc "SELECT count(*) FROM users WHERE email='$G1_EMAIL'")" = 1
check "the last sign-in method can't be disconnected" has "$(as "$GJ2" -X POST "$D/account/forges/codeberg/disconnect")" "Set+a+password+before"
check "set a password (recent sign-in)" has "$(as "$GJ2" --data-urlencode "password=$NEW_PW" --data-urlencode "password2=$NEW_PW" "$D/account/set-password")" "Password+set"
check "the password works now" has "$(sign_in "$(mktemp)" "$G1_EMAIL" "$NEW_PW")" "303 $D/apps"
check "with a password, Codeberg can be disconnected" has "$(as "$GJ2" -X POST "$D/account/forges/codeberg/disconnect")" "Codeberg+disconnected"
check "the student is told by email" mail_has "$G1_EMAIL" "disconnected" "can't log in"

# --- An email that a FodyCloud account already uses: no takeover, link instead.
CJ="$(mktemp)"
check "a forge account can't take over an existing account" has "$(forge_flow "$CJ" /auth/codeberg/start "$G2")" "An+account+already+uses"
check "no forge account was linked" test "$(psql_fc "SELECT count(*) FROM user_identities i JOIN users u ON u.id=i.user_id WHERE u.email='$C_EMAIL'")" = 0
sign_in "$CJ" "$C_EMAIL" "$PASSWORD" >/dev/null
check "connect Codeberg from the Account page" has "$(forge_flow "$CJ" /account/forges/codeberg/connect "$G2" POST)" "Codeberg+connected"
check "the student is told by email" mail_has "$C_EMAIL" "was connected" "$G2"
check "the Account page lists it" has "$(curl -sk "${RES[@]}" -b "$CJ" "$D/account")" "Connected as <strong>$G2</strong>"
check "now Codeberg signs this student in" has "$(forge_flow "$(mktemp)" /auth/codeberg/start "$G2")" "303 $D/apps"
check "connect needs a same-origin form" bash -c "[[ \$(curl -sk ${RES[*]} -b $CJ -o /dev/null -w '%{http_code}' -X POST $D/account/forges/codeberg/connect) == 403 ]]"

# --- Suspended students can't sign in with a forge either.
fc admin ban --email "$C_EMAIL" --reason e2e >/dev/null
check "a suspended student is refused" has "$(forge_flow "$(mktemp)" /auth/codeberg/start "$G2")" "This+account+is+suspended"
fc admin unban --email "$C_EMAIL" >/dev/null
