#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Platform health check, run every 15 minutes by fodycloud-health.timer.
#
# Emails the admins (through `fodycloud admin notify`) when a disk is almost
# full, memory is low, or a TLS certificate expires soon. An alert is re-sent
# at most every 24 hours while the problem lasts, and a "resolved" email
# follows when it is gone.
#
# Thresholds come from host.env; the same variables set in the environment
# win, which lets the tests force alerts:
#   ALERT_DISK_PERCENT=1 /opt/fodycloud/health.sh
set -euo pipefail

# Environment overrides, kept before host.env is sourced.
declare -A override=()
for v in ALERT_DISK_PERCENT ALERT_MEMORY_PERCENT ALERT_CERT_DAYS; do
  override[$v]="${!v:-}"
done
# shellcheck source=/dev/null # written by bootstrap.sh at install time
source /opt/fodycloud/config/host.env
for v in "${!override[@]}"; do
  [[ -n "${override[$v]}" ]] && printf -v "$v" '%s' "${override[$v]}"
done

STATE=/var/lib/fodycloud/health
RESEND_SECONDS=$((24 * 3600))
install -d -m 0700 "$STATE"
cd /opt/fodycloud

notify() {
  docker compose exec -T control-plane fodycloud admin notify --subject "$1" --body "$2" >/dev/null 2>&1 ||
    echo "WARNING: could not send the alert: $1" >&2
}

# alert KEY SUBJECT BODY: sends the alert unless it was sent in the last 24 h.
alert() {
  local file="$STATE/$1" now
  now="$(date +%s)"
  echo "ALERT: $2"
  if [[ -f "$file" ]] && (( now - $(cat "$file") < RESEND_SECONDS )); then
    return
  fi
  notify "[$(hostname)] $2" "$3"
  echo "$now" > "$file"
}

# resolved KEY SUBJECT: sends "resolved" once, if the alert had been sent.
resolved() {
  local file="$STATE/$1"
  [[ -f "$file" ]] || return 0
  notify "[$(hostname)] Resolved: $2" "This problem is gone."
  rm -f "$file"
}

check_disks() {
  local mount pct key seen=""
  for mount in / /var/lib/docker; do
    [[ -d "$mount" ]] || continue
    local dev
    dev="$(df --output=source "$mount" | tail -1)"
    [[ " $seen " == *" $dev "* ]] && continue # same filesystem as / (no data disk)
    seen+=" $dev"
    pct="$(df --output=pcent "$mount" | tail -1 | tr -dc '0-9')"
    key="disk$(tr '/' '-' <<<"$mount")"
    if (( pct >= ALERT_DISK_PERCENT )); then
      alert "$key" "Disk $mount is $pct% full" \
        "The disk holding $mount is $pct% full (alert threshold: $ALERT_DISK_PERCENT%). See docs/operations/incidents.md (The disk is full)."
    else
      resolved "$key" "Disk $mount is almost full"
    fi
  done
}

check_memory() {
  local total avail used
  total="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
  avail="$(awk '/^MemAvailable:/ {print $2}' /proc/meminfo)"
  used=$(( 100 - avail * 100 / total ))
  if (( used >= ALERT_MEMORY_PERCENT )); then
    alert memory "Memory is $used% used" \
      "Only $((avail / 1024)) MB of $((total / 1024)) MB are available (alert threshold: $ALERT_MEMORY_PERCENT% used). Check 'docker stats' for the largest apps."
  else
    resolved memory "Memory is almost full"
  fi
}

# check_certificates: every certificate Traefik obtained (acme-*.json in its
# volume) must stay valid for more than ALERT_CERT_DAYS days.
check_certificates() {
  local dir file domain cert end days now soon=""
  dir="$(docker volume inspect -f '{{.Mountpoint}}' fodycloud_acme 2>/dev/null)" || return 0
  now="$(date +%s)"
  for file in "$dir"/acme-*.json; do
    [[ -s "$file" ]] || continue
    while IFS=$'\t' read -r domain cert; do
      end="$(base64 -d <<<"$cert" | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)" || continue
      [[ -n "$end" ]] || continue
      days=$(( ($(date -d "$end" +%s) - now) / 86400 ))
      if (( days < ALERT_CERT_DAYS )); then
        soon+="  $domain: $days day(s) left"$'\n'
      fi
    done < <(jq -r '.[]?.Certificates[]? | [.domain.main, .certificate] | @tsv' "$file")
  done
  if [[ -n "$soon" ]]; then
    alert certificates "Certificates expire soon" \
      "These certificates expire in less than $ALERT_CERT_DAYS days and haven't been renewed:"$'\n'"$soon"$'\n'"See docs/operations/addresses-and-tls.md (Troubleshooting)."
  else
    resolved certificates "Certificates expire soon"
  fi
}

check_disks
check_memory
check_certificates
echo "health check done"
