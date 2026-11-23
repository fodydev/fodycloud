#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# One-time host setup (run with sudo): a virtual network on which the test VM
# behaves like a real VPS on the internet.
#
#   sudo qemu/net-setup.sh                 # create (idempotent)
#   qemu/net-setup.sh [--user NAME]        # same, from a root shell (default user:
#                                          # the owner of this repository checkout)
#   sudo qemu/net-teardown.sh              # remove everything again
#
# Layout (TEST-NET-3, 203.0.113.0/24: documentation space, never routed on the
# real internet and outside the RFC1918 ranges FodyCloud treats as private):
#
#   your PC  ── bridge fcbr0 ──  203.0.113.1      gateway + NAT to the real internet ("the internet")
#                                203.0.113.2-5    extra client addresses for tests (curl --interface)
#                                203.0.113.10     the test VM = "the VPS" (fixed DHCP lease)
#            └─ tap fctap0  (owned by you, so QEMU runs without root)
#
# Everything is managed by NetworkManager ("shared" mode gives DHCP, DNS for the
# VM and NAT). It also:
#   - turns on NetworkManager's local dnsmasq resolver so that every
#     *.fodycloud.internal name resolves to the VM on your PC (wildcard DNS);
#   - creates the local test CA (qemu/ca-init.sh, name-constrained to
#     fodycloud.internal) and trusts it on this PC, so the test VM's
#     ACME-issued certificates are valid in curl and browsers.
set -euo pipefail

BRIDGE=fcbr0 TAP=fctap0
CON_BR=fodycloud-br CON_TAP=fodycloud-tap
GATEWAY=203.0.113.1 VM_IP=203.0.113.10 VM_MAC=52:54:00:fc:00:10
CLIENTS=(203.0.113.2 203.0.113.3 203.0.113.4 203.0.113.5)
DOMAIN=fodycloud.internal
DNSMASQ_CONF=/etc/NetworkManager/dnsmasq-shared.d/fodycloud.conf
DISPATCHER=/etc/NetworkManager/dispatcher.d/90-fodycloud-clients
NM_DNS_CONF=/etc/NetworkManager/conf.d/90-fodycloud-dns.conf
NM_DNSMASQ_CONF=/etc/NetworkManager/dnsmasq.d/fodycloud.conf
SYSTEM_CA=/usr/local/share/ca-certificates/fodycloud-test-ca.crt
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

[[ $EUID -eq 0 ]] || { echo "Run as root (sudo $0, or from a root shell)." >&2; exit 1; }
# The unprivileged user who will run QEMU and own the test CA files:
# --user NAME, else the sudo caller, else the owner of this repository checkout.
OWNER=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --user) OWNER="${2:?--user needs a name}"; shift 2 ;;
    -h|--help) sed -n '2,25p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1 (usage: $0 [--user NAME])" >&2; exit 2 ;;
  esac
done
OWNER="${OWNER:-${SUDO_USER:-$(stat -c %U "$HERE")}}"
OWNER_UID="$(id -u "$OWNER" 2>/dev/null)" || { echo "unknown user: $OWNER" >&2; exit 1; }
[[ "$OWNER_UID" != 0 ]] || { echo "Refusing to give the TAP device to root (QEMU must run unprivileged): use --user NAME." >&2; exit 1; }
echo ">> Setting up for user $OWNER (uid $OWNER_UID)"
command -v nmcli >/dev/null || { echo "NetworkManager (nmcli) is required." >&2; exit 1; }
[[ -x /usr/sbin/dnsmasq ]] || { echo "dnsmasq is required for NetworkManager shared mode (apt install dnsmasq-base)." >&2; exit 1; }
if ip -br addr | grep -q "203\.0\.113\." && ! nmcli -t -f NAME con show | grep -qx "$CON_BR"; then
  echo "203.0.113.0/24 is already used on this machine; refusing to continue." >&2; exit 1
fi

echo ">> DHCP lease and DNS for the VM ($DNSMASQ_CONF)"
install -d -m 0755 "$(dirname "$DNSMASQ_CONF")"
cat > "$DNSMASQ_CONF" <<EOF
# Managed by fodycloud qemu/net-setup.sh — only used by NetworkManager's shared
# dnsmasq on $BRIDGE (the VM's DHCP/DNS); the host resolver is not affected.
dhcp-host=$VM_MAC,$VM_IP,fodycloud-test,infinite
address=/$DOMAIN/$VM_IP
EOF

# NetworkManager "shared" mode keeps a single address, so the extra client
# addresses are added by a dispatcher hook whenever the bridge comes up.
echo ">> Extra client addresses ${CLIENTS[*]} ($DISPATCHER)"
cat > "$DISPATCHER" <<EOF
#!/bin/sh
# Managed by fodycloud qemu/net-setup.sh: extra "internet client" addresses on $BRIDGE.
[ "\$1" = "$BRIDGE" ] && [ "\$2" = "up" ] || exit 0
for ip in ${CLIENTS[*]}; do /usr/sbin/ip addr replace "\$ip/32" dev $BRIDGE; done
EOF
chmod 0755 "$DISPATCHER"

echo ">> Bridge $BRIDGE ($CON_BR): $GATEWAY/24, shared to the internet"
if nmcli -t -f NAME con show | grep -qx "$CON_BR"; then
  nmcli con modify "$CON_BR" ipv4.method shared ipv4.addresses "$GATEWAY/24" ipv6.method disabled bridge.stp no
else
  nmcli con add type bridge ifname "$BRIDGE" con-name "$CON_BR" \
    ipv4.method shared ipv4.addresses "$GATEWAY/24" ipv6.method disabled \
    bridge.stp no connection.autoconnect yes >/dev/null
fi

echo ">> TAP $TAP ($CON_TAP), owned by uid $OWNER_UID, attached to $BRIDGE"
if nmcli -t -f NAME con show | grep -qx "$CON_TAP"; then
  nmcli con modify "$CON_TAP" tun.mode tap tun.owner "$OWNER_UID" tun.vnet-hdr yes tun.multi-queue no \
    connection.master "$BRIDGE" connection.slave-type bridge
else
  nmcli con add type tun ifname "$TAP" con-name "$CON_TAP" \
    tun.mode tap tun.owner "$OWNER_UID" tun.vnet-hdr yes tun.multi-queue no \
    connection.master "$BRIDGE" connection.slave-type bridge connection.autoconnect yes >/dev/null
fi

nmcli con up "$CON_BR" >/dev/null
nmcli con up "$CON_TAP" >/dev/null
for ip in "${CLIENTS[@]}"; do ip addr replace "$ip/32" dev "$BRIDGE"; done # in case the hook ran before this script finished

echo ">> Checks"
ip -br addr show "$BRIDGE"
if ip -d link show "$TAP" | grep -q "tun type tap.*user $OWNER_UID\|user $OWNER_UID"; then
  echo "   $TAP is a TAP owned by uid $OWNER_UID"
else
  ip -d link show "$TAP"
  echo "WARNING: could not confirm TAP ownership" >&2
fi
# Recent NetworkManager enables forwarding per interface (bridge + uplink), not globally.
if [[ "$(cat /proc/sys/net/ipv4/ip_forward)" == 1 || "$(cat /proc/sys/net/ipv4/conf/$BRIDGE/forwarding)" == 1 ]]; then
  echo "   IPv4 forwarding enabled on $BRIDGE (NAT by NetworkManager)"
else
  echo "WARNING: IPv4 forwarding is off on $BRIDGE; the VM will not reach the internet" >&2
fi
echo ">> Wildcard DNS on this PC: *.$DOMAIN -> $VM_IP (NetworkManager dnsmasq)"
install -d -m 0755 "$(dirname "$NM_DNS_CONF")" "$(dirname "$NM_DNSMASQ_CONF")"
cat > "$NM_DNS_CONF" <<EOF
# Managed by fodycloud qemu/net-setup.sh: use NetworkManager's local dnsmasq
# resolver (needed for the *.$DOMAIN wildcard). Remove with net-teardown.sh.
[main]
dns=dnsmasq
EOF
cat > "$NM_DNSMASQ_CONF" <<EOF
# Managed by fodycloud qemu/net-setup.sh: the test VM answers for every name under $DOMAIN.
address=/$DOMAIN/$VM_IP
EOF
if ! grep -q '^nameserver 127\.0\.0\.1' /etc/resolv.conf; then
  echo "   restarting NetworkManager to switch to its dnsmasq resolver (network drops for a few seconds)"
  systemctl restart NetworkManager
  for _ in $(seq 1 30); do grep -q '^nameserver 127\.0\.0\.1' /etc/resolv.conf && break; sleep 1; done
  nmcli con up "$CON_BR" >/dev/null 2>&1 || true
  nmcli con up "$CON_TAP" >/dev/null 2>&1 || true
else
  nmcli general reload dns-full
fi
for ip in "${CLIENTS[@]}"; do ip addr replace "$ip/32" dev "$BRIDGE"; done
getent hosts "anything.apps.$DOMAIN" | grep -q "$VM_IP" && echo "   anything.apps.$DOMAIN -> $VM_IP" \
  || echo "WARNING: wildcard DNS not active yet (check /etc/resolv.conf points to 127.0.0.1)" >&2

echo ">> Local test CA (trusted by this PC, valid only for $DOMAIN)"
runuser -u "$OWNER" -- "$HERE/ca-init.sh"
install -m 0644 "$HERE/.work/ca/root.crt" "$SYSTEM_CA"
update-ca-certificates >/dev/null 2>&1 && echo "   added to the system trust store (curl, wget, Go, Python…)"
OWNER_HOME="$(getent passwd "$OWNER" | cut -d: -f6)"
if command -v certutil >/dev/null && [[ -d "$OWNER_HOME/.pki/nssdb" ]]; then
  runuser -u "$OWNER" -- certutil -d "sql:$OWNER_HOME/.pki/nssdb" -D -n "FodyCloud Test CA" >/dev/null 2>&1 || true
  runuser -u "$OWNER" -- certutil -d "sql:$OWNER_HOME/.pki/nssdb" -A -t "C,," -n "FodyCloud Test CA" -i "$HERE/.work/ca/root.crt" \
    && echo "   added to Chromium/Chrome (NSS database)"
else
  echo "   Chromium/Chrome: install libnss3-tools and re-run this script, or import qemu/.work/ca/root.crt manually"
fi
echo "   Firefox: Settings > Privacy & Security > Certificates > View Certificates > Authorities > Import"
echo "            qemu/.work/ca/root.crt and tick \"Trust this CA to identify websites\""
echo
echo "Done. Start the VM with qemu/run-vm.sh; it is reachable as https://dashboard.$DOMAIN"
echo "(/etc/hosts lines for $DOMAIN are no longer needed; remove old ones pointing to 127.0.0.1)."
