#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Boot a local Debian 13 VM in QEMU that mimics a fresh VPS, for testing the
# OpenTofu configuration in ../terraform.
#
#   ./run-vm.sh            # download image (once), create disks, boot VM in background
#   ./destroy-vm.sh        # stop VM and delete its disks
#
# Networking (NET=bridge|user, default: bridge when net-setup.sh was run):
#   bridge  the VM is 203.0.113.10 on the virtual network created by
#           `sudo qemu/net-setup.sh`, with real ports 22/80/443, like a VPS.
#   user    QEMU user-mode networking, no setup needed; ports forwarded on
#           127.0.0.1: 2222 -> 22 (ssh), 8080 -> 80, 8443 -> 443.
# The chosen values are written to .work/net.env for the test scripts.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORK="${WORK:-$HERE/.work}"
IMAGE_URL_BASE="https://cloud.debian.org/images/cloud/trixie/latest"
IMAGE_NAME="debian-13-genericcloud-amd64.qcow2"
VM_CPUS="${VM_CPUS:-4}"
VM_MEM="${VM_MEM:-4096}"
ROOT_SIZE="${ROOT_SIZE:-40G}"
DATA_SIZE="${DATA_SIZE:-20G}"
TAP=fctap0 VM_MAC=52:54:00:fc:00:10
if [[ -z "${NET:-}" ]]; then
  if ip link show "$TAP" >/dev/null 2>&1; then NET=bridge; else NET=user; fi
fi
case "$NET" in
  bridge)
    ip link show "$TAP" >/dev/null 2>&1 || { echo "TAP $TAP not found: run 'sudo qemu/net-setup.sh' first (or NET=user)." >&2; exit 1; }
    HOST=203.0.113.10 SSH_PORT=22 HTTP_PORT=80 HTTPS_PORT=443
    NETDEV=(-netdev "tap,id=n0,ifname=$TAP,script=no,downscript=no" -device "virtio-net-pci,netdev=n0,mac=$VM_MAC") ;;
  user)
    HOST=127.0.0.1 SSH_PORT="${SSH_PORT:-2222}" HTTP_PORT="${HTTP_PORT:-8080}" HTTPS_PORT="${HTTPS_PORT:-8443}"
    NETDEV=(-netdev "user,id=n0,hostfwd=tcp:127.0.0.1:${SSH_PORT}-:22,hostfwd=tcp:127.0.0.1:${HTTP_PORT}-:80,hostfwd=tcp:127.0.0.1:${HTTPS_PORT}-:443"
            -device "virtio-net-pci,netdev=n0,mac=$VM_MAC") ;;
  *) echo "NET must be 'bridge' or 'user'" >&2; exit 2 ;;
esac

mkdir -p "$WORK/image"
cd "$WORK"

if [[ -f test.lock ]] && ! flock -n test.lock true; then
  echo "A test suite is using the VM; wait for it to finish or stop it first." >&2
  exit 1
fi
if [[ -f vm.pid ]] && kill -0 "$(cat vm.pid)" 2>/dev/null; then
  echo "VM already running (pid $(cat vm.pid)). Run ./destroy-vm.sh first." >&2
  exit 1
fi

# 1. Base cloud image (downloaded once, checksum verified)
if [[ ! -f "image/$IMAGE_NAME.verified" ]]; then
  echo ">> Downloading $IMAGE_NAME"
  curl -fsSL --retry 3 -o "image/$IMAGE_NAME" "$IMAGE_URL_BASE/$IMAGE_NAME"
  curl -fsSL -o image/SHA512SUMS "$IMAGE_URL_BASE/SHA512SUMS"
  (cd image && grep " $IMAGE_NAME\$" SHA512SUMS | sha512sum -c -)
  touch "image/$IMAGE_NAME.verified"
fi

# 2. Disks: copy-on-write root overlay + empty data disk (becomes /var/lib/docker)
rm -f root.qcow2 data.qcow2 seed.iso
qemu-img create -q -f qcow2 -F qcow2 -b "$WORK/image/$IMAGE_NAME" root.qcow2 "$ROOT_SIZE"
qemu-img create -q -f qcow2 data.qcow2 "$DATA_SIZE"

# 3. Throwaway SSH key for the test VM
[[ -f id_ed25519 ]] || ssh-keygen -q -t ed25519 -N '' -C fodycloud-qemu-test -f id_ed25519

# 4. cloud-init NoCloud seed: emulates what a VPS provider gives you —
#    a root login with your SSH key and nothing else.
mkdir -p seed
cat > seed/meta-data <<EOF
instance-id: fodycloud-test-$(date +%s)
local-hostname: fodycloud-test
EOF
cat > seed/user-data <<EOF
#cloud-config
disable_root: false
ssh_pwauth: true
users:
  - name: root
    lock_passwd: false
    ssh_authorized_keys:
      - $(cat id_ed25519.pub)
chpasswd:
  expire: false
  users:
    - {name: root, password: insecure-test-password, type: text}
EOF
/usr/bin/genisoimage -quiet -output seed.iso -volid cidata -joliet -rock seed/user-data seed/meta-data

# 5. Boot
KVM_OPTS=(-cpu max)
if [[ -r /dev/kvm && -w /dev/kvm ]]; then KVM_OPTS=(-enable-kvm -cpu host); fi

qemu-system-x86_64 \
  -name fodycloud-test "${KVM_OPTS[@]}" \
  -machine q35 -smp "$VM_CPUS" -m "$VM_MEM" \
  -drive file=root.qcow2,if=virtio,format=qcow2 \
  -drive file=data.qcow2,if=virtio,format=qcow2 \
  -drive file=seed.iso,media=cdrom,readonly=on \
  "${NETDEV[@]}" \
  -display none -serial file:console.log \
  -daemonize -pidfile vm.pid

cat > net.env <<EOF
# Written by run-vm.sh: how to reach the current test VM.
NET=$NET
HOST=$HOST
SSH_PORT=$SSH_PORT
HTTP_PORT=$HTTP_PORT
HTTPS_PORT=$HTTPS_PORT
EOF
echo ">> VM started (pid $(cat vm.pid), network: $NET), console log: $WORK/console.log"
echo ">> Waiting for SSH on $HOST:$SSH_PORT ..."
for _ in $(seq 1 90); do
  if ssh -q -i id_ed25519 -p "$SSH_PORT" -o BatchMode=yes -o ConnectTimeout=3 \
       -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
       "root@$HOST" 'cloud-init status --wait >/dev/null 2>&1; true'; then
    echo ">> VM ready: ssh -i $WORK/id_ed25519 -p $SSH_PORT root@$HOST"
    exit 0
  fi
  sleep 2
done
echo "!! VM did not become reachable over SSH; see $WORK/console.log" >&2
exit 1
