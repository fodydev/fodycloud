# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Dashboard"
JAR="$(mktemp)"; trap 'rm -f "$JAR"' EXIT
D="https://dashboard.$DOMAIN:$HTTPS_PORT"; RES=(--resolve "dashboard.$DOMAIN:$HTTPS_PORT:$HOST")
HOME_RES=(--resolve "$DOMAIN:$HTTPS_PORT:$HOST")
check "home page on the apex domain" bash -c "curl -sk ${HOME_RES[*]} https://$DOMAIN:$HTTPS_PORT/ | grep -q 'Your code. Your deployment. Your cloud.'"
docs_get() { curl -sk "${HOME_RES[@]}" -w '\n%{http_code}' "https://$DOMAIN:$HTTPS_PORT/docs$1"; }
check "documentation index is public" bash -c '[[ "$(tail -1 <<<"$1")" == 200 ]] && grep -q "User guide" <<<"$1"' _ "$(docs_get /)"
check "user guide page served" bash -c '[[ "$(tail -1 <<<"$1")" == 200 ]] && grep -q "Getting started" <<<"$1"' _ "$(docs_get /user/getting-started)"
DEV_DOC="$(docs_get /developer/overview)"
check "developer guide shows this platform's API address" bash -c "grep -q 'https://api.$DOMAIN/v1' <<<\"\$1\" && ! grep -q 'fodycloud.example' <<<\"\$1\"" _ "$DEV_DOC"
check "plans page shows the configured plans" bash -c 'grep -q "<td>free</td>" <<<"$1"' _ "$(docs_get /user/plans-and-limits)"
check "unknown documentation page -> 404" bash -c '[[ "$(tail -1 <<<"$1")" == 404 ]]' _ "$(docs_get /user/no-such-page)"
check "login without same-origin header refused (CSRF)" bash -c "[[ \$(curl -sk -o /dev/null -w '%{http_code}' ${RES[*]} --data 'email=$A_EMAIL&password=$PASSWORD' $D/login) == 403 ]]"
check "login sets a session and redirects to /apps" bash -c "curl -sk -c $JAR -o /dev/null -w '%{http_code} %{redirect_url}' ${RES[*]} -H 'Origin: $D' --data 'email=$A_EMAIL&password=$PASSWORD' $D/login | grep -q '^303 .*/apps'"
check "apps page lists the app" bash -c "curl -sk -b $JAR ${RES[*]} $D/apps | grep -q '$APP'"
check "session cookie is HttpOnly, Secure, host-only (__Host- prefix)" grep -qE '^#HttpOnly_dashboard\.[^[:space:]]+[[:space:]]FALSE[[:space:]]/[[:space:]]TRUE[[:space:]][0-9]+[[:space:]]__Host-fc_session' "$JAR"
check "signed-in hint cookie is set for the whole domain" grep -qE "^\.?${DOMAIN}[[:space:]]TRUE[[:space:]]/[[:space:]]TRUE[[:space:]][0-9]+[[:space:]]fc_signed_in[[:space:]]1" "$JAR"
check "signed-in visitor of the home page goes to the dashboard" bash -c "curl -sk -b $JAR ${HOME_RES[*]} -o /dev/null -w '%{http_code} %{redirect_url}' https://$DOMAIN:$HTTPS_PORT/ | grep -q '^303 https://dashboard.$DOMAIN/apps'"
check "the admin panel is not on the dashboard" bash -c "[[ \$(curl -sk -b $JAR ${RES[*]} -o /dev/null -w '%{http_code}' $D/admin) == 404 ]]"
DASH_DOC="$(curl -sk -b "$JAR" "${RES[@]}" -w '\n%{http_code}' "$D/docs/user/stacks")"
check "signed-in users read the guides inside the dashboard" bash -c '[[ "$(tail -1 <<<"$1")" == 200 ]] && grep -q "Choose your stack" <<<"$1" && grep -q "'"$A_EMAIL"'" <<<"$1"' _ "$DASH_DOC"
check "signed-in visitor of the public guides goes to the dashboard copy" bash -c "curl -sk -b $JAR ${HOME_RES[*]} -o /dev/null -w '%{http_code} %{redirect_url}' https://$DOMAIN:$HTTPS_PORT/docs/user/stacks | grep -q '^303 https://dashboard.$DOMAIN/docs/user/stacks'"
check "signed-out visitor of the dashboard guides goes to the public copy" bash -c "curl -sk ${RES[*]} -o /dev/null -w '%{http_code} %{redirect_url}' $D/docs/user/stacks | grep -q '^303 https://$DOMAIN/docs/user/stacks'"
check "dashboard 'Learn more' links stay in the dashboard" bash -c "curl -sk -b $JAR ${RES[*]} $D/apps/$APP | grep -q 'href=\"https://dashboard.$DOMAIN/docs/user/environment-variables\"'"
check "the API is not served on the dashboard" bash -c "[[ \$(curl -sk ${RES[*]} -o /dev/null -w '%{http_code}' -H 'Authorization: Bearer $TOK_A' $D/v1/me) == 404 ]]"
check "MCP is not served on the API site" bash -c "[[ \$(curl -sk --resolve api.$DOMAIN:$HTTPS_PORT:$HOST -o /dev/null -w '%{http_code}' -X POST https://api.$DOMAIN:$HTTPS_PORT/mcp) == 404 ]]"
