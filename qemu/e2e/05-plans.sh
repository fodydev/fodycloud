# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Admin plan assignment (no payment)"
check "admin assigns 'student' with CLI" fc admin set-plan --email "$A_EMAIL" --plan student --note e2e
check "limits updated live (1 cpu, 1g, 512 pids)" bash -c "[[ '$(limits_of)' == '1000000000 1073741824 512' ]]"
check "non-admin cannot assign plans via API" expect 403 \
  "$(api PUT "/v1/admin/users/$A_EMAIL/plan" "$TOK_A" '{"plan":"research"}')"
remote "cd /opt/fodycloud && docker compose exec -T postgres psql -U fodycloud -d fodycloud -qc \"UPDATE subscriptions SET valid_until = now() - interval '1 minute'\"" >/dev/null
check "expired subscription falls back to free" fc admin expire
check "limits back to free after expiry" bash -c "[[ '$(limits_of)' == '250000000 268435456 128' ]]"
check "plan change is audited" remote "cd /opt/fodycloud && docker compose exec -T postgres psql -U fodycloud -d fodycloud -tAc \
  \"SELECT count(*) FROM plan_changes p JOIN users u ON u.id=p.user_id WHERE u.email='$A_EMAIL' AND p.new_plan='student'\" | grep -qv '^0$'"
