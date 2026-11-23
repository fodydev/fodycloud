# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Plan reminders"
fc admin set-plan --email "$B_EMAIL" --plan student --until "$(date -u -d '+2 days' +%F)" --note e2e >/dev/null
fc admin expire >/dev/null
check "plan-expiry reminder emailed" mailpit_text "$B_EMAIL" "plan ends"
fc admin revoke-plan --email "$B_EMAIL" >/dev/null
