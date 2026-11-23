# Security policy

FodyCloud runs other people's code on shared servers, so security reports matter to us. Thank
you for taking the time to send one.

## Report a vulnerability

**Please don't open a public issue.** Report privately, in either of these ways:

- **GitHub:** [Report a vulnerability](https://github.com/fodydev/fodycloud/security/advisories/new)
  (the repository's **Security** tab). This is the preferred channel: we can discuss the
  report, credit you and publish an advisory in the same place.
- **Email:** [security@fodydev.org](mailto:security@fodydev.org).

Include what you can of:

- the affected component (control plane, builder, deployer, dashboard, API, MCP server, the
  OpenTofu configuration or a server script) and the version (the `version` of `GET /v1/about`,
  or a commit);
- the steps to reproduce it, and what an attacker gains (another student's data, escape from an
  app container, the control plane, the host…);
- a proof of concept, if you have one.

## What happens next

| Step | Target |
|---|---|
| We acknowledge your report | within 5 days |
| We confirm the issue and tell you its severity | within 14 days |
| We release a fix | depends on the severity; we keep you informed at least every 14 days |
| We publish an advisory | when the fix is out, crediting you unless you prefer otherwise |

We follow coordinated disclosure: please give us **90 days** from your report, or until a fix
is released if that comes first, before you publish details. If we need more time, we'll explain
why and agree on a date with you.

## Supported versions

FodyCloud is pre-production and has no long-term releases yet. Security fixes go to the `main`
branch and to the latest release. Operators should follow
[docs/operations/upgrades.md](docs/operations/upgrades.md) to deploy them.

## Scope

**In scope:** the code in this repository, and the configuration it installs on a server. That
includes anything that breaks the isolation the platform promises (see the
[security model](docs/operations/security.md#trust-boundaries)):

- one student reaching another student's apps, data, logs, databases or account;
- student code (in a build or an app) escaping its container, reaching the control plane,
  the Docker socket, the platform's databases or the host;
- bypassing plan limits, bans, rate limits, token scopes or email verification;
- authentication, session, CSRF and injection flaws in the dashboard, admin panel, API or MCP
  server;
- secrets leaking through logs, URLs, the source archive at `/source`, or backups.

**Out of scope:**

- vulnerabilities in a student's own app, or in the public repositories it's built from;
- denial of service by volume (flooding), and findings that need a compromised admin account
  or physical access to the server;
- third-party services an operator chooses (GitHub, GitLab, Codeberg, the SMTP or S3 provider,
  Let's Encrypt): report those to their maintainers, though we'd like to hear if FodyCloud uses
  them unsafely;
- missing best-practice headers or settings with no demonstrated impact.

## Testing rules

- **Test on your own platform**, not on a platform other people use: the
  [local test VM](qemu/README.md) runs the whole platform on your computer.
- If you must test a live platform, use only accounts you own, don't access or change other
  people's data, don't degrade the service, and stop and report as soon as you find an issue.

We won't take legal action against anyone who follows this policy in good faith, and we'll ask
platform operators running FodyCloud not to either.
