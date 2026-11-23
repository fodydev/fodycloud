#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# FodyCloud host bootstrap — converges a fresh Debian/Ubuntu VPS into a
# FodyCloud node. Shipped and run by Terraform; safe to re-run (idempotent).
#
#   bootstrap.sh <stage-dir>
#
# <stage-dir> mirrors the filesystem: <stage>/etc/..., <stage>/opt/fodycloud/...
set -euo pipefail

STAGE="${1:?usage: bootstrap.sh <stage-dir>}"
export DEBIAN_FRONTEND=noninteractive
log()  { printf '\n==> %s\n' "$*"; }
warn() { printf 'WARN: %s\n' "$*" >&2; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || die "must run as root"
# shellcheck source=/dev/null
source "$STAGE/opt/fodycloud/config/host.env"
# shellcheck source=/dev/null
source /etc/os-release
case "$ID" in debian|ubuntu) ;; *) die "unsupported OS: $ID (Debian 12/13 or Ubuntu 22.04/24.04 expected)";; esac

# --------------------------------------------------------------------------
# Install staged files; remember which ones changed.
# --------------------------------------------------------------------------
declare -A CHANGED=()
install_staged() {
  local rel src dst mode
  while IFS= read -r -d '' src; do
    rel="${src#"$STAGE"}"
    dst="$rel"
    mode="$(stat -c '%a' "$src")"
    if [[ ! -f "$dst" ]] || ! cmp -s "$src" "$dst" || [[ "$(stat -c '%a' "$dst")" != "$mode" ]]; then
      install -D -m "$mode" -o root -g root "$src" "$dst"
      CHANGED["$dst"]=1
      echo "   updated $dst"
    fi
  done < <(find "$STAGE" -type f -print0)
}
changed() { local p; for p in "$@"; do [[ -n "${CHANGED[$p]:-}" ]] && return 0; done; return 1; }

# --------------------------------------------------------------------------
log "Base packages"
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  ca-certificates curl gnupg jq nftables fail2ban python3-systemd unattended-upgrades \
  xfsprogs chrony apt-transport-https >/dev/null

log "Timezone: $TIMEZONE"
timedatectl set-timezone "$TIMEZONE"

# --------------------------------------------------------------------------
log "Docker data disk"
if [[ -n "$DATA_DISK_DEVICE" ]]; then
  [[ -b "$DATA_DISK_DEVICE" ]] || die "data disk $DATA_DISK_DEVICE not found"
  if ! mountpoint -q /var/lib/docker; then
    fstype="$(blkid -o value -s TYPE "$DATA_DISK_DEVICE" || true)"
    if [[ -z "$fstype" ]]; then
      if lsblk -nro MOUNTPOINT "$DATA_DISK_DEVICE" | grep -q .; then die "$DATA_DISK_DEVICE has mounted partitions"; fi
      echo "   formatting $DATA_DISK_DEVICE as XFS"
      mkfs.xfs -q -L fcdata "$DATA_DISK_DEVICE"
      fstype=xfs
    fi
    [[ "$fstype" == xfs ]] || die "$DATA_DISK_DEVICE contains a $fstype filesystem; refusing to touch it (XFS required)"
    if [[ -d /var/lib/docker ]] && [[ -n "$(ls -A /var/lib/docker 2>/dev/null)" ]]; then
      die "/var/lib/docker already has data on the root disk; migrate it manually before enabling the data disk"
    fi
    uuid="$(blkid -o value -s UUID "$DATA_DISK_DEVICE")"
    mkdir -p /var/lib/docker
    sed -i '\|[[:space:]]/var/lib/docker[[:space:]]|d' /etc/fstab
    echo "UUID=$uuid /var/lib/docker xfs defaults,noatime,pquota,nofail 0 2" >> /etc/fstab
    systemctl daemon-reload
    mount /var/lib/docker
  fi
  findmnt -no OPTIONS /var/lib/docker | grep -q prjquota || die "/var/lib/docker is mounted without project quotas (pquota)"
  echo "   /var/lib/docker on $DATA_DISK_DEVICE (xfs, pquota) — per-app disk quotas enabled"
else
  warn "no data disk configured: per-app disk quotas (--storage-opt size=) are unavailable"
fi

# --------------------------------------------------------------------------
log "Configuration files"
install_staged

# --------------------------------------------------------------------------
log "Kernel parameters"
modprobe br_netfilter 2>/dev/null || true
echo br_netfilter > /etc/modules-load.d/fodycloud.conf
if changed /etc/sysctl.d/90-fodycloud.conf; then sysctl -q --system; fi

# --------------------------------------------------------------------------
log "Swap (${SWAP_SIZE_GB}G)"
want=$(( SWAP_SIZE_GB * 1024 * 1024 * 1024 ))
have=$(stat -c %s /swapfile 2>/dev/null || echo 0)
if [[ "$have" != "$want" ]]; then
  if swapon --show=NAME --noheadings | grep -qx /swapfile; then swapoff /swapfile; fi
  rm -f /swapfile
  sed -i '\|^/swapfile |d' /etc/fstab
  if (( SWAP_SIZE_GB > 0 )); then
    fallocate -l "$want" /swapfile
    chmod 600 /swapfile
    mkswap -q /swapfile
    echo "/swapfile none swap sw 0 0" >> /etc/fstab
  fi
fi
if (( SWAP_SIZE_GB > 0 )) && ! swapon --show=NAME --noheadings | grep -qx /swapfile; then swapon /swapfile; fi

# --------------------------------------------------------------------------
log "Firewall (nftables table inet fodycloud)"
nft -c -f /etc/fodycloud/firewall.nft
systemctl daemon-reload
systemctl enable -q fodycloud-firewall.service
if changed /etc/fodycloud/firewall.nft /etc/systemd/system/fodycloud-firewall.service \
   || ! systemctl is-active -q fodycloud-firewall.service; then
  systemctl restart fodycloud-firewall.service
fi

# --------------------------------------------------------------------------
if changed /etc/systemd/resolved.conf.d/90-fodycloud.conf && systemctl is-active -q systemd-resolved; then
  systemctl restart systemd-resolved
fi

log "SSH hardening"
sshd -t || die "sshd configuration is invalid"
if changed /etc/ssh/sshd_config.d/10-fodycloud.conf; then systemctl reload ssh 2>/dev/null || systemctl reload sshd; fi

log "fail2ban"
systemctl enable -q fail2ban
if changed /etc/fail2ban/jail.d/fodycloud.local || ! systemctl is-active -q fail2ban; then systemctl restart fail2ban; fi

log "Unattended upgrades"
systemctl enable -q --now unattended-upgrades.service

# --------------------------------------------------------------------------
log "Docker Engine"
if ! command -v docker >/dev/null || ! docker compose version >/dev/null 2>&1; then
  install -d -m 0755 /etc/apt/keyrings
  curl -fsSL "https://download.docker.com/linux/$ID/gpg" -o /etc/apt/keyrings/docker.asc
  chmod a+r /etc/apt/keyrings/docker.asc
  cat > /etc/apt/sources.list.d/docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/$ID
Suites: $VERSION_CODENAME
Components: stable
Signed-By: /etc/apt/keyrings/docker.asc
EOF
  apt-get update -qq
  apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin >/dev/null
fi
systemctl enable -q docker.service containerd.service
if changed /etc/docker/daemon.json /etc/systemd/system/docker.service.d/10-fodycloud.conf || ! systemctl is-active -q docker; then
  systemctl restart docker
fi
docker version --format '   docker {{.Server.Version}} (API {{.Server.APIVersion}})'

# --------------------------------------------------------------------------
log "Docker networks"
ensure_network() {
  local name="$1"; shift
  if docker network inspect "$name" >/dev/null 2>&1; then
    echo "   $name exists"
  else
    docker network create "$@" "$name" >/dev/null
    echo "   $name created"
  fi
}
ensure_network fodycloud-edge --driver bridge -o com.docker.network.bridge.name=fc-edge
ensure_network fodycloud-apps --driver bridge -o com.docker.network.bridge.name=fc-apps \
  --subnet "$APPS_SUBNET" --ip-range "$APPS_IP_RANGE"
ensure_network fodycloud-build --driver bridge -o com.docker.network.bridge.name=fc-build
actual_subnet="$(docker network inspect fodycloud-apps -f '{{(index .IPAM.Config 0).Subnet}}')"
[[ "$actual_subnet" == "$APPS_SUBNET" ]] || warn "fodycloud-apps has subnet $actual_subnet, expected $APPS_SUBNET (recreate it manually)"

# --------------------------------------------------------------------------
log "Platform stack"
cd /opt/fodycloud
docker compose --progress quiet pull --ignore-buildable
# Not a compose service: the control plane runs it for apps' health checks.
docker pull --quiet "$(grep ^PROBE_IMAGE= .env | cut -d= -f2)" >/dev/null
echo "   building control plane (version $(grep ^CONTROL_PLANE_VERSION= .env | cut -d= -f2))"
docker compose --progress quiet build control-plane
docker compose --progress quiet up -d --remove-orphans --wait --wait-timeout 300
# pg_hba of the students' PostgreSQL server: compose doesn't notice changes
# to bind-mounted files, so reload it.
if changed /opt/fodycloud/config/apps-pg_hba.conf; then
  docker compose exec -T apps-postgres sh -c 'pg_ctl reload -D "$PGDATA"' >/dev/null || warn "apps-postgres: reload failed"
fi
# traefik.yml is Traefik's *static* configuration (entrypoints, ACME, metrics):
# it is only read at startup, and compose does not notice bind-mounted changes.
if changed /opt/fodycloud/traefik/traefik.yml /opt/fodycloud/traefik/dns.env /opt/fodycloud/traefik/acme-ca.pem; then
  echo "   restarting traefik (static configuration changed)"
  docker compose --progress quiet up -d --force-recreate --wait --wait-timeout 120 traefik
fi
docker compose ps --format 'table {{.Service}}\t{{.Status}}'

# --------------------------------------------------------------------------
if [[ -s /opt/fodycloud/config/admin.env ]]; then
  log "Initial admin"
  # shellcheck source=/dev/null
  source /opt/fodycloud/config/admin.env
  if out="$(docker compose exec -T -e FC_NEW_PASSWORD="$ADMIN_PASSWORD" control-plane \
        fodycloud admin create-admin --email "$ADMIN_EMAIL" 2>&1)"; then
    echo "   $out"
  elif grep -q "already exists" <<<"$out"; then
    echo "   $ADMIN_EMAIL already exists"
  else
    die "could not create the admin account: $out"
  fi
fi

log "Backups, cleanup and health-check timers"
install -d -m 0700 /var/backups/fodycloud
systemctl enable -q --now fodycloud-backup.timer fodycloud-prune.timer fodycloud-health.timer

log "FodyCloud node ready"
