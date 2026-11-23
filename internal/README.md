# The control plane's Go packages

Everything under `internal/` is the control plane: one binary (`cmd/fodycloud`) that serves the
websites, the API and MCP, runs the build worker and the monitor, and provides the admin CLI.

## How a request flows

```
browser ─┐                       ┌─ web (dashboard, admin panel, home, docs, wake pages)
script ──┼─ Traefik ─ sites ─────┼─ api (REST /v1)            ──┐
agent ───┘   (by host name)      └─ mcpserver (MCP /mcp)        ├─ service ─ PostgreSQL
                                                                │   (every business rule)
worker ─ claims queued deployments ─ builder ─ deployer ────────┘
monitor ─ docker stats + Traefik metrics ─ abuse flags, idle sleep
```

- **`service` holds every business rule**: ownership, plans and quotas, validation, the stack
  review. `api`, `web` and `mcpserver` only translate HTTP/MCP to service calls and back. A new
  rule goes in `service` first.
- **The `deployments` table is the job queue.** The service inserts a `queued` row; the worker
  claims it (`FOR UPDATE SKIP LOCKED`), runs the builder and the deployer, and records the result.

## Packages

| Package | Role | Start reading at |
|---|---|---|
| `service` | Business rules, one file per topic: `apps.go`, `deployments.go`, `stack.go`, `env.go`, `domains.go`, `plans.go`, `moderation.go`, `accounts.go`, `jobs.go`, `sleep.go`, `logs.go`. | `service.go` (the `Service` type and errors) |
| `auth` | Accounts and credentials: students (`User`, sessions, `fc_` tokens) and admins (`Admin`, admin sessions, `fca_` tokens), in separate tables. | `auth.go`, `admin.go` |
| `sites` | The platform's addresses and their roles, from the base domain. | `sites.go` |
| `web` | The websites. `router.go` picks a site by host; `dashboard.go`, `admin.go`, `docs.go`, `wake.go` serve them; `sessions.go` handles cookies; `templates/` and `static/` are embedded. | `router.go` |
| `api` | REST API under `/v1`: students' endpoints and admin endpoints, JSON errors. | `api.go` (`Handler`) |
| `mcpserver` | MCP tools for AI assistants, over the same service calls. | `mcpserver.go` |
| `worker` | Claims jobs from the queue and runs analyses and builds. | `worker.go` |
| `stacks` | The catalog of languages and frameworks, validation, Railpack options, detection mapping. | `stacks.go` (`Catalog`) |
| `builder` | Clone, analyse (Railpack), build (BuildKit). | `builder.go` (`Detect`, `Build`) |
| `deployer` | Runs app containers with plan limits and Traefik labels; blue/green swap. | `deployer.go` |
| `monitor` | CPU-abuse detection and idle sleep. | `monitor.go` |
| `mail` | Email templates, the outbox and SMTP delivery. | `mail.go` |
| `plans` | Loads plan limits from `plans.json` (written by OpenTofu). | `plans.go` |
| `ratelimit` | Token buckets, login lockout, client IP. | `ratelimit.go` |
| `repo` | Repository URL validation and safe `git` invocation. | `repo.go` |
| `dockercli` | Thin wrapper over the `docker` CLI (needed for buildx anyway). | `docker.go` |
| `config` | Environment variables (`FC_*`) → `Config`. | `config.go` |
| `db` | Connection and embedded migrations (`migrations/0001_init.sql`). | `db.go` |

The user, developer and admin guides are embedded from the top-level `docs/` package.

## Conventions

See [CONTRIBUTING.md](../CONTRIBUTING.md#go) for the Go conventions, and `CLAUDE.md` for the
project's rules (two kinds of accounts, MCP annotations, email, migrations…).
