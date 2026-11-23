# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Login rate limiting"
L_EMAIL="locked-$RANDOM@$DOMAIN"
# On the virtual network the host has extra "internet client" addresses, so
# these checks run from their own IPs and leave the main client untouched.
# login_try reads these arrays by name (local -n).
# shellcheck disable=SC2034
CLIENT_A=() CLIENT_B=() CLIENT_C=()
# shellcheck disable=SC2034
if [[ "$NET" == bridge ]]; then CLIENT_A=(--interface 203.0.113.3) CLIENT_B=(--interface 203.0.113.4) CLIENT_C=(--interface 203.0.113.5); fi
login_try() { local -n via="$1"; shift
  curl -sk --max-time 10 "${via[@]}" "${RES[@]}" -H "Origin: $D" -w '\n%{http_code}' --data-urlencode "email=$1" --data-urlencode "password=wrong-password-x" "$D/login"; }
for _ in 1 2 3 4 5; do login_try CLIENT_A "$L_EMAIL" >/dev/null; done
R="$(login_try CLIENT_A "$L_EMAIL")"
check "account locked after 5 failed logins (429)" bash -c '[[ "$(tail -1 <<<"$1")" == 429 ]] && grep -q "this account" <<<"$1"' _ "$R"
for i in $(seq 1 10); do login_try CLIENT_B "other-$i-$RANDOM@$DOMAIN" >/dev/null; done
R="$(login_try CLIENT_B "other-last-$RANDOM@$DOMAIN")"
check "too many logins from one address (429)" bash -c '[[ "$(tail -1 <<<"$1")" == 429 ]]' _ "$R"
if [[ "$NET" == bridge ]]; then
  R="$(login_try CLIENT_C "fresh-$RANDOM@$DOMAIN")"
  check "another address is not affected by that limit (normal login error, not 429)" bash -c '[[ "$(tail -1 <<<"$1")" == 303 ]]' _ "$R"
fi
