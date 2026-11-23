#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Inspect and restore FodyCloud off-site backups (restic).
#
#   restore.sh snapshots                 list backups
#   restore.sh dump [SNAPSHOT]           print the platform's pg_dumpall SQL dump (default: latest) to stdout
#   restore.sh apps-dump [SNAPSHOT]      print the students' databases dump (apps-postgres) to stdout
#   restore.sh volume APP [SNAPSHOT]     restore an app's volume into /var/backups/fodycloud/restore/APP
#   restore.sh check                     verify the repository
#
# Full recovery is described in docs/restore.md.
set -euo pipefail
# shellcheck source=/dev/null # written by bootstrap.sh at install time
source /opt/fodycloud/config/host.env
[[ -s /opt/fodycloud/config/backup.env ]] || { echo "off-site backup is not configured" >&2; exit 1; }
restic() {
  docker run --rm -i --env-file /opt/fodycloud/config/backup.env --hostname fodycloud \
    ${RESTIC_NETWORK:+--network "$RESTIC_NETWORK"} -v fodycloud-restic-cache:/root/.cache/restic \
    -v /var/backups/fodycloud/restore:/restore \
    "$RESTIC_IMAGE" "$@"
}
case "${1:-}" in
  snapshots) restic snapshots ;;
  dump)      restic dump --tag postgres "${2:-latest}" /postgres-all.sql ;;
  apps-dump) restic dump --tag apps-postgres "${2:-latest}" /apps-postgres-all.sql ;;
  volume)
    app="${2:?usage: restore.sh volume APP [SNAPSHOT]}"
    [[ "$app" =~ ^[a-z][a-z0-9-]{1,30}[a-z0-9]$ ]] || { echo "invalid app name" >&2; exit 2; }
    install -d -m 0700 /var/backups/fodycloud/restore
    restic restore --tag volumes "${3:-latest}" --target /restore --include "/volumes/fc-vol-$app"
    echo "Restored into /var/backups/fodycloud/restore/volumes/fc-vol-$app/_data" ;;
  check)     restic check ;;
  *) sed -n '2,10p' "$0"; exit 2 ;;
esac
