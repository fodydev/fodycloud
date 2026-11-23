# shellcheck shell=bash disable=SC2153
# SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
# SPDX-License-Identifier: AGPL-3.0-or-later
# Part of e2e-test.sh (sourced in order). Variables such as TOK_A, JAR or
# RES come from e2e-test.sh and earlier parts (hence SC2153 is off here).

section "Off-site backups (restic to S3)"
check "backup job runs" remote "systemctl start fodycloud-backup.service"
check "a restic snapshot exists off-site" remote "/opt/fodycloud/restore.sh snapshots | grep -q postgres"
check "the apps' databases and volumes are backed up too" remote \
  "/opt/fodycloud/restore.sh snapshots | grep -q apps-postgres && /opt/fodycloud/restore.sh snapshots | grep -q volumes"
check "an app's volume can be restored" remote \
  "/opt/fodycloud/restore.sh volume e2e-store >/dev/null && grep -qx kept /var/backups/fodycloud/restore/volumes/fc-vol-e2e-store/_data/note.txt && rm -rf /var/backups/fodycloud/restore"
check "the apps' databases dump holds the app's data" remote \
  "/opt/fodycloud/restore.sh apps-dump | grep -q '^hello\$'"
remote 'docker rm -f fc-restore-test >/dev/null 2>&1; docker run -d --name fc-restore-test -e POSTGRES_PASSWORD=x postgres:17-alpine >/dev/null
  for _ in $(seq 1 60); do [ "$(docker logs fc-restore-test 2>&1 | grep -c "ready to accept connections")" -ge 2 ] && break; sleep 1; done
  /opt/fodycloud/restore.sh dump | docker exec -i fc-restore-test psql -q -U postgres -d postgres >/dev/null 2>&1'
LIVE="$(remote "cd /opt/fodycloud && docker compose exec -T postgres psql -U fodycloud -d fodycloud -tAc 'SELECT count(*) FROM users'")"
RESTORED="$(remote "docker exec fc-restore-test psql -U postgres -d fodycloud -tAc 'SELECT count(*) FROM users'" 2>&1)"
check "restore into a fresh Postgres matches (users: live $LIVE, restored $RESTORED)" test "$LIVE" = "$RESTORED"
remote 'docker rm -f fc-restore-test >/dev/null'
