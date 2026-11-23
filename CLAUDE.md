# CLAUDE.md

FodyCloud is a student cloud-hosting platform. Students deploy apps from **public** GitHub, GitLab or Codeberg repos, and each app runs in its own Docker container with plan-based limits and automatic HTTPS. User guides: `docs/` (served at `<domain>/docs`, and `dashboard.<domain>/docs` for signed-in users; admin guide at `admin.<domain>/docs`). Operators: `README.md`.

## Open-source only

FodyCloud itself is **AGPL-3.0-or-later** (`LICENSE`, `NOTICE`, `TRADEMARKS.md`). New Go, shell, SQL and `.tf` files start with the two SPDX lines (`SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors`, `SPDX-License-Identifier: AGPL-3.0-or-later`); other files are covered by `REUSE.toml`. Vendored code keeps its own license (text in `LICENSES/`, entry in `REUSE.toml` and `NOTICE`). Go dependencies must also be AGPL-compatible (`GO_ALLOWED_LICENSES` in `scripts/check.sh`). Commits are signed off (DCO, `git commit -s`).

FodyCloud is built **only with open-source software** (OSI-approved licenses). Before adding any tool, library, container image or service, check its license, and check again on major upgrades, since several projects have relicensed. Prefer community forks when an upstream goes source-available:

| Use | Not | Why |
|---|---|---|
| **OpenTofu** (`tofu`, MPL-2.0) | Terraform ≥ 1.6 | Terraform is BUSL-1.1 since 1.6 |
| **Valkey** (BSD-3) | Redis 7.4–7.x | Redis 7.4 is RSALv2/SSPL |
| SeaweedFS, Garage (S3 for self-hosting and tests) | MinIO images | MinIO stopped publishing community images |

Current stack, all OSI: Go, PostgreSQL, Valkey, Traefik, Docker Engine/Moby, BuildKit, Railpack, docker-socket-proxy, restic, htmx, pgx, goldmark, the MCP Go SDK, OpenTofu, Debian, BusyBox (apps' health checks), QEMU, Mailpit and SeaweedFS. Planned for OAuth tests: mock-oauth2-server (MIT). App runtimes installed by Railpack in students' images: Python, Node.js, Bun, PHP with FrankenPHP and Composer, Go, OpenJDK with Maven/Gradle, Caddy (static sites).

External *services* chosen by the operator (an S3 provider, an SMTP provider, GitHub/GitLab/Codeberg, Let's Encrypt) aren't software we ship. Keep them swappable through standard protocols (S3, SMTP, OAuth/OIDC, ACME).

## Layout

- **Control plane (Go 1.27):** one module at the repo root, `github.com/fodydev/fodycloud`. A single binary, `cmd/fodycloud`, provides `serve | migrate | admin …`.
  - `internal/service`: **every business rule** (ownership, quotas, plans, repo validation). `api/` (REST), `web/` (dashboard) and `mcpserver/` (MCP) are thin layers over it.
  - `internal/worker`: claims queued `deployments` rows (`FOR UPDATE SKIP LOCKED`) and runs `builder`, then `deployer`. The `deployments` table *is* the job queue.
  - `internal/stacks`: the catalog of languages, frameworks and versions (Static, Python, Node.js with Bun, PHP, Go, Java); `Detect` maps Railpack's output to it, and `Railpack()` turns a stack into provider, packages, commands and env vars.
  - `internal/builder`: clones the repo, then:
    - `Detect` runs `railpack prepare` only, for the review step;
    - `Build` writes `.fodycloud-railpack.json` for the confirmed stack, runs `railpack prepare`, then `docker buildx build` on the `fodycloud-builder` builder (docker-container driver, network `fodycloud-build`).
  - `internal/deployer`: `docker run` with plan limits and Traefik labels, a blue/green swap, and `docker update` for plan changes.
  - `internal/monitor`: once a minute, samples `docker stats` (CPU, memory) and Traefik's internal Prometheus counters (requests). It feeds:
    - per-app metrics (`app_metrics`: per minute for 7 days, hourly rollups for 30 days; `service.RecordMetrics`/`RollupMetrics`);
    - CPU-abuse flags and idle sleep;
    - the unused-app rule (`service.CheckAbandoned`: plan `abandon_after_days`, warn, then stop after `abandon_grace_days`; never delete).
  - Platform alerts are a host script, `terraform/files/health.sh` (`fodycloud-health.timer`, every 15 min). It checks disk, memory and certificate expiry from Traefik's `acme-*.json`, and mails admins through `fodycloud admin notify`, at most once a day per alert, with a "Resolved" email after.
  - `internal/mail`: email goes into the `email_outbox` table and a background loop delivers it (STARTTLS required when credentials are sent). `internal/ratelimit` holds the in-memory token buckets and the login lockout.
  - `internal/dockercli`: wraps the **docker CLI** rather than the SDK, because the CLI is already needed for buildx.
  - `internal/db/migrations/*.sql`: embedded and applied on startup. To change the schema, add a new numbered file; never edit an applied one.
  - `internal/web/templates`, `internal/web/static`: embedded. htmx is vendored (`static/htmx.min.js`, 2.0.11), with no CDN.
- **`docs/`:** the user guide (`docs/user/`), the developer guide (`docs/developer/`) and the admin guide (`docs/admin/`), embedded by `docs/docs.go` (navigation: `docs.Nav`, `docs.AdminNav`) and rendered by `internal/web/docs.go` (goldmark, raw HTML off).
- **Sites (`internal/sites`, dispatch in `internal/web/router.go`):** one role per host.
  - `<domain>` is the home page and the public guides (`/docs`); a non-secret `fc_signed_in` hint cookie (`Domain=<domain>`) sends signed-in visitors to the dashboard, including from `/docs/…`.
  - `dashboard.` serves sign-in, the students' pages, and the guides at `/docs` for signed-in users (signed-out visitors are sent to the public copy).
  - `admin.` serves the panel and `/docs`, behind its own admin-only sign-in (`auth.SessionAdmin`, 8 h).
  - `api.` serves `/v1`. It is bearer-token only, with no cookies.
  - `mcp.` serves `/mcp`.
  - `*.apps.` serves the wake page and its `/static/` assets.
  - `{{.URL "docs" "/x"}}` points to the dashboard's copy on pages with a signed-in user, else to the home site's.
  - Links between sites use `{{.URL "docs" "/path"}}` in templates, never relative paths.
- **`terraform/`:** provisions an **existing VPS over SSH**; it does not create servers.
  - One concern per file (`terraform/README.md`): `files.tf` packs `local.files` (platform files plus the platform's whole **source** under `/opt/fodycloud/src`, from the `source_list` allow-list) into a self-extracting base64 bundle; `provision.tf` uploads it and runs `files/bootstrap.sh`; `settings.tf`, `secrets.tf`, `variables.tf` (validated), `outputs.tf`.
  - The server builds the control-plane image itself (`docker compose build control-plane`), so no registry is needed.
- **`qemu/`:** local test VM (`qemu/README.md`). `smoke-test.sh` covers the infrastructure; `e2e-test.sh` covers the product, sourcing `e2e/NN-*.sh` in order (one file per area, sharing variables) and `e2e/lib.sh`; `testlib.sh` holds `check`/`remote`/`section`/`finish`, the connection settings, VM lock and boot-ID guard.
- **Docs for people:** `README.md` (entry point), `CONTRIBUTING.md` (developers: checks and per-language conventions), `internal/README.md`, `terraform/README.md`, `qemu/README.md`, `docs/operations/` (operators' handbook, not served; the admin CLI lives here).
  - **Network:** `sudo qemu/net-setup.sh` (once) creates the NetworkManager bridge `fcbr0` on 203.0.113.0/24 (TEST-NET-3: never routed, and not private, so FodyCloud treats it as public).
    - The host is `.1` (the NAT gateway, i.e. "the internet"); `.2`–`.5` are extra client IPs for `curl --interface`, added by a dispatcher hook because NM's shared mode keeps one address.
    - The VM is `.10` (fixed MAC → static DHCP lease) on the user-owned TAP `fctap0`, with real ports 22/80/443.
    - `NET=user` falls back to slirp with `test-usernet.tfvars`.
    - `run-vm.sh` writes `.work/net.env`, which the tests read.
    - The setup also enables NetworkManager's dnsmasq resolver on the PC (`*.fodycloud.internal` → `.10`, no `/etc/hosts` needed) and trusts the local test CA.
  - **Test CA:** `qemu/ca-init.sh` makes a root in `qemu/.work/ca/`, name-constrained to `fodycloud.internal`.
    - With `test_ca_dir` set, step-ca (pinned image) runs in the VM with that root, alias `ca.fodycloud.internal`.
    - Traefik's ACME client uses it (`acme_ca_server`, `LEGO_CA_CERTIFICATES`), so certificates go through HTTP-01 for real.
    - The tests verify them with `--cacert` (no `-k`).

## Commands

```bash
scripts/check.sh      # must pass: gofmt, vet, tests, staticcheck, shellcheck, tofu fmt/validate, actionlint, reuse lint, go-licenses (pinned); CI runs it on every PR
scripts/check-signoff.sh   # every commit since origin/main is signed off (git commit -s); CI runs it on PRs

# Full test cycle (VM = 203.0.113.10 on the virtual network; `sudo qemu/net-setup.sh` once)
qemu/run-vm.sh
tofu -chdir=terraform apply -var-file=../qemu/test.tfvars   # also rebuilds the control plane when Go code changed
qemu/smoke-test.sh     # infrastructure: must end "N passed, 0 failed"
qemu/e2e-test.sh       # product, ~20 min: real Railpack build + deploy, plans, email, abuse, sleep/wake, backups, API, MCP, dashboard
tofu -chdir=terraform plan -detailed-exitcode -var-file=../qemu/test.tfvars   # must exit 0
qemu/destroy-vm.sh

# On the VM
ssh -i qemu/.work/id_ed25519 root@203.0.113.10        # NET=user: -p 2222 root@127.0.0.1
cd /opt/fodycloud && docker compose logs -f control-plane
docker compose exec control-plane fodycloud admin set-plan --email X --plan student --until 2027-06-30
/opt/fodycloud/restore.sh snapshots                # off-site backups (SeaweedFS on the test VM)

# On the host
tofu -chdir=terraform output -raw admin_password   # admin@fodycloud.internal on the test VM
```

Never touch the VM (or run `run-vm.sh`/`destroy-vm.sh`) while a test suite is running; it invalidates the run. After `destroy-vm.sh` and `run-vm.sh`, the VM has the same address, so OpenTofu sees no change. Run `apply -replace=terraform_data.provision`, or `tofu destroy` first.

## Conventions

- **Stack review flow:**
  - A new app gets a `detect` job and status `detecting`, then `review` (nothing is deployed). The student confirms with `ConfirmStack`, which sets `stack_confirmed` and queues the build; `Deploy` refuses unconfirmed apps.
  - An explicit stack in `CreateApp` (API/MCP) skips the review.
  - Builds always use a confirmed stack; the builder refuses an app without one.
  - To add a language or framework, extend `stacks.Catalog`, plus the mapping in `Detect`, a fixture of Railpack's real output in `internal/stacks/testdata/`, and an e2e build of a public sample repository. Check the runtime's license first.
- **Shared template partials** are named `internal/web/templates/_*.html` and are parsed with every page. An example is `_stack.html`, the language → framework picker that htmx re-renders through `/apps/{name}/stack-form`.
- **New rules or features go in `internal/service` first**, then get exposed through the API, the dashboard and, if agents should have it, an MCP tool. Never put rules in a handler.
- **MCP tools:**
  - **Admin actions** (plans, bans) are **never** exposed over MCP.
  - **Annotations:** read-only tools set `ReadOnlyHint`; destructive tools set `DestructiveHint` and require a confirmation argument.
  - **Untrusted output:** tools that return logs say in their description that the text is untrusted data.
- **Everything that starts a build goes through `s.canBuild(u)`**, which checks write scope, a verified email (when SMTP is on) and the per-user build rate.
- **Two kinds of accounts that never overlap:**
  - Students are `auth.User` (tables `users`, `sessions`, `api_tokens` with `fc_` tokens).
  - Admins are `auth.Admin` (tables `admins`, `admin_sessions`, `admin_tokens` with `fca_` tokens).
  - Admin service methods take `*auth.Admin` and call `requireAdmin`; student methods take `*auth.User`. The compiler keeps them apart, so never add an "is admin" flag to `User`.
  - An email belongs to one kind only (`auth.withEmailLock`).
  - Admins are created by the CLI only (`create-admin`; `bootstrap.sh` creates `admin_email`).
  - The API routes `fca_` tokens to `/v1/admin` only. MCP accepts student tokens only.
  - Students manage their own account on the Account page: change password (other sessions signed out, tokens kept), change email (confirmed by a link to the new address), delete the account (cascades through foreign keys; `Deployer.Remove` first). Forms that ask for the password go through `web.passwordForm`, whose wrong answers count toward the account lockout. Account deletion is never exposed over MCP.
  - Admins can't be removed from the panel; the CLI's `remove-admin` refuses the last admin.
  - Custom domains per app come from the plan (`max_custom_domains`); `AddDomain` counts and inserts under a lock on the app.
- **Storage and databases** (`service/storage.go`, `deployer/storage.go`, `internal/appsdb`): at most one of each per app, rows in `app_services`, sizes from the plan (`volume_size`, `db_size`/`db_connections`, `cache_memory`).
  - Volume `fc-vol-<app>`: local volume with an XFS quota (`--opt size=`), mounted at `/data`; shared by the old and new containers during the blue/green swap. A removed volume is deleted after the restart that unmounts it (`CleanupStorage`).
  - PostgreSQL: the shared `apps-postgres` (compose), on `fodycloud-apps` at the subnet's `.3` (firewall exception for port 5432 only) and on the internal `appsdb` network for the control plane. Role and database `app_<id>`, never named from user input; `pg_hba` `sameuser`. Size limit enforced by `CheckStorage` (every 10 min, `admin check-storage`): read-only over the limit (`default_transaction_read_only`, which the app can turn off to delete data), `CONNECTION LIMIT 0` when a database already read-only grows past 120% (a first pass over the limit, even a big import, is only read-only).
  - Valkey: container `fc-valkey-<app>` on the internal network `fc-net-<app>` (alias `valkey`), no persistence, `--user 65534` (its entrypoint's `su-exec` needs caps). `REDIS_URL` uses the `default` user: `redis://:pass@` fails in valkey-cli.
  - Storage containers carry `fodycloud.service-of`, **never** `fodycloud.app`: deploys, sleep and metrics select app containers by `fodycloud.app`, so the label would get the cache removed or stopped.
  - The connection variables are injected at build and run time (`JobConfig`); `service.ReservedEnv` can't be set by students. Volumes and the apps' databases are in the nightly restic backup (tags `volumes`, `apps-postgres`).
- **Email:** send with `s.mail(ctx, to, "<template>", data)`. Templates live in `internal/mail` (first line `Subject: …`). Never send synchronously from a request.
- **Test-only services** (Mailpit, SeaweedFS) live in `terraform/files/compose.test.yaml` and run only when `test_fixtures = true` (through `COMPOSE_FILE` in `.env`). Production code must not depend on them.
- **Mutating service methods call `requireWrite(u)`**, which enforces the `apps:read` / `apps:write` token scopes.
- **Another user's app returns `ErrNotFound`, not `ErrForbidden`**, so the app's existence doesn't leak.
- **Plans are only assigned by admins** (no online payment yet). They live in `subscriptions`, with an audit row in `plan_changes`. `ApplyPlan` enforces a plan on live containers.
- **Source code offer (AGPL-3.0 §13):** the image packs its build context (`/opt/fodycloud/src`) into `/usr/share/fodycloud/source.tar.gz`, served at `<domain>/source.tar.gz` behind the `/source` page (`web/source.go`), which every page's footer links to; `GET /v1/about` (public) and the MCP server's description point to it, with `source_url` (the repository). `source_list` in `files.tf` is an **allow-list**, since everything in it is published: a new top-level file or directory must be added there, and never state files, `terraform.tfvars`, `.terraform/` or `qemu/.work/` (the e2e suite checks the archive).
- **Adding a file to the server:** add an entry to `local.platform_files` in `files.tf`, plus a `changed /path` check in `bootstrap.sh` if a service must reload. Shell scripts go in `files/` (no `${` escaping); settings reach them through `templates/host.env.tftpl`.
- **Docker Compose runs only the platform's own services.** Student apps are plain containers on `fodycloud-apps` (students can't use Compose deployments).
- **Pin image tags and tool versions:**
  - Go: `Dockerfile`;
  - docker CLI: `docker:29.9.3-cli`;
  - Railpack: 0.43.0, with its sha256 in `Dockerfile` and the matching frontend tag in `config.go`;
  - platform images: `var.images`.

## Documentation

- **A user-visible change updates `docs/` in the same commit:** new or renamed UI labels, error messages, API fields, limits, MCP tools. A change to installing or running the platform updates `docs/operations/`. The root README is only an entry point that links to the guides.
- **Audiences:**
  - `docs/user/` is for students using the dashboard. Plain words, with no Railpack, Traefik or Docker.
  - `docs/developer/` is for API, CI and MCP users. It doesn't cover contributors; that's the README and this file.
  - `docs/admin/` is for the FodyCloud team (plans, suspensions, abuse, admin CLI/API). It is served only to signed-in admins on `admin.<domain>/docs`.
- **Structure (Diátaxis):**
  - one tutorial (`getting-started`);
  - how-to pages, one task each;
  - reference pages (stacks, limits, API, MCP tools, errors);
  - short explanations.
- **Each page** starts with what the reader achieves and what they need. Steps are numbered, with the expected result.
- **Style:**
  - second person, present tense, sentence-case headings;
  - UI labels in **bold**, exactly as on screen;
  - errors quoted with their exact message, cause and fix;
  - consistent terms (app, deployment, stack, working directory, publish folder, environment variable, plan, token).
- **Placeholders:**
  - examples use `fodycloud.example` (`docs.PlaceholderDomain`), which the dashboard replaces with the real domain;
  - the table after `<!-- fodycloud:plans -->` is replaced with the live plans.
- **Links:** use relative `.md` links. They work on the forge and are rewritten to site paths. Links from the admin guide to a public guide become `https://<domain>/docs/…` URLs, and links to other repository files point to the forge. Add new pages to `docs.Nav` or `docs.AdminNav`. `internal/web/docs_test.go` fails on a page missing from the nav, a broken link or a missing `#anchor`.
- **No server commands in the online docs:** the admin CLI (`fodycloud admin …`, `docker compose …`) is documented only in the README. `TestDocsNeverMentionTheAdminCLI` enforces it.
- **Test new instructions** against the test VM before committing them, as a reader would.

## Legal policies

- `docs/legal/` holds the terms of service, acceptable-use policy and privacy policy, served at `<domain>/legal/…`.
- Their `%OPERATOR_*%`, `%LEGAL_VERSION%` and `%EFFECTIVE_DATE%` placeholders come from the `operator_*` variables and `docs.LegalVersion`.
- Sign-up requires accepting them; `users.terms_accepted_at` and `terms_version` record it.
- When a policy changes materially, bump `docs.LegalVersion` and `LegalEffectiveDate`.
- When a feature changes what data is collected or how long it's kept (logs, metrics, backups), update `privacy.md` in the same commit.

## Gotchas (learned the hard way)

- **Session cookies must stay host-only:**
  - never set `Domain=` on `__Host-fc_session` or `__Host-fc_admin`;
  - student apps live under `*.apps.<domain>`, a subdomain of every site;
  - sessions have a `kind` (`dashboard`/`admin`), and each site only accepts its own.

- **`ssh_port` vs `server_ssh_port`:** OpenTofu connects to `ssh_port` (2222 with slirp `NET=user`). The firewall and fail2ban use `server_ssh_port` (22). Mixing them up locks you out.
- **Firewall:**
  - It lives in its own `table inet fodycloud`; never `flush ruleset`, which wipes Docker's rules.
  - Rules blocking apps or builds from private networks must match `ct state new` only. Traefik has an IP on `fc-apps`, and its replies to visitors on private or CGNAT addresses would otherwise be dropped.
  - Apps are isolated from each other with nft rules on `fc-apps`, not `enable_icc=false` (which would also block Traefik).
- **Firewall changes on the VM:** test them with an auto-rollback (`systemd-run --on-active=60 nft delete table inet fodycloud`). The serial console is a log file, not a login.
- **Disk quotas** (`--storage-opt size=`) need `/var/lib/docker` on XFS mounted with `pquota`, hence `data_disk_device`. XFS labels are at most 12 characters. `docker update` cannot change the disk size; it applies on the next deploy.
- **buildx builders are stored in the *client's* config.** The builder is created by the control plane and persisted in the `controlplane-docker` volume (`/root/.docker`). A builder created with the host's docker CLI is invisible to the container.
- **Railpack:**
  - Pass `--build-arg cache-key=app-<id>` so build caches don't leak between tenants. Apps must listen on `$PORT` (8080).
  - Build modes use `prepare --build-cmd/--start-cmd`. The start command lands in `deploy.startCommand` and runs via `bash -c`, so `$PORT` expands at runtime. `RAILPACK_START_CMD` passed through `--env` does **not** work: it becomes a secret.
  - **Start commands from Procfiles:** Railpack copies a Procfile's `web:` command, often written for another platform (`heroku-php-apache2`, a Go binary name). Only languages with `TrustStart` (Python, Node.js) keep Railpack's detected command in the suggestion; Go runs `./out`, Java runs the jar of `target/` or `build/libs/` (Railpack's Gradle command globs `*/build/libs`, which misses single-project builds), and PHP always gets `--start-cmd /start-container.sh` (FrankenPHP). `RAILPACK_PHP_ROOT_DIR` must be absolute (`/app/<publish folder>`); PHP has no version choice (pinning makes mise compile PHP from source; the FrankenPHP image follows `composer.json`), and Railpack doesn't read Gradle/Maven Java versions, so `Detect` does.
  - **FrankenPHP needs `NET_BIND_SERVICE`:** its binary has that file capability, and with `--cap-drop ALL` the kernel refuses to exec it ("Operation not permitted"). App containers get `--cap-add NET_BIND_SERVICE` back.
  - Pass `--error-missing-start` for every server build. Without it, Railpack builds an image whose command is a bare `/bin/bash`, which exits 0 and restart-loops while looking healthy. Static stacks (`staticfile`, and Node single-page apps with `RAILPACK_SPA_OUTPUT_DIR`) have no start command, so they skip it, and they must not get `--start-cmd`, which would disable static serving.
  - Detection data: `railpack-info.json` has `detectedProviders` and `metadata` (`pythonRuntime`, `nodeRuntime`), plus `resolvedPackages` for the version. The staticfile root dir appears only in a log line.
  - Every `prepare --env` variable becomes a build secret, including Railpack's own `RAILPACK_*` settings from the stack. `builder.BuildEnv` merges them with the app env, and the merged map feeds both `prepare` and `SecretArgs`, or the build fails with `secret RAILPACK_…: not found`.
  - `prepare --env K=V` stores only the *names* in the plan. Values go to buildx as `--secret id=K,env=FCSECRET_K` (prefixed so a student variable named `PATH`/`DOCKER_HOST` can't hijack the docker CLI), plus `--build-arg secrets-hash=…`.
- **Startup check:** a container counts as up only if `RestartCount == 0`. With a restart policy, a crash-looping app is "running" at any given instant.
- **Health checks** (`apps.health_path`): the deployer probes the new container before the swap with `busybox nc` (`var.images.probe`), run with `--network container:<app>` so it reaches `127.0.0.1:$PORT` without joining `fodycloud-apps` (whose nft rules isolate apps). The request is raw HTTP/1.0, so redirects aren't followed; the path is validated by `service.CleanHealthPath` because it goes into the request line.
- **Traffic gate:** app containers get a Docker health check that passes only once `/.fodycloud-ready` exists, and Traefik ignores containers that are `starting` or `unhealthy`. `Deploy` creates the file (`docker exec`, shell builtin only) after the startup or health checks pass, waits for `healthy`, then 3 s for Traefik's throttle, then removes the old container. Without it, Traefik routes visitors to a version still being checked, and to a failed one for seconds after its removal (502).
- **Live build logs (SSE):** `service.FollowBuildLog` subscribes to an in-memory hub (the worker and the web server share a process) *before* reading the stored log; offsets count characters (`deployments.log_size`), so they survive the 1 MiB trim. Traefik's `websecure` `writeTimeout` is 60 s, so each stream ends after 50 s (`api.StreamWindow`) and clients resume with `Last-Event-ID`. The dashboard keeps its 2 s polling as the fallback; `app.js` suppresses it while the stream works.
- **`docker logs` replays the container's stderr on stderr.** Use `dockercli.Combined` for logs, or app errors disappear.
- **Never put secrets in URLs:** a new API token is rendered directly in the POST response, not passed through a redirect query string, which would land in Traefik's access log.
- **htmx and the CSP:** the strict CSP (`style-src 'self'`) needs `includeIndicatorStyles:false` in the `htmx-config` meta tag. Client behaviour lives in `static/app.js`, never inline; it keeps each `pre.log[data-log]` scroll position across htmx swaps, so give every polled log a stable `data-log` key. Static URLs go through `{{asset "…"}}`, which adds a content hash so the one-day cache doesn't serve stale files.
- **Working directory (`root_dir`):** always resolve it with `builder.WorkingDir`, which follows symlinks and refuses paths outside the clone. The publish folder is relative to it.
- **`assigned_by` / `changed_by` / `banned_by` reference `admins(id)`.** The CLI admin has ID 0, which is stored as NULL, so pass nil, never `&0`.
- **`traefik.yml` is static config**, read only at Traefik startup, and compose doesn't notice bind-mount changes. `bootstrap.sh` force-recreates Traefik when `traefik.yml` or `dns.env` changes; keep that hook when adding static options.
- **Client IP:** use the **last** `X-Forwarded-For` hop (`ratelimit.ClientIP`). Traefik appends the real peer, and earlier hops can be forged.
- **Client IPs in tests:** most test traffic comes from one address (`203.0.113.1` on the bridge, `10.0.2.2` with slirp), so per-IP limits apply to the whole suite. `test.tfvars` raises `signup_limit_per_hour`; the login rate-limit checks use the extra client IPs (`curl --interface 203.0.113.3-5`) and run last.
- **Test domain is `fodycloud.internal`**: `.local` is answered by mDNS before DNS on typical Linux desktops. Careful with blind renames: `/etc/fail2ban/jail.d/fodycloud.local` is a fail2ban file name (fail2ban only reads `*.conf`/`*.local`), not the domain.
- **Test CA pitfalls:**
  - Every name step-ca serves must sit under `fodycloud.internal`, or the name constraint rejects it. That's why step-ca's own hostname is `ca.fodycloud.internal`, a Docker network alias, not `step-ca`.
  - step-ca issues 24-hour certificates, so Traefik gets `certificatesDuration: 24`.
  - With slirp (`NET=user`) the PC's wildcard DNS points at the bridge VM, so `test-usernet.tfvars` keeps self-signed certificates.
- **Never touch the test VM during a run:** the suite holds `.work/test.lock` (`flock`), `run-vm.sh`/`destroy-vm.sh` refuse, and `section` aborts when the VM's boot ID changes.
- **Wake routing:** the low-priority `HostRegexp` catch-all in `traefik/dynamic/wake.yml` (Terraform-rendered, for regex escaping) sends `*.apps.<domain>` to the control plane only when no app container serves the host. `main.go` dispatches those hosts to `web.ServeWake`.
- **"Must fail" checks need a positive control** in the tests, so they can't pass just because a tool is missing.
- **Use OpenTofu (`tofu`).** The configuration stays compatible with both; the `terraform` binary at `/usr/bin/terraform` (v1.17.2) works the same way, but documentation and new scripts use `tofu`.

## Security

- The control plane mounts the host Docker socket (root-equivalent). It's the only trusted component with access; student code runs only inside BuildKit (`fc-build`) or app containers (`fc-apps`).
- **Git** runs with no credentials, HTTPS only, no hooks and no submodules (`repo.SafeArgs`).
- **Passwords:** bcrypt. **Tokens and sessions:** stored as SHA-256 hashes, with the `fc_` prefix on tokens. **CSRF:** cookie-authenticated POSTs must be same-origin.
- **OpenTofu state** holds the generated secrets (Postgres, Valkey, admin and restic passwords); use an encrypted remote backend in production. The bundle is uploaded to the SSH user's home directory, not `/tmp`, and deleted after the run.
- Never commit `terraform.tfvars`, state files or `qemu/.work/`.
