# OpenTofu configuration

This configuration turns an **existing server reachable over SSH** into a FodyCloud platform. It
doesn't create servers: point it at one you already have. Operators: start with the
[installation guide](../docs/operations/install.md).

## How it works

1. OpenTofu renders every file the server needs (configuration, scripts, systemd units) and
   collects the control-plane **source**.
2. It packs them into one self-extracting bundle, uploads it over SSH to the SSH user's home
   directory, and runs `/opt/fodycloud/bootstrap.sh`.
3. `bootstrap.sh` (idempotent) installs packages, the firewall and Docker, builds the control
   plane image **on the server** (no registry needed), and starts the services with Docker
   Compose.

Any change to a shipped file, including the Go source, changes the bundle's hash and
re-provisions on the next `tofu apply`.

## Files

| File | Content |
|---|---|
| `variables.tf` | Every setting, grouped and described: connection, domain and TLS, plans, email, backups, limits, test fixtures. |
| `settings.tf` | Values derived from the variables (ACME mode, SMTP and backup endpoints, test CA). |
| `files.tf` | Everything shipped to the server (`local.platform_files`) and the control-plane source bundle. |
| `secrets.tf` | Generated passwords (database, Valkey, backups, first admin). |
| `provision.tf` | The upload and the run of `bootstrap.sh` (`terraform_data.provision`). |
| `outputs.tf` | Addresses, DNS records, TLS mode, secrets (sensitive). |
| `versions.tf` | Providers and the (commented) remote state backend. |
| `files/` | Files shipped as they are: `bootstrap.sh`, `compose.yaml`, backup scripts, systemd units. |
| `templates/` | Files rendered with variables (`*.tftpl`): Traefik, firewall, environment files. |

## Add a file to the server

1. Put it in `files/` (as is) or `templates/` (rendered).
2. Add an entry to `local.platform_files` in `files.tf`: destination path, mode, content.
3. If a service must reload when it changes, add a `changed /path` check in `bootstrap.sh`.

Shell scripts go in `files/` so they need no `${…}` escaping; settings reach them through
`templates/host.env.tftpl`.

## Commands

```bash
tofu -chdir=terraform init                       # once
tofu fmt -recursive && tofu -chdir=terraform validate
tofu -chdir=terraform apply                      # real server (terraform.tfvars)
tofu -chdir=terraform apply -var-file=../qemu/test.tfvars   # the local test VM
```

The configuration works with OpenTofu (open source, used here) and Terraform alike.
