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
once, in a transaction: a failed migration changes nothing and the control plane doesn't start.

From `v0.1.0` on, upgrades migrate your database **in place**:

- migrations only move forward and keep your data; a released migration is never edited;
- a new version's schema still works with the previous version's control plane, so you can go
  back to it (`git checkout <old tag>` and `tofu apply`) without restoring the database. The
  [upgrade notes](#upgrade-notes) say so when a version is an exception, or when going back needs
  a step of its own.

## Before upgrading a real platform

1. Run a backup and check it: `systemctl start fodycloud-backup.service`
   (see [Backups](backups-and-restore.md#check-that-backups-work)).
2. Read the [upgrade notes](#upgrade-notes) and the release notes since your version (or
   `git log <old>..<new>`).
3. Upgrade a test platform first, for example the QEMU VM described in
   [CONTRIBUTING.md](../../CONTRIBUTING.md).

## Upgrade notes

Versions follow [semantic versioning](https://semver.org). Before `v1.0.0`, a minor version
(`v0.3.0`) can add or change settings; a patch version (`v0.3.1`) only fixes bugs. Each version that needs something from you is listed here, newest first: new or
renamed `terraform.tfvars` settings, manual steps, and database changes.

### Next version (unreleased)

- **New, optional: sign-in with GitHub, GitLab or Codeberg.** Off unless you set the new
  `oauth_github`, `oauth_gitlab` or `oauth_codeberg` variables; with your current
  `terraform.tfvars`, nothing changes for students. See
  [Sign-in with GitHub, GitLab or Codeberg](forge-sign-in.md).
- **Database:** migration `0002_oauth` adds two tables (`user_identities`, `oauth_signups`) and
  changes no existing table. Going back to `v0.1.0` works on the migrated database; students who
  signed up with a forge then have no way to log in until you upgrade again.
- **Going back to `v0.1.0`:** first run `rm -rf /opt/fodycloud/src` on the server, then
  `tofu apply` from `v0.1.0`. `v0.1.0` unpacks its source over the newer one without removing
  the newer files, and its control plane then fails to build. From this version on, provisioning
  removes them itself.
- **Privacy policy:** a paragraph about forge accounts, and a new version (`2026-11-26`). If
  you edited `docs/legal/privacy.md`, merge the change, and add the paragraph before you turn a
  forge on.
- `tofu plan` shows only `terraform_data.provision` being replaced.

### v0.1.0

The first public release. Platforms installed from earlier, unreleased commits may need a fresh
database.

