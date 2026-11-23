# FodyCloud documentation

FodyCloud puts your code online: you give it a public repository, it builds your app and serves
it over HTTPS at `https://<your-app>.apps.fodycloud.example`.

| Address | What you find there |
|---|---|
| [fodycloud.example](https://fodycloud.example) | The home page. |
| [dashboard.fodycloud.example](https://dashboard.fodycloud.example) | Sign in and manage your apps. |
| [fodycloud.example/docs](https://fodycloud.example/docs) | This documentation (inside the dashboard once you're signed in). |
| [api.fodycloud.example](https://api.fodycloud.example) | The REST API, under `/v1`. |
| [mcp.fodycloud.example](https://mcp.fodycloud.example) | The MCP server for AI assistants, at `/mcp`. |

## User guide

For everyone who deploys apps from the dashboard.

- [Getting started](user/getting-started.md): put your first app online in ten minutes.
- [Deploy your own app](user/deploy-an-app.md): prepare your repository, deploy, update and
  manage your app.
- [Choose your stack](user/stacks.md): static websites, Python and Node.js, and what to do when
  the detection is wrong.
- [Environment variables](user/environment-variables.md): settings and secrets.
- [Use your own domain](user/custom-domains.md)
- [Logs and troubleshooting](user/logs-and-troubleshooting.md): every error message and its fix.
- [Plans and limits](user/plans-and-limits.md)
- [Account and security](user/account-and-security.md)

## Developer guide

For developers who automate FodyCloud with the API, CI pipelines or AI assistants.

- [Developer overview](developer/overview.md): a five-minute quick start with curl.
- [Authentication](developer/authentication.md): API tokens and scopes.
- [API reference](developer/api-reference.md)
- [Builds and runtime](developer/build-and-runtime.md): what happens between a push and a
  running app.
- [Deploy from CI](developer/ci-cd.md): GitHub Actions and GitLab CI.
- [AI assistants (MCP)](developer/mcp.md)

## Running FodyCloud

To install and operate your own FodyCloud platform, see the
[operations handbook](operations/README.md) in the repository. Admins of a platform find the
admin guide at `https://admin.fodycloud.example/docs`.
