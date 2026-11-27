# Operations handbook

This handbook is for the people who install and run a FodyCloud platform: the servers, DNS,
certificates, backups and the admin command line. It isn't served online.

The FodyCloud team's daily work in the admin panel (plans, suspensions, abuse flags) is covered
by the [admin guide](../admin/overview.md), which every platform serves at
`https://admin.<domain>/docs`.

| Page | Read it when you… |
|---|---|
| [Install a platform](install.md) | set up FodyCloud on a new server |
| [Addresses, DNS and certificates](addresses-and-tls.md) | configure DNS, or HTTPS doesn't work |
| [Sign-in with GitHub, GitLab or Codeberg](forge-sign-in.md) | let students log in with a forge account, or turn it off |
| [Admin CLI](admin-cli.md) | create admin accounts, script admin tasks, or the web interface is down |
| [Upgrades](upgrades.md) | deploy a new version of FodyCloud |
| [Backups and restore](backups-and-restore.md) | check backups, restore data, or move to a new server |
| [Security model](security.md) | need to know what protects the platform and its students |
| [Monitoring and incidents](incidents.md) | something is wrong: diagnosis steps and fixes |

## The platform at a glance

One server runs everything, provisioned from your computer with OpenTofu over SSH:

| Layer | What |
|---|---|
| OS | Debian 13 (also Debian 12 / Ubuntu 22.04+), chrony, swap, sysctl tuning, unattended security upgrades |
| Storage | Optional data disk, XFS with project quotas, on `/var/lib/docker` (per-app disk limits) |
| Security | Key-only SSH, fail2ban, nftables firewall (only SSH, 80 and 443 in) |
| Edge | Traefik v3: HTTPS with Let's Encrypt (or a private ACME CA), security headers |
| Platform | The control plane (one Go binary: dashboard, admin panel, API, MCP, build worker), PostgreSQL 17 and Valkey 8 on an internal network |
| Apps | One container per app on `fodycloud-apps`; builds on an isolated BuildKit builder; optional HTTP health checks before each deployment goes live (BusyBox image, `images.probe`) |
| Maintenance | Nightly database dump and encrypted off-site copy; weekly build-cache and image cleanup |

Everything lives under `/opt/fodycloud` on the server:

| Path | Content |
|---|---|
| `compose.yaml`, `.env` | the platform services and their settings |
| `config/` | `plans.json`, `host.env`, the initial admin's credentials |
| `traefik/` | Traefik's static and dynamic configuration |
| `src/` | the control-plane source, built on the server |
| `bootstrap.sh`, `backup.sh`, `restore.sh` | provisioning, nightly backup, restore helper |
