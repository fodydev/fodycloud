#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Stop the local test VM and delete its disks (keeps the downloaded base image).
set -euo pipefail
WORK="${WORK:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/.work}"
cd "$WORK" 2>/dev/null || exit 0
if [[ -f test.lock ]] && ! flock -n test.lock true && [[ "${FORCE:-}" != 1 ]]; then
  echo "A test suite is using the VM; wait for it to finish, stop it, or use FORCE=1." >&2
  exit 1
fi
if [[ -f vm.pid ]]; then
  pid="$(cat vm.pid)"
  if kill -0 "$pid" 2>/dev/null; then
    kill "$pid"
    for _ in $(seq 1 20); do kill -0 "$pid" 2>/dev/null || break; sleep 0.5; done
  fi
fi
rm -rf vm.pid root.qcow2 data.qcow2 seed.iso seed console.log net.env
echo ">> VM destroyed"
