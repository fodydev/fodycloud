# Monitoring and incidents

This page shows where to look when something goes wrong, then gives a fix for each common
incident. All commands run on the server, in `/opt/fodycloud`, as root.

## Where to look

| What | Command |
|---|---|
| Services and their health | `docker compose ps` |
| Control plane (web, API, builds, monitor) | `docker compose logs -f --since 30m control-plane` |
| Traefik (routing, certificates) | `docker compose logs --since 30m traefik` |
| Control-plane health (prints `ok`) | `docker compose exec control-plane wget -qO- http://127.0.0.1:8080/healthz` |
| Disk | `df -h / /var/lib/docker` |
| Firewall | `nft list table inet fodycloud` |
| Timers (backup, cleanup) | `systemctl list-timers 'fodycloud-*'` |
| An app's containers | `docker ps -a --filter label=fodycloud.app=<app>` |

Admins receive emails for failed backups, failed backup checks, CPU-abuse flags and the
platform alerts below. Use `fodycloud admin notify` to send your own alerts
([Admin CLI](admin-cli.md#repositories-and-alerts)).

## Platform alerts

Every 15 minutes, `fodycloud-health.timer` runs `/opt/fodycloud/health.sh`, which emails every
admin when:

| Alert | Threshold (setting, default) |
|---|---|
| A disk (`/` or `/var/lib/docker`) is almost full | `alert_disk_percent`, 85 % |
| Memory is almost all in use | `alert_memory_percent`, 90 % |
| A TLS certificate expires soon (apps, custom domains and the platform's own) | `alert_cert_days`, 14 days |

An alert is sent again at most once a day while the problem lasts, and a "Resolved" email
follows when it is gone. Run the check by hand with `systemctl start fodycloud-health.service`
and read its result with `journalctl -u fodycloud-health -n 20`.

Per-app CPU, memory and requests are on each app's page (for its owner) and through the API.

## The platform is down

1. `docker compose ps`: every service should be `running (healthy)`.
2. If one isn't: `docker compose logs --since 1h <service>`, then `docker compose up -d`.
3. If the control plane keeps restarting, its log usually names the cause: the database is
   unreachable, a migration failed, or a configuration value is invalid.
4. If nothing answers on ports 80/443: `nft list table inet fodycloud` and
   `systemctl status fodycloud-firewall`. Never run `nft flush ruleset`: it also removes Docker's
   rules.

## Builds stay queued or fail for everyone

1. `docker compose logs --since 30m control-plane | grep -i build` shows the worker's errors.
2. Check the builder:
   `docker compose exec control-plane docker buildx inspect fodycloud-builder --bootstrap`.
3. If the builder is broken, remove it; the control plane recreates it on the next build:
   `docker compose exec control-plane docker buildx rm fodycloud-builder`.
4. Disk full is a common cause: see below.

## The disk is full

1. `df -h /var/lib/docker` and `docker system df`.
2. Run the weekly cleanup now: `systemctl start fodycloud-prune.service`. It trims the build
   cache to 20 GB and removes app images older than two weeks that no container uses.
3. Old database dumps live in `/var/backups/fodycloud` (kept `backup_retention_days`).
4. If it is still full, grow the data disk, or lower the plans' `disk` limits.

## Certificates fail

See [Addresses, DNS and certificates](addresses-and-tls.md#troubleshooting).

## Emails don't arrive

Email is queued in the database and sent in the background, with retries.

```bash
docker compose exec -T postgres psql -U fodycloud -d fodycloud -c \
  "SELECT to_addr, subject, attempts, last_error FROM email_outbox WHERE sent_at IS NULL ORDER BY id DESC LIMIT 10;"
```

`last_error` shows what the SMTP server answered. Check `smtp_*` in `terraform.tfvars`: the
platform requires STARTTLS when it sends credentials.

## A student's app misbehaves

| Situation | Action |
|---|---|
| Uses all its CPU | It appears in the admin panel's **Abuse flags**; see the [admin guide](../admin/abuse-and-bans.md). |
| Mining, attacks, phishing | Suspend the account and ban the repository from the admin panel. |
| Needs to stop right now | `docker stop <container>` (from `docker ps --filter label=fodycloud.app=<app>`), then suspend the account. |

## Restore data

See [Backups and restore](backups-and-restore.md).
