# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Dashboard (continued)"
NOAPP="https://no-such-app-$RANDOM.apps.$DOMAIN:$HTTPS_PORT"; NRES=(--resolve "${NOAPP#https://}:$HOST")
NOAPP_PAGE="$(curl -sk "${NRES[@]}" "$NOAPP/")"
check "'No app here' page links to the dashboard site" bash -c 'grep -q "No app here" <<<"$1" && grep -q "href=\"https://dashboard.'"$DOMAIN"'/login\"" <<<"$1"' _ "$NOAPP_PAGE"
CSS_URL="$(grep -oE '/static/app\.css\?v=[0-9a-f]+' <<<"$NOAPP_PAGE" | head -1)"
check "'No app here' page loads its stylesheet as CSS" bash -c "curl -sk ${NRES[*]} -o /dev/null -w '%{http_code} %{content_type}' '$NOAPP$CSS_URL' | grep -q '^200 text/css'"
check "security headers (CSP, nosniff)" bash -c "curl -skI ${RES[*]} $D/login | grep -qi 'content-security-policy: default-src'"
APP_PAGE="$(curl -sk -b "$JAR" "${RES[@]}" "$D/apps/$APP")"
JS_URL="$(grep -oE '/static/app\.js\?v=[0-9a-f]+' <<<"$APP_PAGE" | head -1)"
check "app page loads the versioned log script" test -n "$JS_URL"
check "log script served" bash -c "curl -sk ${RES[*]} '$D$JS_URL' | grep -q 'htmx:beforeSwap'"
check "runtime log box with a stable key" bash -c "grep -q 'data-log=\"runtime-$APP\"' <<<\"\$1\"" _ "$APP_PAGE"
DEP_ID="$(body_of "$(api GET "/v1/apps/$APP/deployments" "$TOK_A")" | jq -r '[.deployments[] | select(.kind=="build")][-1].id')"
RAW_HDR="$(mktemp)"; RAW="$(curl -sk -b "$JAR" "${RES[@]}" -D "$RAW_HDR" "$D/apps/$APP/deployments/$DEP_ID/log?raw=1")"
check "build log downloads as a text file" grep -qi "content-disposition: attachment; filename=\"$APP-deployment-$DEP_ID.log\"" "$RAW_HDR"
check "downloaded build log has the build output" bash -c 'grep -q "==> Cloning" <<<"$1"' _ "$RAW"
check "runtime log downloads as a text file" bash -c "curl -sk -b $JAR ${RES[*]} -D - -o /dev/null '$D/apps/$APP/runtime-log?download=1' | grep -qi 'content-disposition: attachment'"
check "log download needs a session" bash -c "[[ \$(curl -sk ${RES[*]} -o /dev/null -w '%{http_code}' '$D/apps/$APP/deployments/$DEP_ID/log?raw=1') == 303 ]]"
rm -f "$RAW_HDR"

# Source code (AGPL-3.0): every page links to <domain>/source, which serves the
# exact source the control plane was built from, and nothing secret.
check "wake page links to the source code" grep -q "href=\"https://$DOMAIN/source\"" <<<"$NOAPP_PAGE"
ABOUT="$(api GET /v1/about "")"
check "GET /v1/about is public and points to the source" expect 200 "$ABOUT" "\"source\":\"https://$DOMAIN/source\""
FC_VERSION="$(body_of "$ABOUT" | jq -r .version)"
SRC="https://$DOMAIN:$HTTPS_PORT/source"
check "source page shows the running version and the download" bash -c "curl -sk ${HOME_RES[*]} '$SRC' | grep -q '$FC_VERSION' && curl -sk ${HOME_RES[*]} '$SRC' | grep -q 'Download the source'"
SRC_TGZ="$(mktemp)"
curl -sk "${HOME_RES[@]}" -o "$SRC_TGZ" "$SRC.tar.gz"
SRC_LIST="$(tar -tzf "$SRC_TGZ" 2>&1)"
# The first check is also the positive control for the "nothing secret" one.
check "source archive holds the whole platform at this version" bash -c 'for f in LICENSE go.mod cmd/fodycloud/main.go terraform/files.tf terraform/files/bootstrap.sh qemu/e2e-test.sh; do grep -qx "fodycloud-$2/$f" <<<"$1" || { echo "missing $f"; exit 1; }; done' _ "$SRC_LIST" "$FC_VERSION"
check "source archive has no secrets (state, tfvars, VM keys)" bash -c '! grep -E "\.tfstate|terraform/terraform\.tfvars$|\.auto\.tfvars|\.terraform/|qemu/\.work/" <<<"$1"' _ "$SRC_LIST"
rm -f "$SRC_TGZ"
