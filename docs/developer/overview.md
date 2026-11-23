# Developer overview

FodyCloud can be driven by code as well as by the dashboard. This guide is for developers who
automate deployments, integrate FodyCloud in a CI pipeline, or let an AI assistant manage their
apps.

## Three ways in, the same rules

| Interface | Address | Use it for |
|---|---|---|
| Dashboard | `https://dashboard.fodycloud.example` | Day-to-day use by people. |
| REST API | `https://api.fodycloud.example/v1` | Scripts, CI/CD, your own tools. |
| MCP server | `https://mcp.fodycloud.example/mcp` | AI assistants such as Claude Code. |

All three apply the same rules: ownership, plan limits, email verification, build rate limits.
Nothing is possible through the API that isn't possible in the dashboard, and admin actions are
never available over MCP.

## Quick start with curl

This walk-through deploys an app from the terminal in about five minutes. You need `curl`, and
`jq` to read the JSON responses.

### 1. Create a token

On the dashboard's **Account & API** page, create a token with **Allow changes** ticked, then
store it in your shell:

```bash
export FC_TOKEN='fc_…'          # paste your token
export FC_API='https://api.fodycloud.example/v1'
```

Check it works:

```bash
curl -s -H "Authorization: Bearer $FC_TOKEN" "$FC_API/me" | jq '{email, plan: .plan.plan}'
```

You get your email and plan. A `401` means the token is wrong or revoked.

### 2. Create the app

Without a `stack`, FodyCloud analyses the repository and waits for your review. Nothing is
deployed yet.

```bash
curl -s -X POST -H "Authorization: Bearer $FC_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"api-demo","repo_url":"https://github.com/heroku/node-js-getting-started","branch":"main"}' \
  "$FC_API/apps" | jq '{status: .app.status, job: .deployment.kind}'
```

```json
{ "status": "detecting", "job": "detect" }
```

### 3. Read the detection

Poll the app until its status is `review` (a few seconds), then look at what was detected:

```bash
until [ "$(curl -s -H "Authorization: Bearer $FC_TOKEN" "$FC_API/apps/api-demo" | jq -r .status)" = review ]; do sleep 3; done
curl -s -H "Authorization: Bearer $FC_TOKEN" "$FC_API/apps/api-demo" | jq '.detection | {message, suggested}'
```

The `suggested` object is a complete stack, ready to confirm.

### 4. Confirm the stack and deploy

Send the suggested stack back, changed if needed:

```bash
STACK=$(curl -s -H "Authorization: Bearer $FC_TOKEN" "$FC_API/apps/api-demo" | jq -c .detection.suggested)
curl -s -X PUT -H "Authorization: Bearer $FC_TOKEN" -H 'Content-Type: application/json' \
  -d "$STACK" "$FC_API/apps/api-demo/stack" | jq '{stack, id: .deployment.id}'
```

### 5. Wait for the deployment

```bash
while :; do
  S=$(curl -s -H "Authorization: Bearer $FC_TOKEN" "$FC_API/apps/api-demo/deployments" | jq -r '.deployments[0].status')
  echo "$S"; case $S in running|failed|cancelled) break;; esac; sleep 5
done
```

When it prints `running`, open `https://api-demo.apps.fodycloud.example`.

### 6. Clean up

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE -H "Authorization: Bearer $FC_TOKEN" \
  "$FC_API/apps/api-demo?confirm=api-demo"
```

`204` means the app is deleted.

## Next steps

- [Authentication](authentication.md): token scopes and how to keep tokens safe.
- [API reference](api-reference.md): every endpoint, request and response.
- [Builds and runtime](build-and-runtime.md): how your code is built and run.
- [Deploy from CI](ci-cd.md): deploy on every push.
- [AI assistants (MCP)](mcp.md): let Claude Code or another MCP client manage your apps.
