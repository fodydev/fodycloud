#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Daily backup:
#   1. local gzip dumps in /var/backups/fodycloud (kept BACKUP_RETENTION_DAYS)
#      of the platform's PostgreSQL and of the students' (apps-postgres);
#   2. when configured, an encrypted off-site copy with restic (S3-compatible)
#      of both dumps and of the apps' persistent volumes (fc-vol-*).
set -euo pipefail
# shellcheck source=/dev/null # written by bootstrap.sh at install time
source /opt/fodycloud/config/host.env
cd /opt/fodycloud
DEST=/var/backups/fodycloud
install -d -m 0700 "$DEST"

notify() { docker compose exec -T control-plane fodycloud admin notify --subject "$1" --body "$2" >/dev/null 2>&1 || true; }
fail() { echo "ERROR: $1" >&2; notify "Backup failed on $(hostname)" "$1"; exit 1; }
trap 'fail "backup.sh failed at line $LINENO"' ERR

STAMP="$(date +%Y%m%d-%H%M%S)"
OUT="$DEST/postgres-$STAMP.sql.gz"
docker compose exec -T postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' | gzip -9 > "$OUT.partial"
mv "$OUT.partial" "$OUT"
chmod 0600 "$OUT"
APPS_OUT="$DEST/apps-postgres-$STAMP.sql.gz"
docker compose exec -T apps-postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' | gzip -9 > "$APPS_OUT.partial"
mv "$APPS_OUT.partial" "$APPS_OUT"
chmod 0600 "$APPS_OUT"
find "$DEST" \( -name 'postgres-*.sql.gz' -o -name 'apps-postgres-*.sql.gz' \) -mtime +"$BACKUP_RETENTION_DAYS" -delete
echo "Local backups written: $OUT ($(du -h "$OUT" | cut -f1)), $APPS_OUT ($(du -h "$APPS_OUT" | cut -f1))"

[[ -s /opt/fodycloud/config/backup.env ]] || { echo "Off-site backup not configured."; exit 0; }
trap - ERR
# The apps' volumes are visible read-only under /volumes.
restic() {
  docker run --rm -i --env-file /opt/fodycloud/config/backup.env --hostname fodycloud \
    ${RESTIC_NETWORK:+--network "$RESTIC_NETWORK"} \
    -v fodycloud-restic-cache:/root/.cache/restic \
    -v /var/lib/docker/volumes:/volumes:ro \
    "$RESTIC_IMAGE" "$@"
}
# Create the repository on first use. `init` answers at once whether or not it
# exists; probing with `restic cat config` instead makes restic retry for many
# minutes against a repository that does not exist yet.
if ! out="$(restic init 2>&1)"; then
  grep -qiE "already (exists|initialized)" <<<"$out" || fail "restic init failed: $out"
fi
zcat "$OUT" | restic backup --quiet --tag postgres --stdin --stdin-filename postgres-all.sql || fail "restic backup failed"
zcat "$APPS_OUT" | restic backup --quiet --tag apps-postgres --stdin --stdin-filename apps-postgres-all.sql || fail "restic backup of the apps' databases failed"
mapfile -t volumes < <(docker volume ls --quiet --filter label=fodycloud.service-of)
if (( ${#volumes[@]} > 0 )); then
  restic backup --quiet --tag volumes "${volumes[@]/#//volumes/}" || fail "restic backup of the apps' volumes failed"
fi
for tag in postgres apps-postgres volumes; do
  restic forget --quiet --tag "$tag" --keep-daily 7 --keep-weekly 4 --keep-monthly 6 --prune || fail "restic forget/prune ($tag) failed"
done
if [[ "$(date +%u)" == 7 || "${FORCE_CHECK:-}" == 1 ]]; then
  restic check || fail "restic check reported problems in the backup repository"
fi
echo "Off-site backup done ($(restic snapshots --tag postgres --json | grep -o '"short_id"' | wc -l) snapshot(s) kept)."
