#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# End-to-end test of the FodyCloud control plane on the QEMU VM:
# accounts and tokens, repository validation, Railpack build, deployment with
# plan limits, admin plan assignment and expiry, env vars, isolation between
# users, the MCP server and the dashboard.
#
#   ./e2e-test.sh            (after run-vm.sh + tofu apply; takes ~20 minutes)
#
# The checks live in e2e/NN-*.sh, one file per area, sourced in order: later
# parts reuse the users, tokens and apps of earlier ones.
set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=qemu/testlib.sh
source "$HERE/testlib.sh"
SAMPLE_REPO="${SAMPLE_REPO:-https://github.com/heroku/node-js-getting-started}"
SAMPLE_MARKER="${SAMPLE_MARKER:-Getting Started}" # text the sample app's home page contains
APP="e2e-node"
# shellcheck disable=SC2034 # used by the e2e/ parts
A_EMAIL="student-a@$DOMAIN" B_EMAIL="student-b@$DOMAIN" PASSWORD="e2e-password-123"

# shellcheck source=qemu/e2e/lib.sh
source "$HERE/e2e/lib.sh"
command -v jq >/dev/null || { echo "jq is required"; exit 1; }

for part in "$HERE"/e2e/[0-9][0-9]-*.sh; do
  # shellcheck source=/dev/null
  source "$part"
done
finish
