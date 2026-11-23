# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Egress restrictions (spam, mining)"
remote 'docker rm -f fc-test-egress >/dev/null 2>&1; docker run -d --name fc-test-egress --network fodycloud-apps alpine:3 sleep 300 >/dev/null'
check "control: an app can reach portquiz.net:8080" remote "docker exec fc-test-egress nc -z -w 5 portquiz.net 8080"
check "SMTP port 25 blocked from apps" remote "! docker exec fc-test-egress nc -z -w 5 portquiz.net 25"
check "mining-pool port 3333 blocked from apps" remote "! docker exec fc-test-egress nc -z -w 5 portquiz.net 3333"
remote 'docker rm -f fc-test-egress >/dev/null'
