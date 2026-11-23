#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Remove the virtual network created by net-setup.sh (run with sudo).
set -euo pipefail
[[ $EUID -eq 0 ]] || { echo "Run as root (sudo $0, or from a root shell)." >&2; exit 1; }
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OWNER="${1:-${SUDO_USER:-$(stat -c %U "$HERE")}}"   # optional: net-teardown.sh USER
for con in fodycloud-tap fodycloud-br; do
  if nmcli -t -f NAME con show | grep -qx "$con"; then
    nmcli con delete "$con" >/dev/null && echo ">> removed NetworkManager connection $con"
  fi
done
rm -f /etc/NetworkManager/dnsmasq-shared.d/fodycloud.conf /etc/NetworkManager/dispatcher.d/90-fodycloud-clients \
      /etc/NetworkManager/dnsmasq.d/fodycloud.conf
echo ">> removed the dnsmasq and dispatcher configuration"
if [[ -f /etc/NetworkManager/conf.d/90-fodycloud-dns.conf ]]; then
  rm -f /etc/NetworkManager/conf.d/90-fodycloud-dns.conf
  echo ">> restoring NetworkManager's default resolver (network drops for a few seconds)"
  systemctl restart NetworkManager
fi
if [[ -f /usr/local/share/ca-certificates/fodycloud-test-ca.crt ]]; then
  rm -f /usr/local/share/ca-certificates/fodycloud-test-ca.crt
  update-ca-certificates --fresh >/dev/null 2>&1 && echo ">> removed the test CA from the system trust store"
fi
if [[ "$OWNER" != root ]] && command -v certutil >/dev/null; then
  H="$(getent passwd "$OWNER" | cut -d: -f6)"
  runuser -u "$OWNER" -- certutil -d "sql:$H/.pki/nssdb" -D -n "FodyCloud Test CA" >/dev/null 2>&1 && echo ">> removed the test CA from Chromium"
fi
echo "Done. Remove the test CA from Firefox manually if you imported it (Authorities > FodyCloud test)."
echo "The CA itself stays in qemu/.work/ca/ (delete that directory to discard it)."
