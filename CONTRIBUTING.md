# Contributing to FodyCloud

This guide is for the people who develop FodyCloud itself. It covers how to contribute, the
repository layout, the checks to run, and the conventions for each language. Project-wide rules
(open source only, the two kinds of accounts, MCP annotations, migrations…) are in
[`CLAUDE.md`](CLAUDE.md), which is also the best briefing on how the platform works.

Everyone taking part follows the [Code of conduct](CODE_OF_CONDUCT.md). How decisions are made
is in [GOVERNANCE.md](GOVERNANCE.md).

## How to contribute

- **Found a bug?** Open an issue with the **Bug report** form. **Found a security issue?** Don't
  open an issue: follow [SECURITY.md](SECURITY.md).
- **Want a feature, or a language or framework?** Open an issue first (**Feature request** or
  **Stack request**), so we agree on the design before you write code. Small fixes (typos,
  docs, an obvious bug) can go straight to a pull request.
- **Looking for something to do?** Issues labelled `good first issue` are small and well
  described; `help wanted` ones are larger. Say in the issue that you're on it.

Then:

1. Fork the repository and create a branch from `main`.
2. Make the change, with its tests and its docs (see [Documentation](#documentation)).
3. Run `scripts/check.sh` (below) and, if the change touches the server, the build pipeline or
   the websites, the VM test cycle.
4. Commit with `git commit -s` ([Licensing and sign-off](#licensing-and-sign-off)) and open a
   pull request. The template asks what changed, why, and how you tested it.

A maintainer reviews it, usually within a week. Keep pull requests small: one logical change
each is easier to review and to revert.

## Repository layout

| Path | Content | Read |
|---|---|---|
| `cmd/fodycloud/` | The binary: `serve`, `migrate`, `admin …` (`main.go`, `serve.go`, `admin.go`, `app.go`) | |
| `internal/` | The control plane's packages | [internal/README.md](internal/README.md) |
| `docs/` | User, developer and admin guides (served by the platform) and the operations handbook (not served) | [docs/README.md](docs/README.md), [docs/operations/](docs/operations/README.md) |
| `terraform/` | The OpenTofu configuration that provisions a server | [terraform/README.md](terraform/README.md) |
| `qemu/` | The local test VM and the test suites | [qemu/README.md](qemu/README.md) |
| `scripts/check.sh` | Every static check | below |

## Before each commit

```bash
scripts/check.sh
```

It runs `gofmt`, `go vet`, `go test`, staticcheck (pinned, fetched with `go run`), shellcheck
on every `*.sh` (a local binary, or its pinned container image through Docker or Podman),
`tofu fmt` / `validate`, `reuse lint` (every file has a license, same local-or-container rule)
and `go-licenses` (every Go dependency has an allowed license). All must pass.

For changes that touch the server, the build pipeline or the websites, also run the full cycle on
the test VM ([qemu/README.md](qemu/README.md#test-cycle)).

**CI** (`.github/workflows/check.yml`) runs `scripts/check.sh` on every pull request and push to
`main`, and `scripts/check-signoff.sh` on pull requests; both must pass before merging. The VM
cycle isn't run in CI (it needs about an hour and a VM): reviewers ask for it when a change needs
it, and maintainers run it before each release.

## Conventions

### Go

- **Format and lint:** `gofmt`, `go vet` and staticcheck are clean. Imports are grouped as
  standard library, third party, then `github.com/fodydev/fodycloud/…`.
- **Business rules live in `internal/service`.** `api`, `web` and `mcpserver` parse input, call
  the service and render the result; they never decide.
- **One file per topic** in a package (`service/apps.go`, `service/plans.go`…). When a file
  passes about 400 lines or mixes topics, split it.
- **Small functions with one job.** A function that registers many routes groups them
  (`signInRoutes`, `appRoutes`…); repeated preambles become helpers (`authed`, `publicForm`).
- **Doc comments** on every package and every exported name, starting with the name. Comment
  the *why* (a constraint, a pitfall, a security reason), not what the code already says.
- **Errors:** wrap with context (`fmt.Errorf("git clone: %w", err)`); user-facing messages come
  from `service.InvalidError` / `QuotaError` and read as plain sentences.
- **Types keep things apart.** Students are `*auth.User`, admins `*auth.Admin`; never add a flag
  that lets one stand in for the other.
- **Tests:** table-driven where cases repeat; unit tests run without a database (handlers are
  tested with `httptest`). Behaviour that needs Docker or PostgreSQL is covered by the e2e suite.

### SQL

- The schema is `internal/db/migrations/0001_init.sql`, grouped by topic with comments.
  Pre-production rules are in `CLAUDE.md` (edit it in place; no compatibility code).
- Lower-case identifiers, `snake_case`, `TEXT` over `VARCHAR`, `TIMESTAMPTZ` for times, and an
  explicit `ON DELETE` on every foreign key.

### HTML templates, CSS and JavaScript

- Templates in `internal/web/templates/`; shared partials are named `_*.html`.
- The CSP forbids inline styles and scripts: styles go in `static/app.css`, behaviour in
  `static/app.js`, links to static files through `{{asset "…"}}`.
- Links between the platform's sites use `{{.URL "dashboard" "/path"}}`, never relative paths.
- Labels in templates are the words the user guide quotes: change both together.
- CSS uses the design tokens (`--brand-green`, `--text`…) declared at the top of `app.css`, so
  light and dark themes stay in sync.

### Shell

- `#!/usr/bin/env bash`, then `set -euo pipefail` for scripts that must stop on error (test
  suites use `set -uo pipefail` and count failures instead).
- shellcheck is clean. Disable a rule only on the line that needs it, with the reason in a
  comment; repository-wide exceptions live in `.shellcheckrc` with their reason.
- Quote every expansion, prefer `[[ … ]]`, use functions with a comment saying what they take
  and return.
- Server scripts live in `terraform/files/` and get their settings from `host.env`.

### OpenTofu

- `tofu fmt` is clean. One concern per file (see [terraform/README.md](terraform/README.md)).
- Every variable has a `description`, a `type` and, when it makes sense, a `default` and a
  `validation`.
- Long expressions are split over lines; nested conditionals are written one case per line.
- Pin images and tool versions (`var.images`, `Dockerfile`).

### Documentation

- A user-visible change updates `docs/` in the same commit; the rules are in `CLAUDE.md`
  (Documentation).
- A change to how the platform is installed or run updates `docs/operations/`.
- Each directory with its own tooling has a `README.md` explaining what is there and how to work
  with it.

## Propose a new stack

FodyCloud supports the languages and frameworks in `internal/stacks` (see the
[stacks reference](docs/user/stacks.md)). To add one:

1. **Open a Stack request issue** with the language or framework, a public sample repository
   that should deploy, and the runtime's license: it must be OSI-approved (`CLAUDE.md`, Open-source
   only).
2. **Extend the catalog:** add it to `stacks.Catalog`, map Railpack's detection to it in
   `Detect`, and set its commands, versions and settings in `Railpack()`.
3. **Add a fixture** of Railpack's real output (`railpack-info.json`) for a sample repository to
   `internal/stacks/testdata/`, with a test of `Detect` on it.
4. **Add an e2e build** of the public sample repository to `qemu/e2e/20-more-stacks.sh`: it must
   build, start, and answer on `$PORT`.
5. **Document it** in [docs/user/stacks.md](docs/user/stacks.md) (and the developer docs if it
   has API values).

Read the Railpack notes in `CLAUDE.md` (Gotchas) first: start commands, static sites and build
secrets have pitfalls.

## Commits

- One logical change per commit, with a message that says what changed and why.
- The checks pass, and the VM cycle passes for changes it covers.
- Only open-source dependencies (OSI-approved licenses); check the license before adding one.
  A Go dependency must also be compatible with the AGPL: `GO_ALLOWED_LICENSES` in
  `scripts/check.sh` lists the accepted ones.
- Every commit is signed off (`git commit -s`): see below.

## Releases

For maintainers. FodyCloud uses [semantic versioning](https://semver.org) tags (`v0.3.0`); see
[Upgrade notes](docs/operations/upgrades.md#upgrade-notes) for what each kind of version may
change.

1. On an up-to-date `main`, run the whole VM test cycle
   ([qemu/README.md](qemu/README.md#test-cycle)). Both suites must end with `0 failed`.
2. In a pull request, add the version to the
   [upgrade notes](docs/operations/upgrades.md#upgrade-notes) if operators must do anything (new
   settings, manual steps, a fresh database), and merge it.
3. Tag the merged commit and push the tag:

   ```bash
   git tag -a v0.3.0 -m "FodyCloud v0.3.0"
   git push origin v0.3.0
   ```

4. The **Release** workflow runs the checks and publishes the release, with notes generated from
   the merged pull requests (versions below `v1.0.0` are marked as pre-releases). Edit the notes
   on GitHub to highlight what students and operators will notice.
5. If the release fixes a vulnerability, publish its security advisory ([SECURITY.md](SECURITY.md)).

## Licensing and sign-off

FodyCloud is licensed under the [GNU AGPL-3.0-or-later](LICENSE). You keep the copyright of
your contributions and license them under the same terms; there is no contributor license
agreement.

Instead, every commit carries a `Signed-off-by:` line (`git commit -s`), which certifies the
[Developer Certificate of Origin 1.1](https://developercertificate.org/): you wrote the change,
or have the right to submit it under the project's license, and you agree that it's public and
kept in the history. Use your real name or a name you're known by, with a working email address.
Forgot it? `git commit --amend -s`, or `git rebase --signoff main` for a whole branch.

**License headers.** A new Go, shell, SQL or OpenTofu file starts with:

```go
// SPDX-FileCopyrightText: 2026 FodyDev and the FodyCloud contributors
// SPDX-License-Identifier: AGPL-3.0-or-later
```

with the comment syntax of the language (`#`, `--`), after the shebang or the
`# shellcheck shell=bash` line if there is one. Other files (docs, templates, static files, data)
are covered by `REUSE.toml`. Code copied from elsewhere keeps its own copyright and license:
give them in its header (or in `REUSE.toml` for a vendored file), add the license text to
`LICENSES/` (`reuse download <SPDX-ID>`) and list it in `NOTICE`.

The names "FodyCloud" and "FodyDev" and the logo are covered by [TRADEMARKS.md](TRADEMARKS.md).
