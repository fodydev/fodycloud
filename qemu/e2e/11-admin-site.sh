# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Admin site"
ADM="https://admin.$DOMAIN:$HTTPS_PORT"; ARES=(--resolve "admin.$DOMAIN:$HTTPS_PORT:$HOST"); AJAR="$(mktemp)"
fc admin create-admin --email "e2e-admin@$DOMAIN" >/dev/null 2>&1 || true
DUP_ADMIN="$(fc admin create-admin --email "$A_EMAIL" 2>&1)"
check "an admin can't be created with a student's email" bash -c 'grep -q "already exists" <<<"$1" && ! grep -q "created admin" <<<"$1"' _ "$DUP_ADMIN"
adm() { curl -sk --max-time 30 "${ARES[@]}" -b "$AJAR" -c "$AJAR" -H "Origin: $ADM" -w '\n%{http_code} %{redirect_url}' "$@"; }
check "admin panel asks for the admin sign-in" bash -c 'grep -q "Sign in to the admin panel" <<<"$1"' _ "$(adm "$ADM/")"
check "admin guide asks for the admin sign-in" bash -c 'grep -q "Sign in to the admin panel" <<<"$1" && ! grep -q "Assign a plan" <<<"$1"' _ "$(adm "$ADM/docs/plans")"
check "a dashboard session doesn't open the admin panel" bash -c "curl -sk ${ARES[*]} -b '__Host-fc_admin=$(awk '/__Host-fc_session/{print $7}' "$JAR")' $ADM/ | grep -q 'Sign in to the admin panel'"
check "non-admin sign-in refused like a wrong password" bash -c 'grep -q "Invalid+email+or+password" <<<"$1"' _ \
  "$(adm -o /dev/null --data-urlencode "email=$B_EMAIL" --data-urlencode "password=$PASSWORD" "$ADM/login")"
check "admin sign-in opens the panel" bash -c '[[ "$(tail -1 <<<"$1")" =~ ^303\ https://admin\.'"${DOMAIN//./\\.}"'(:[0-9]+)?/$ ]]' _ \
  "$(adm -o /dev/null --data-urlencode "email=e2e-admin@$DOMAIN" --data-urlencode "password=$PASSWORD" "$ADM/login")"
check "admin session cookie is host-only (__Host-fc_admin)" grep -qE '^#HttpOnly_admin\.[^[:space:]]+[[:space:]]FALSE[[:space:]]/[[:space:]]TRUE[[:space:]][0-9]+[[:space:]]__Host-fc_admin' "$AJAR"
check "admin panel lists users" bash -c 'grep -q "Assign a plan" <<<"$1" && grep -q "'"$B_EMAIL"'" <<<"$1"' _ "$(adm "$ADM/")"
check "admin guide is served to admins" bash -c 'grep -q "Admin overview" <<<"$1" && grep -q "Assign a plan" <<<"$1"' _ "$(adm "$ADM/docs/plans")"
check "admin assigns a plan from the panel" bash -c 'grep -q "Plan+updated" <<<"$1"' _ \
  "$(adm -o /dev/null --data-urlencode "email=$B_EMAIL" --data-urlencode plan=student --data-urlencode note=e2e "$ADM/plan")"
check "plan visible through the API" bash -c 'jq -e ".plan.plan == \"student\"" <<<"$1" >/dev/null' _ "$(body_of "$(api GET /v1/me "$TOK_B")")"
fc admin revoke-plan --email "$B_EMAIL" >/dev/null
check "admin action without same-origin header refused" bash -c "[[ \$(curl -sk ${ARES[*]} -b $AJAR -o /dev/null -w '%{http_code}' --data 'email=x' $ADM/plan) == 403 ]]"
check "admin guide is not in the public guides" bash -c "[[ \$(curl -sk ${HOME_RES[*]} -o /dev/null -w '%{http_code}' https://$DOMAIN:$HTTPS_PORT/docs/admin/plans) == 404 ]]"
check "the panel's users table has no admin label (admins aren't users)" bash -c 'grep -q "Assign a plan" <<<"$1" && ! grep -qE "<td>[^<]*<span class=\"pill\">admin" <<<"$1"' _ "$(adm "$ADM/")"
NEW_TOK_PAGE="$(adm --data-urlencode name=e2e-panel "$ADM/tokens")"
PANEL_TOK="$(grep -oE 'fca_[A-Za-z0-9_-]{20,}' <<<"$NEW_TOK_PAGE" | head -1)"
check "admin creates an admin API token in the panel (shown once)" test -n "$PANEL_TOK"
check "the admin token works on the admin API" expect 200 "$(api GET /v1/admin/abuse "$PANEL_TOK")"
check "the admin token is refused on students' endpoints" expect 403 "$(api GET /v1/apps "$PANEL_TOK")" "admin tokens only work on /v1/admin"
check "the admin token is refused by MCP (401)" bash -c "[[ \$(curl -sk --resolve mcp.$DOMAIN:$HTTPS_PORT:$HOST -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer $PANEL_TOK' -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' --data '{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}' https://mcp.$DOMAIN:$HTTPS_PORT/mcp) == 401 ]]"
check "a student token is refused on the admin API" expect 403 "$(api GET /v1/admin/abuse "$TOK_A")" "admins only"
TOK_ID="$(grep -oE 'action="/tokens/[0-9]+/revoke"' <<<"$(adm "$ADM/")" | grep -oE '[0-9]+' | tail -1)"
check "admin revokes the admin token" bash -c 'grep -q "Admin+token+revoked" <<<"$1"' _ "$(adm -o /dev/null -X POST "$ADM/tokens/$TOK_ID/revoke")"
check "a revoked admin token stops working" expect 401 "$(api GET /v1/admin/abuse "$PANEL_TOK")"
ADMIN_ON_DASH="$(curl -sk "${RES[@]}" -H "Origin: $D" -o /dev/null -w '%{redirect_url}' --data-urlencode "email=e2e-admin@$DOMAIN" --data-urlencode "password=$PASSWORD" "$D/login")"
check "an admin can't sign in to the dashboard" bash -c 'grep -q "Invalid+email+or+password" <<<"$1"' _ "$ADMIN_ON_DASH"
ADMIN_SIGNUP="$(curl -sk "${RES[@]}" -H "Origin: $D" -o /dev/null -w '%{redirect_url}' --data-urlencode "email=e2e-admin@$DOMAIN" --data-urlencode "password=$PASSWORD" --data accept_terms=1 "$D/signup")"
check "nobody can sign up with an admin's email" bash -c 'grep -q "already+exists" <<<"$1"' _ "$ADMIN_SIGNUP"
check "the dashboard has no Admin link" bash -c "! curl -sk -b $JAR ${RES[*]} $D/apps | grep -q 'href=\"https://admin.$DOMAIN/\"'"
check "admin signs out" bash -c 'grep -q "Signed+out" <<<"$1"' _ "$(adm -o /dev/null -X POST "$ADM/logout")"
rm -f "$AJAR"
