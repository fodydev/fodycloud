# Backups and restore

FodyCloud backs up the platform database (accounts, apps, deployments, plans, env vars), the
students' databases (the `apps-postgres` server) and the apps' persistent volumes. Valkey caches
aren't backed up: they are in memory only.

| Where | What | Retention |
|---|---|---|
| `/var/backups/fodycloud/postgres-*.sql.gz` and `apps-postgres-*.sql.gz` on the server | daily `pg_dumpall` of each server, gzip | `backup_retention_days` (7) |
| Off-site restic repository (S3-compatible, encrypted) | the same dumps (tags `postgres`, `apps-postgres`) and the volumes `fc-vol-*` (tag `volumes`) | 7 daily, 4 weekly, 6 monthly |

The off-site copy is configured with the `backup_s3_*` variables of `terraform.tfvars`. **It
can't be decrypted without `restic_password`**, so keep a copy of it outside the server and
outside the OpenTofu state (`tofu output -raw restic_password`).

Backups run every night (`fodycloud-backup.timer`). Admins are emailed if a backup fails or if
the weekly `restic check` finds a problem.

## Check that backups work

```bash
ssh root@<server>
systemctl start fodycloud-backup.service && journalctl -u fodycloud-backup -n 20
/opt/fodycloud/restore.sh snapshots      # off-site snapshots
/opt/fodycloud/restore.sh check          # verify the repository
```

## Restore the database on the same server

Use this after data loss or a bad migration. The server and its Docker data are intact.

```bash
cd /opt/fodycloud
docker compose stop control-plane                              # no writes during the restore
/opt/fodycloud/restore.sh dump > /root/restore.sql             # latest off-site dump
#   or: zcat /var/backups/fodycloud/postgres-<stamp>.sql.gz > /root/restore.sql

docker compose exec -T postgres psql -U fodycloud -d postgres -c 'DROP DATABASE fodycloud WITH (FORCE)'
docker compose exec -T postgres psql -U fodycloud -d postgres -f - < /root/restore.sql
docker compose start control-plane
shred -u /root/restore.sql                                     # it contains secrets (env vars, hashes)
```

Errors such as `role "fodycloud" already exists` are expected: `pg_dumpall` recreates roles that
already exist.

## Restore a student's database or volume

A student asks for their data as it was on a given date. Restore it next to the live data, check
it with them, then copy it back.

```bash
/opt/fodycloud/restore.sh snapshots                    # find the snapshot of that date (tags apps-postgres, volumes)

# Database of app ID 12 (role and database app_12): the dump holds every app's database;
# copy the part between "\connect app_12" and the next "\connect" into a new database.
/opt/fodycloud/restore.sh apps-dump <snapshot> > /root/apps.sql

# Volume of the app "my-app":
/opt/fodycloud/restore.sh volume my-app <snapshot>     # into /var/backups/fodycloud/restore/volumes/fc-vol-my-app/_data
docker run --rm -v /var/backups/fodycloud/restore/volumes/fc-vol-my-app/_data:/from:ro \
  -v fc-vol-my-app:/to busybox:1.37.0 cp -a /from/. /to/
```

An app's ID: `docker compose exec postgres psql -U fodycloud -d fodycloud -c "SELECT id FROM apps WHERE name='my-app'"`.
Delete `/root/apps.sql` and the restored folder afterwards (`shred -u`, `rm -rf`): they hold
students' data.

## Rebuild on a new server

1. Create the VPS, then point `host` (and DNS) at it.
2. Run `tofu apply -replace=terraform_data.provision` with the **same** `restic_password`
   and `backup_s3_*` values. The platform comes up empty.
3. Restore the database as above.
4. Rebuild every app. Images aren't backed up, so queue a fresh build of each app; the worker
   builds them from their repositories, 2 at a time:
   ```bash
   cd /opt/fodycloud
   docker compose exec -T postgres psql -U fodycloud -d fodycloud -c \
     "UPDATE apps SET status='building', current_deployment_id=NULL WHERE status <> 'stopped';
      INSERT INTO deployments(app_id, kind) SELECT id, 'build' FROM apps WHERE status = 'building';"
   ```

## Test a restore without touching production

```bash
docker run -d --name restore-test -e POSTGRES_PASSWORD=x postgres:17-alpine
# wait until initialisation is over (the server restarts once)
until [ "$(docker logs restore-test 2>&1 | grep -c 'ready to accept connections')" -ge 2 ]; do sleep 1; done
/opt/fodycloud/restore.sh dump | docker exec -i restore-test psql -q -U postgres -d postgres
docker exec restore-test psql -U postgres -d fodycloud -c 'SELECT count(*) FROM users'
docker rm -f restore-test
```

The end-to-end tests (`qemu/e2e/20-backups.sh`) run this check on every test run.
