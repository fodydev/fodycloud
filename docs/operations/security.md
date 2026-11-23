# Security model

This page summarises what protects the platform, its students and their apps, and what you,
the operator, must protect yourself.

## Trust boundaries

| Component | Trust | Why |
|---|---|---|
| Control plane | trusted | It mounts the host's Docker socket (root-equivalent) to build and run apps. Only FodyCloud's own code runs there. |
| Builds | untrusted | Student code runs only inside the BuildKit builder, on the `fodycloud-build` network. |
| Apps | untrusted | Student code runs as unprivileged containers (`cap-drop ALL` except `NET_BIND_SERVICE`, which PHP's FrankenPHP binary requires; `no-new-privileges`) on `fodycloud-apps`. An app's Valkey cache runs on a network private to that app, with no internet access. |
| Students' databases | trusted service, untrusted clients | `apps-postgres` sits on `fodycloud-apps` at the `.3` address; the firewall lets apps reach only its port 5432. `pg_hba` lets each app's role open only its own database (`sameuser`), and the superuser `fcadmin` only from the control plane's internal `appsdb` network. |
| Traefik | trusted | Reaches Docker only through a read-only socket proxy. |

## Protections

| Protection | How |
|---|---|
| Network isolation | Apps and builds can't reach each other, the host, the database, or private and cloud-metadata networks. Traefik is their only way in. |
| Egress | Apps and builds can't send email on port 25 or reach common mining-pool ports. |
| Resource limits | CPU, memory, processes and disk per app, from the plan. Builds: 2 CPUs, 4 GB, 15 minutes. |
| Accounts | Passwords hashed with bcrypt. Sessions and API tokens stored as SHA-256 hashes. Admin accounts are separate from student accounts, with 8-hour sessions. |
| Cookies | Host-only (`__Host-` prefix), so student apps under `*.apps.<domain>` can't read or plant them. Same-origin check on every form. |
| Rate limits | Sign-in: 10/min per IP, and a 15-minute lockout after 5 failures on an account. Sign-up and password reset: 5/hour per IP. API and MCP: 120/min per account. Builds: 20/hour per student. |
| Email verification | Students confirm their address before deploying (when SMTP is configured). |
| Abuse | Apps at their CPU limit for 30 minutes are flagged; on the free plan, two flags in 24 h stop the app. Admins can suspend accounts and ban repositories, under the acceptable-use policy that students accept at sign-up. |
| Server | Key-only SSH, fail2ban, nftables firewall (only SSH, 80 and 443 in), unattended security upgrades. |
| Repositories | Cloned over HTTPS without credentials, no hooks, no submodules. |

## What you must protect

- **The OpenTofu state.** It holds every generated secret: use an encrypted remote backend.
- **`restic_password`.** Without it the off-site backups can't be decrypted; with it, anyone
  holding the bucket credentials can read them. Store it in a password manager, not on the
  server.
- **SSH access to the server.** It is root access to everything, student data included.
  Restrict it with `admin_ssh_cidrs` and pin `ssh_host_key`.
- **Admin accounts.** Create one per person, and reset or remove them when people leave
  ([Admin CLI](admin-cli.md)).
