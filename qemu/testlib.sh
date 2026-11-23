# shellcheck shell=bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Shared by smoke-test.sh and e2e-test.sh (sourced, needs $HERE).
#
# - remote, check, section and finish are the test vocabulary;
# - connection settings come from .work/net.env (written by run-vm.sh);
#   variables already set in the environment win;
# - the VM is locked for the duration of the suite (run-vm.sh / destroy-vm.sh
#   refuse to touch it), released automatically when the suite exits;
# - every section checks that the VM was not rebooted or replaced meanwhile.

if [[ -f "$HERE/.work/net.env" ]]; then
  while IFS='=' read -r k v; do
    [[ "$k" =~ ^[A-Z_]+$ ]] && declare "NETENV_$k=$v"
  done < "$HERE/.work/net.env"
fi
NET="${NET:-${NETENV_NET:-user}}"
HOST="${HOST:-${NETENV_HOST:-127.0.0.1}}"
SSH_PORT="${SSH_PORT:-${NETENV_SSH_PORT:-2222}}"
HTTP_PORT="${HTTP_PORT:-${NETENV_HTTP_PORT:-8080}}"
HTTPS_PORT="${HTTPS_PORT:-${NETENV_HTTPS_PORT:-8443}}"
SSH_KEY="${SSH_KEY:-$HERE/.work/id_ed25519}"
DOMAIN="${DOMAIN:-fodycloud.internal}"

exec 9>"$HERE/.work/test.lock"
if ! flock -n 9; then
  echo "Another test suite is already using the VM." >&2
  exit 1
fi

_vm_boot_id() {
  ssh -q -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR "root@$HOST" cat /proc/sys/kernel/random/boot_id 2>/dev/null
}
VM_BOOT_ID="$(_vm_boot_id)"
if [[ -z "$VM_BOOT_ID" ]]; then
  echo "Cannot reach the test VM over SSH at $HOST:$SSH_PORT (network: $NET)." >&2
  exit 1
fi
echo "Testing $HOST (network: $NET, domain: $DOMAIN)"

# valid_cert HOSTNAME: the certificate verifies against the local test CA (no -k).
CA_ROOT="$HERE/.work/ca/root.crt"
valid_cert() {
  for _ in $(seq 1 45); do # the first certificate is issued asynchronously after the router appears
    curl -sf --max-time 5 --cacert "$CA_ROOT" --resolve "$1:$HTTPS_PORT:$HOST" -o /dev/null "https://$1:$HTTPS_PORT/" && return 0
    sleep 2
  done
  curl -sS --max-time 5 --cacert "$CA_ROOT" --resolve "$1:$HTTPS_PORT:$HOST" -o /dev/null "https://$1:$HTTPS_PORT/" 2>&1 | tail -1
  return 1
}
issuer_of() { echo | openssl s_client -connect "$HOST:$HTTPS_PORT" -servername "$1" 2>/dev/null | openssl x509 -noout -issuer; }

# section NAME: print a header after checking the VM is still the same boot.
section() {
  local now
  now="$(_vm_boot_id)"
  if [[ "$now" != "$VM_BOOT_ID" ]]; then
    echo "ABORT: the test VM was rebooted, replaced or became unreachable during the run (boot id ${VM_BOOT_ID:0:8} -> ${now:0:8}). Results are invalid." >&2
    exit 3
  fi
  echo "== $*"
}

SSH_OPTS=(-q -i "$SSH_KEY" -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR)

# remote CMD…: runs a shell command on the VM as root. The command is built
# here, so local variables expand before it is sent.
# shellcheck disable=SC2029
remote() { ssh "${SSH_OPTS[@]}" "root@$HOST" "$@"; }

# check NAME CMD…: runs CMD, prints PASS or FAIL (with the last lines of its
# output), and counts the result.
PASS=0 FAIL=0
check() {
  local name="$1"; shift
  local out line
  if out="$("$@" 2>&1)"; then
    printf '  \033[32mPASS\033[0m %s\n' "$name"; PASS=$((PASS + 1))
  else
    printf '  \033[31mFAIL\033[0m %s\n' "$name"; FAIL=$((FAIL + 1))
    if [[ -n "$out" ]]; then
      tail -8 <<<"$out" | while IFS= read -r line; do printf '         %s\n' "$line"; done
    fi
  fi
}

# finish: prints the summary; the exit status is 0 only when nothing failed.
finish() {
  echo
  echo "Result: $PASS passed, $FAIL failed"
  (( FAIL == 0 ))
}
