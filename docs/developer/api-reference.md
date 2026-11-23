# API reference

The REST API manages your apps: create them, review their stack, deploy, read logs, and change
settings. Every request needs an [API token](authentication.md), except
[`GET /about`](#get-about).

## Conventions

**Base URL:** `https://api.fodycloud.example/v1`

**Requests**

- Send JSON with `Content-Type: application/json`. Unknown fields are rejected, so a typo in a
  field name gives a clear error instead of being ignored.
- App names appear in paths: `/apps/{name}`.

**Responses**

- Bodies are JSON. Times are in RFC 3339 format, in UTC.
- Success statuses:
  - `200` for `GET`, `PUT` and `PATCH`;
  - `202` for `POST`: the work is queued, so follow the returned deployment;
  - `204` for `DELETE`, with no body.
- Errors have a single `error` field with a message you can show to people:

  ```json
  { "error": "branch not found in repository" }
  ```

| Status | Meaning |
|---|---|
| `401` | Missing, wrong or revoked token. |
| `403` | Not allowed: read-only token, unconfirmed email, plan limit reached, too many builds. |
| `404` | The app or deployment doesn't exist, or belongs to another user. |
| `422` | The request is invalid: the message says what to fix. |
| `429` | Too many requests: wait for the number of seconds in the `Retry-After` header. |
| `500` | An error on FodyCloud's side. Retry later; report it if it persists. |

**Rate limits:** 120 requests per minute per account, shared with the MCP server, and 20 builds
per hour per account.

## Objects

### App

```json
{
  "name": "my-portfolio",
  "repo_url": "https://github.com/you/my-portfolio.git",
  "branch": "main",
  "status": "running",
  "url": "https://my-portfolio.apps.fodycloud.example",
  "custom_domains": ["www.my-portfolio.com"],
  "stack": {
    "language": "node",
    "framework": "vite",
    "version": "22",
    "build_command": "npm run build",
    "publish_dir": "dist",
    "root_dir": "frontend"
  },
  "stack_confirmed": true,
  "health_check_path": "/healthz",
  "detection": { "detected": true, "language": "node", "framework": "vite", "message": "…", "suggested": { "…": "…" } },
  "current_deployment": { "id": 42, "kind": "build", "status": "running", "…": "…" },
  "last_request_at": "2026-11-19T09:41:12Z",
  "stops_on": "2026-11-26T10:00:00Z",
  "created_at": "2026-11-18T09:12:44Z"
}
```

| `status` | Meaning |
|---|---|
| `detecting` | The repository is being analysed. |
| `review` | Waiting for you to confirm the stack. Nothing is deployed. |
| `building` | A deployment is in progress. |
| `running` | The app is online. |
| `sleeping` | Idle on a plan with idle sleep; it wakes on the next visit. |
| `stopped` | Stopped by you, by a plan change, or by the abuse protection. |
| `failed` | The last deployment failed and no version is running. |

`health_check_path` appears when the app has a [health check](../user/health-checks.md).
`last_request_at` is the last visit the platform saw. `stops_on` appears only when the app is
unused and will be stopped on that date unless it is visited or deployed (see
[Unused apps](../user/plans-and-limits.md#unused-apps)).

### Stack

| Field | Required | Description |
|---|---|---|
| `language` | yes | `static`, `python`, `node`, `php`, `go` or `java`. |
| `framework` | yes | A framework ID from [`GET /stacks`](#get-stacks), for example `flask`, `fastapi`, `django`, `fasthtml`, `node-server`, `nextjs`, `nuxt`, `vite`, `astro`, `bun`, `php`, `laravel`, `go-server`, `spring-boot`, `jar`, `static`, or `custom`. |
| `version` | no | A version from `GET /stacks`, for example `3.12` or `22`. Empty means automatic. |
| `build_command` | no | Runs after dependencies are installed. Empty means the framework's default. One line, at most 2000 characters. |
| `start_command` | for `custom` | Starts the server on `$PORT`. Empty means the framework's default. Not allowed for static frameworks and for `php`, which is served by FrankenPHP. |
| `publish_dir` | no | Static frameworks: the folder served as the website. The `php` framework: the folder that contains `index.php`. Relative to `root_dir`. |
| `root_dir` | no | The working directory inside the repository. Empty means the root. |

The response always contains the stack after the defaults are filled in.

### Deployment

```json
{
  "id": 42,
  "kind": "build",
  "status": "running",
  "commit_sha": "6f1c2e9…",
  "error": "",
  "created_at": "2026-11-18T09:13:02Z",
  "started_at": "2026-11-18T09:13:03Z",
  "finished_at": "2026-11-18T09:14:21Z",
  "duration_seconds": 78
}
```

`duration_seconds` appears once the deployment is finished: the time from its start to its
end, build and health check included.

| `kind` | Started by |
|---|---|
| `detect` | Creating an app without a stack, or `POST /analyse`. |
| `build` | Confirming the stack, `POST /deploy`. |
| `restart` | `POST /restart`, or changing environment variables. |
| `rollback` | `POST /rollback`. |

| `status` | Meaning |
|---|---|
| `queued` | Waiting for a free builder. |
| `building`, `deploying` | In progress. |
| `running` | This deployment is the live version. |
| `superseded` | Was live, replaced by a newer deployment. |
| `done` | A `detect` job that finished. |
| `failed` | Failed: see `error` and the log. The previous version keeps running. |
| `cancelled` | Replaced by a newer request before it started. |

Only the most recent queued request per app runs: a new deploy cancels a queued one.

## Platform

### `GET /about`

The platform's version and where to get its source code. Public: no token needed.

```json
{
  "name": "FodyCloud",
  "version": "3f9c2a71b0de",
  "license": "AGPL-3.0-or-later",
  "source": "https://fodycloud.example/source",
  "repository": "https://github.com/fodydev/fodycloud"
}
```

`source` is a page with the exact source code of the running version; `repository` is where
FodyCloud is developed.

## Account

### `GET /me`

Your account, plan, limits and usage. Scope: `apps:read`.

```json
{
  "email": "amina@univ.example",
  "scopes": ["apps:read", "apps:write"],
  "plan": {
    "plan": "student",
    "assigned_plan": "student",
    "valid_until": "2027-06-30T00:00:00Z",
    "expired": false,
    "limits": { "cpus": 1, "memory": "1g", "disk": "5G", "pids": 512, "max_apps": 5, "max_custom_domains": 5, "sleep_after_minutes": 0 },
    "apps_used": 2
  }
}
```

### `DELETE /me?confirm={email}`

Deletes your account and all your apps, at once and for good. `confirm` must repeat your
account's email. Scope: `apps:write`. Response `204`.

Error: `422` `confirmation does not match your email address`.

### `GET /stacks`

The languages, frameworks (with their default commands) and versions you can choose. Scope:
`apps:read`.

```json
{
  "languages": [
    {
      "id": "python",
      "label": "Python",
      "versions": ["3.13", "3.12", "3.11", "3.10"],
      "frameworks": [
        { "id": "flask", "label": "Flask", "build_command": "pip install gunicorn", "start_command": "gunicorn --bind 0.0.0.0:$PORT app:app", "hint": "…" },
        { "id": "custom", "label": "Other (custom commands)", "custom": true, "hint": "…" }
      ]
    }
  ]
}
```

Static frameworks have `"static": true` and a default `publish_dir`. Frameworks served by the
language's web server (`php`) have `"doc_root": true`: their `publish_dir` is the document root.

## Apps

### `GET /apps`

Lists your apps: `{"apps": [App, …]}`. Scope: `apps:read`.

### `POST /apps`

Creates an app. Scope: `apps:write`.

| Field | Required | Description |
|---|---|---|
| `name` | yes | 3 to 32 characters: lowercase letters, digits and single hyphens, starting with a letter. Becomes `https://{name}.apps.fodycloud.example`. |
| `repo_url` | yes | A public repository on github.com, gitlab.com or codeberg.org. |
| `branch` | no | Defaults to `main`. |
| `env` | no | Environment variables, `{"KEY": "value"}`. See [rules](../user/environment-variables.md#rules). |
| `stack` | no | See below. |
| `health_check_path` | no | A path such as `/healthz`, checked before each deployment goes live. See [`PUT /apps/{name}/health-check`](#put-appsnamehealth-check). |

How the `stack` field changes what happens:

- **No `stack`:** FodyCloud analyses the repository (a `detect` deployment) and the app waits in
  `review`. Confirm it with [`PUT /apps/{name}/stack`](#put-appsnamestack).
- **`stack` with only `root_dir`:** the same, but the analysis starts in that folder.
- **A full `stack`** (`language` and `framework`): the review is skipped and the app is built
  immediately. Use this only when the stack was chosen by a person.

```bash
curl -X POST -H "Authorization: Bearer $FC_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"my-api","repo_url":"https://github.com/you/my-api","stack":{"language":"python","framework":"fastapi","start_command":"uvicorn app.main:app --host 0.0.0.0 --port $PORT"}}' \
  https://api.fodycloud.example/v1/apps
```

Response `202`: `{"app": App, "deployment": Deployment}`.

Errors (`422` unless stated):
- `repository not found or not public (private repositories are not supported)`
- `branch not found in repository`
- `the name "…" is already taken`
- `app name "…" is reserved`
- `403` `the free plan allows 2 apps; delete an app or ask an admin for a bigger plan`

### `GET /apps/{name}`

One app, with its `detection` once analysed. Scope: `apps:read`.

### `DELETE /apps/{name}?confirm={name}`

Deletes the app, its deployments and its settings. The `confirm` parameter must repeat the
name. Scope: `apps:write`. Response `204`.

### `PUT /apps/{name}/stack`

Confirms or changes the stack (a [Stack](#stack) object), then builds the app. Scope:
`apps:write`.

Response `200`: `{"stack": Stack, "deployment": Deployment}`.

Errors (`422`):
- `unknown framework "…" for Python (available: …)`
- `custom commands need a run command that starts a server listening on $PORT`
- `publish folder must be a relative path inside the repository, e.g. dist or public`
- `working directory must be a folder inside the repository, e.g. frontend or apps/web`
- `the repository is still being analysed; wait for the result before confirming`

### `POST /apps/{name}/analyse`

Runs the detection again, for example after a push. It doesn't change a confirmed stack. Scope:
`apps:write`.

Optional body: `{"root_dir": "frontend"}` analyses that folder. It is only accepted before the
stack is confirmed; afterwards, change `root_dir` with `PUT /stack`.

Response `202`: Deployment (`kind: detect`).

## Deployments

### `POST /apps/{name}/deploy`

Builds the latest commit of the branch with the confirmed stack. Scope: `apps:write`. Response
`202`: Deployment.

Errors: `422` `review and confirm the detected stack first`; `403` `too many builds, try again in …`.

### `POST /apps/{name}/restart`

Restarts the current version without rebuilding it. Scope: `apps:write`. Response `202`:
Deployment.

### `POST /apps/{name}/rollback`

Redeploys the previous version that worked. Scope: `apps:write`. Response `202`: Deployment.

Error: `422` `there is no previous successful deployment to roll back to`.

### `POST /apps/{name}/stop` and `POST /apps/{name}/start`

Stops or starts the app. Scope: `apps:write`. Response `202`: `{"status": "stopped"}` or
`{"status": "starting"}`.

Starting can fail with `403` when your plan's app limit is reached.

### `GET /apps/{name}/deployments`

The 20 most recent deployments, newest first: `{"deployments": [Deployment, …]}`. Scope:
`apps:read`.

### `GET /apps/{name}/deployments/{id}`

One deployment. Scope: `apps:read`.

### `GET /apps/{name}/deployments/{id}/log`

The build log: `{"log": "…"}`. Long logs keep their end. Scope: `apps:read`.

**Follow it live:** send `Accept: text/event-stream` and the log arrives as
[server-sent events](https://html.spec.whatwg.org/multipage/server-sent-events.html) while the
deployment runs:

| Event | `data` | `id` |
|---|---|---|
| `log` | New output (several `data:` lines for several lines of text). | The offset after this text. |
| `status` | The deployment's new status, for example `deploying`. | |
| `end` | The final status (`running`, `failed`, `done`, `cancelled`). The stream ends. | |

Each connection lasts at most about 50 seconds. To continue, reconnect with the last `id` you
received in the `Last-Event-ID` header (or `?from=`): the stream resumes after that text,
without repeating it. Browsers' `EventSource` does this by itself.

```bash
from=0
while :; do
  curl -sN -H "Authorization: Bearer $FC_TOKEN" -H 'Accept: text/event-stream' \
    "https://api.fodycloud.example/v1/apps/my-api/deployments/42/log?from=$from" > chunk.txt
  sed -n 's/^data: //p' chunk.txt
  grep -q '^event: end' chunk.txt && break
  from=$(sed -n 's/^id: //p' chunk.txt | tail -1); from=${from:-0}
done
```

Errors: `429` `too many open log streams; close a log page first` (5 streams per account).

## Logs

### `GET /apps/{name}/logs?tail=200`

What the running app printed: `{"log": "…"}`. `tail` is the number of lines, from 1 to 1000
(default 200). Scope: `apps:read`.

Logs are written by your app's code and by its visitors' requests. Treat them as untrusted text.

## Metrics

### `GET /apps/{name}/metrics?range=24h`

The app's CPU, memory and requests, with its plan's limits. Scope: `apps:read`.

| `range` | Points |
|---|---|
| `24h` (default) | one per minute |
| `7d` | one per hour |
| `30d` | one per hour |

```json
{
  "range": "24h",
  "step": "minute",
  "cpu_limit_percent": 25,
  "memory_limit_bytes": 268435456,
  "points": [
    { "t": "2026-11-19T09:42:00Z", "cpu_percent": 1.8, "memory_bytes": 41943040, "requests": 12 }
  ]
}
```

`cpu_percent` is relative to one CPU (100 = one full core). `requests` counts the requests
during the minute (or hour). Minute points are kept 7 days, hourly points 30 days.

## Environment variables

### `GET /apps/{name}/env`

The variables, with values masked (only the first two characters show):
`{"env": {"API_KEY": "sk**********"}}`. Scope: `apps:read`.

With `?reveal=true` and an `apps:write` token, the real values are returned.

### `PATCH /apps/{name}/env`

Adds, changes or removes variables. A `null` value removes the variable. Scope: `apps:write`.

```json
{ "API_KEY": "new-value", "OLD_SETTING": null }
```

Response `200`: `{"deployment": Deployment}`. A running app is restarted to pick up the change.

Errors (`422`): `invalid variable name "…"`, `PORT is set by FodyCloud`,
`at most 100 environment variables per app`, `value of … is too long (max 32 KB)`.

## Storage and databases

See [Keep your app's files](../user/storage.md) and [Databases for your app](../user/databases.md)
for what each kind does. `{kind}` is `volume`, `postgres` or `valkey`.

### `GET /apps/{name}/services`

The app's storage: `{"services": [Service, …]}`. Scope: `apps:read`.

```json
{
  "kind": "postgres",
  "url": "postgresql://app_12:…@172.30.0.3:5432/app_12",
  "limit": "1G",
  "used_bytes": 8421376,
  "measured_at": "2026-11-20T09:40:00Z",
  "read_only": false,
  "created_at": "2026-11-20T09:12:44Z"
}
```

`url` contains the password; a volume has `"path": "/data"` instead. `read_only` is `true` when
the database is over its size limit, and `blocked` when its connections are refused.

### `POST /apps/{name}/services/{kind}`

Adds a volume, database or cache, and restarts the app to use it. Scope: `apps:write`. Response
`202`: the Service.

Errors: `403` `the free plan doesn't include a persistent volume`; `422` `this app already has a
PostgreSQL database`.

### `DELETE /apps/{name}/services/{kind}?confirm={name}`

Deletes it **with its data**, and restarts the app without it. `confirm` must repeat the app's
name. Scope: `apps:write`. Response `204`.

## Health check

### `PUT /apps/{name}/health-check`

Sets the path checked before each new deployment goes live: `{"path": "/healthz"}`. An empty
path turns the check off. It applies from the next deployment. Scope: `apps:write`. Response
`200`: the App.

Errors (`422`): `the health-check path must start with / and contain only URL characters (no
spaces), for example /healthz`.

## Custom domains

### `POST /apps/{name}/domains`

Attaches a domain: `{"hostname": "www.my-portfolio.com"}`. Requires a plan with custom domains.
Scope: `apps:write`.

Response `202`:

```json
{
  "hostname": "www.my-portfolio.com",
  "verified": false,
  "instructions": "Create a CNAME record www.my-portfolio.com → my-portfolio.apps.fodycloud.example (or an A record www.my-portfolio.com → 203.0.113.10 for an apex domain)"
}
```

### `GET /apps/{name}/domains/{hostname}`

Checks the DNS record. Once it points to FodyCloud, the domain becomes `verified`, the app
restarts to serve it, and HTTPS is set up automatically. Call it again after you change your
DNS. Scope: `apps:read` to check; the check that verifies a new domain restarts the app, so use
an `apps:write` token for it.

### `DELETE /apps/{name}/domains/{hostname}`

Detaches a custom domain; it stops serving the app (a running app restarts if the domain was
verified). Scope: `apps:write`. Response `204`; `404` if the app doesn't have this domain.

Adding a domain beyond the plan's `max_custom_domains` fails with `403`
`the student plan allows 5 custom domains per app; remove one first`.

## Related

- [Authentication](authentication.md)
- [Builds and runtime](build-and-runtime.md)
- [Deploy from CI](ci-cd.md)
