# FodyCloud

**Your code. Your deployment. Your cloud.** FodyCloud is cloud hosting for students. They deploy
apps from **public** GitHub, GitLab or Codeberg repositories: FodyCloud detects the language and
framework, the student confirms them, and the app is built, run in a resource-limited container,
and served over HTTPS at `https://<app>.apps.<domain>`.

FodyCloud is open source and built only with open-source software. It is still pre-production.

## Where to start

| You are… | Read |
|---|---|
| A student or a developer using a FodyCloud platform | the [user and developer guides](docs/README.md), also served at `https://<domain>/docs` |
| On the FodyCloud team, using the admin panel | the [admin guide](docs/admin/overview.md), also served at `https://admin.<domain>/docs` |
| Installing or running a platform | the [operations handbook](docs/operations/README.md) |
| Developing FodyCloud | [CONTRIBUTING.md](CONTRIBUTING.md), then the READMEs of [internal/](internal/README.md), [terraform/](terraform/README.md) and [qemu/](qemu/README.md) |
| Reporting a security issue | [SECURITY.md](SECURITY.md): privately, never in a public issue |

## What's in this repository

```
./
├── cmd/fodycloud/     # the binary: serve | migrate | admin …
├── internal/          # the control plane: websites, API, MCP, build worker, business rules
├── docs/              # user, developer and admin guides (served), operations handbook (not served)
├── terraform/         # OpenTofu: provisions an existing server over SSH, ships and builds the code
├── qemu/              # a local test platform in a VM, and the test suites
├── scripts/check.sh   # every static check (formatting, vet, tests, linters)
└── Dockerfile         # the control-plane image, built on the server
```

## A platform's addresses

| Address | Role |
|---|---|
| `<domain>` | Home page, and the user and developer guides at `/docs` |
| `dashboard.<domain>` | Students' sign-in and dashboard (and the guides, for signed-in students) |
| `admin.<domain>` | Admin panel and admin guide, for admin accounts only |
| `api.<domain>` | REST API under `/v1` |
| `mcp.<domain>` | MCP server for AI assistants, at `/mcp` |
| `<app>.apps.<domain>` | Students' apps |

DNS records and certificates: [operations handbook](docs/operations/addresses-and-tls.md).

## License

FodyCloud is free software, licensed under the [GNU Affero General Public License v3.0 or
later](LICENSE). If you run a modified version as a service, the AGPL requires you to offer
your users its source code. Copyright and third-party notices are in [NOTICE](NOTICE); the
names and logo are covered by [TRADEMARKS.md](TRADEMARKS.md). Contributions are signed off
under the DCO ([CONTRIBUTING.md](CONTRIBUTING.md#licensing-and-sign-off)). Everyone taking part
follows the [Code of conduct](CODE_OF_CONDUCT.md); how the project is run is in
[GOVERNANCE.md](GOVERNANCE.md).
