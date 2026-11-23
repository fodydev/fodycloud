# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Repository validation"
check "private/missing GitHub repo rejected" expect 422 \
  "$(api POST /v1/apps "$TOK_A" '{"name":"e2e-private","repo_url":"https://github.com/fodydev/definitely-not-a-repo-xyz"}')" "not public"
check "unsupported provider rejected" expect 422 \
  "$(api POST /v1/apps "$TOK_A" '{"name":"e2e-bb","repo_url":"https://bitbucket.org/a/b"}')" "github.com, gitlab.com or codeberg.org"
check "missing branch rejected" expect 422 \
  "$(api POST /v1/apps "$TOK_A" "{\"name\":\"e2e-branch\",\"repo_url\":\"$SAMPLE_REPO\",\"branch\":\"no-such-branch\"}")" "branch not found"
check "reserved app name rejected" expect 422 \
  "$(api POST /v1/apps "$TOK_A" "{\"name\":\"admin\",\"repo_url\":\"$SAMPLE_REPO\"}")" "reserved"
check "read-only token cannot create apps" expect 403 \
  "$(api POST /v1/apps "$TOK_RO" "{\"name\":\"$APP\",\"repo_url\":\"$SAMPLE_REPO\"}")" "read-only"
