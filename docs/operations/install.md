# Install a platform

This page sets up FodyCloud on a new server. It takes about 30 minutes, most of it waiting for
the first build.

**You need:**

- a VPS with Debian 13, reachable over SSH as root (or a user with passwordless sudo) with your
  key, ideally with a second disk for Docker data;
- a domain name, with access to its DNS records;
- on your computer: [OpenTofu](https://opentofu.org) 1.6 or later and a clone of this repository.

## 1. Configure

```bash
cd terraform
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars`. The essential settings:

| Setting | Example | Meaning |
|---|---|---|
| `host` | `"198.51.100.20"` | the server's address |
| `base_domain` | `"fodycloud.example"` | the platform's domain (see [Addresses](addresses-and-tls.md)) |
| `acme_email` | `"ops@fodycloud.example"` | contact for Let's Encrypt; empty = self-signed certificates |
| `acme_staging` | `true` | use Let's Encrypt's staging CA for the first run |
| `data_disk_device` | `"/dev/vdb"` | the data disk, formatted XFS (erased!); empty = no disk quotas |
| `admin_email` | `"ops@fodycloud.example"` | the first admin account |

For a real platform, also set:

- `smtp_*` and `mail_from`: email verification, password reset and admin alerts. Without SMTP,
  students can deploy without confirming their email.
- `backup_s3_*`: the off-site, encrypted backup (Backblaze B2, Cloudflare R2, Wasabi, any
  S3-compatible storage). See [Backups](backups-and-restore.md).
- `ssh_host_key`: pins the server's SSH host key; `admin_ssh_cidrs`: restricts SSH to your
  networks.
- **The operator, named in the legal policies:** `operator_name` (your legal name),
  `operator_country` (governing law), `operator_contact_email` (privacy requests and abuse
  reports) and `operator_data_location` (where the server is).
- **Source code:** `source_url`, the public repository linked from `https://<domain>/source`
  (default: `https://github.com/fodydev/fodycloud`). FodyCloud is licensed under the AGPL-3.0,
  which requires offering your users the source of the version you run. The platform does this
  itself: every page links to `/source`, which serves an archive of the exact source the
  control plane was built from, your changes included. If you run a modified version, point
  `source_url` to your fork, and see [TRADEMARKS.md](../../TRADEMARKS.md)
  for the name.

Optional tuning: `alert_disk_percent`, `alert_memory_percent` and `alert_cert_days` (platform
alerts, see [Monitoring](incidents.md#platform-alerts)), the plans' `abandon_after_days` and
`abandon_grace_days` (unused apps).

Every variable is described in `terraform/variables.tf`.

> **Review the legal policies before opening the platform.** FodyCloud ships a terms of
> service, an acceptable-use policy and a privacy policy (`docs/legal/`, served at
> `https://<domain>/legal/…`) filled in with your operator settings. They are a starting point,
> not legal advice: have them reviewed for your country, edit the files, and change
> `LegalVersion` in `docs/docs.go` when the texts change materially (students accept a version
> at sign-up).

## 2. Provision

```bash
tofu init
tofu apply
```

`tofu apply` uploads the configuration and the control-plane source to the server, then runs
`/opt/fodycloud/bootstrap.sh`, which installs Docker, the firewall and the services, builds the
control plane on the server, and starts everything. The first run takes 10 to 20 minutes.

## 3. Point DNS at the server

```bash
tofu output dns_records
```

Create these records at your DNS provider. See [Addresses, DNS and certificates](addresses-and-tls.md).

## 4. Switch to real certificates

Once DNS points to the server and `https://<domain>` shows a staging certificate, set
`acme_staging = false` and run `tofu apply` again.

## 5. Keep the secrets safe

```bash
tofu output -raw admin_password    # first sign-in to https://admin.<domain>
tofu output -raw restic_password   # decrypts the off-site backups: store it OUTSIDE the server
```

The OpenTofu state holds every generated secret (database, Valkey, admin and backup
passwords). For a real platform, use an encrypted remote backend (see `terraform/versions.tf`).

## 6. Check the platform

1. Open `https://<domain>`: the home page loads over HTTPS.
2. Sign in to `https://admin.<domain>` with `admin_email` and the admin password.
3. Sign up on `https://dashboard.<domain>` with another email and deploy a sample app, as in the
   [getting started tutorial](../user/getting-started.md).

## Next steps

- Create the team's admin accounts: [Admin CLI](admin-cli.md).
- Check the first backup: [Backups and restore](backups-and-restore.md#check-that-backups-work).
