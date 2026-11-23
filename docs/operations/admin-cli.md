# Admin CLI

The admin command line runs on the server, inside the control-plane container. Use it to create
admin accounts (the admin panel can't), to script admin tasks on the server, and when the web
interface is down. The admin panel's day-to-day features are in the
[admin guide](../admin/overview.md).

## Run a command

```bash
ssh root@<server>
cd /opt/fodycloud
docker compose exec control-plane fodycloud            # prints every command
docker compose exec control-plane fodycloud admin <command> [options]
```

Pass passwords in the `FC_NEW_PASSWORD` environment variable (`-e FC_NEW_PASSWORD=…`) rather than
`--password`, so they don't appear in the server's process list.

Commands run as a built-in admin. Changes they make are recorded in the audit log without an
admin name (and plan notes start with `[cli]`), unless `FC_ADMIN_EMAIL` names an admin account.

## Admin accounts

Admin accounts are separate from student accounts: they sign in only to `admin.<domain>`, and an
email belongs to one kind of account only.

| Task | Command |
|---|---|
| Create an admin | `docker compose exec -e FC_NEW_PASSWORD='a-long-password' control-plane fodycloud admin create-admin --email teacher@univ.cm` |
| Reset an admin's password (signs them out, revokes their admin tokens) | `docker compose exec -e FC_NEW_PASSWORD='a-new-password' control-plane fodycloud admin reset-admin-password --email teacher@univ.cm` |
| Create an admin API token (`fca_…`, printed once) | `docker compose exec control-plane fodycloud admin create-admin-token --email teacher@univ.cm --name payments-sync` |
| List admins | `docker compose exec control-plane fodycloud admin list-admins` |
| Remove an admin (signs them out, deletes their admin tokens; refuses the last admin) | `docker compose exec control-plane fodycloud admin remove-admin --email teacher@univ.cm` |

## Students

| Task | Command |
|---|---|
| Create a student account (email already confirmed) | `docker compose exec -e FC_NEW_PASSWORD='a-long-password' control-plane fodycloud admin create-user --email student@univ.cm` |
| Create a student API token (`fc_…`, printed once) | `docker compose exec control-plane fodycloud admin create-token --email student@univ.cm --name ci --scopes apps:read,apps:write` |
| Suspend / reinstate | `docker compose exec control-plane fodycloud admin ban --email student@univ.cm --reason "Mining"` / `… admin unban --email student@univ.cm` |

## Plans

| Task | Command |
|---|---|
| Assign a plan (until the end of that day) | `docker compose exec control-plane fodycloud admin set-plan --email student@univ.cm --plan student --until 2027-06-30 --note "paid by MoMo, ref 123"` |
| Back to free | `docker compose exec control-plane fodycloud admin revoke-plan --email student@univ.cm` |
| Apply expired plans and send reminders now | `docker compose exec control-plane fodycloud admin expire` |

Plan changes apply immediately to running apps. Expiry also runs automatically every 10
minutes. The plans themselves (`free`, `student`, `research` and their limits) are set by the
`plans` variable in `terraform.tfvars`; change it and run `tofu apply`.

## Repositories and alerts

| Task | Command |
|---|---|
| Measure volumes and databases, and apply the database size limits now (it also runs every 10 minutes) | `docker compose exec control-plane fodycloud admin check-storage` |
| Ban / allow a repository | `docker compose exec control-plane fodycloud admin ban-repo --repo https://github.com/x/miner --reason "Mining"` / `… admin unban-repo --repo https://github.com/x/miner` |
| Email every admin | `docker compose exec control-plane fodycloud admin notify --subject "Disk almost full" --body "…"` |

`notify` is what the backup script uses to report failures; use it from your own scripts too.
