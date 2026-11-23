# Upgrades

This page deploys a new version of FodyCloud, or a configuration change, to a running platform.

## Deploy a new version

FodyCloud is released as version tags (`v0.3.0`), listed on the repository's
[releases page](https://github.com/fodydev/fodycloud/releases) with their changes. Read the
[upgrade notes](#upgrade-notes) of every version between yours and the new one first.

On your computer, in your clone of the repository:

```bash
git fetch --tags
git checkout v0.3.0           # the version to deploy
tofu -chdir=terraform plan    # shows terraform_data.provision being replaced
tofu -chdir=terraform apply
```

Running `main` instead of a release gets you fixes sooner, with less testing.

`tofu apply` ships the new source and configuration and runs `bootstrap.sh` again. The script is
idempotent: it rebuilds the control-plane image when the source changed, recreates only the
services whose configuration changed, and applies database migrations when the control plane
starts.

What students notice:

- the dashboard, API and MCP are unavailable for a few seconds while the control plane restarts;
- **running apps keep serving**: app containers aren't restarted;
- builds in progress fail and are marked as interrupted; students redeploy them.

## Change the configuration

Edit `terraform.tfvars` (plans, SMTP, backups, limits…) and run `tofu apply`. The same rules
apply: only what changed is reloaded.

New limits in `plans` reach a running app at its next deploy or restart, or when an admin
changes that student's plan; apps aren't restarted for you.

## Database migrations

The control plane applies the SQL files of `internal/db/migrations/` in order at startup, each
once.

> **FodyCloud is still pre-production.** The schema is a single file that may change without a
> migration path: an upgrade can require a fresh database. Until the first production release,
> back up before upgrading, and be ready to recreate the platform.

## Before upgrading a real platform

1. Run a backup and check it: `systemctl start fodycloud-backup.service`
   (see [Backups](backups-and-restore.md#check-that-backups-work)).
2. Read the [upgrade notes](#upgrade-notes) and the release notes since your version (or
   `git log <old>..<new>`).
3. Upgrade a test platform first, for example the QEMU VM described in
   [CONTRIBUTING.md](../../CONTRIBUTING.md).

## Upgrade notes

Versions follow [semantic versioning](https://semver.org). Before `v1.0.0`, a minor version
(`v0.3.0`) can change the configuration or need a fresh database; a patch version (`v0.3.1`)
only fixes bugs. Each version that needs something from you is listed here, newest first: new or
renamed `terraform.tfvars` settings, manual steps, and database changes.

No versions have been released yet.

