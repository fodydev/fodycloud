# Authentication

The API and the MCP server authenticate requests with **API tokens**. This page explains how to
create them, what they can do, and how to keep them safe.

## Create a token

1. Log in to the dashboard and open **Account & API**.
2. Under **API tokens**, type a name that says where the token is used, for example
   `github-actions-portfolio` or `claude-code-laptop`.
3. Keep **Allow changes (apps:write)** ticked for a token that deploys; untick it for a
   read-only token.
4. Select **Create token**, then copy the token. **It is shown only once.**

Tokens start with `fc_`. FodyCloud stores only a hash of each token, so a lost token can't be
recovered: revoke it and create a new one.

## Use a token

Send it in the `Authorization` header of every request:

```bash
curl -H "Authorization: Bearer fc_…" https://api.fodycloud.example/v1/me
```

Never put a token in a URL: URLs end up in logs and browser history.

## Scopes

| Scope | Allows |
|---|---|
| `apps:read` | Reading apps, deployments, logs, environment variable names, plan and usage. |
| `apps:write` | Everything in `apps:read`, plus creating, deploying, changing and deleting apps. |

A request that needs `apps:write` with a read-only token fails with `403` and
`this token is read-only (scope apps:write required)`.

Give each tool the smallest scope it needs: a monitoring script or a status badge only needs
`apps:read`.

## Keep tokens safe

- **One token per use.** If a CI pipeline leaks its token, you revoke that token without
  breaking your other tools.
- **Store tokens as secrets:** GitHub Actions *secrets*, GitLab CI *masked variables*, or a
  password manager. Never commit them, even to a private repository.
- **Revoke tokens you no longer use** from the **Account & API** page. Revocation takes effect
  immediately.
- **Resetting your password revokes all your tokens.** Create new ones afterwards.

## Authentication errors

| Status | Body | Meaning |
|---|---|---|
| `401` | `{"error":"authentication required"}` | No token, or the token is wrong or revoked. |
| `403` | `{"error":"forbidden: this token is read-only (scope apps:write required)"}` | The token lacks the `apps:write` scope. |
| `403` | `{"error":"forbidden: confirm your email address first (…)"}` | Your account's email isn't confirmed yet. |
| `403` | `{"error":"forbidden: admins only"}` | The endpoint is reserved for the FodyCloud team's admin accounts. |
| `404` | `{"error":"not found"}` | The app doesn't exist **or belongs to someone else**. FodyCloud doesn't reveal which. |

A suspended account's tokens stop working and return `401`.

## Related

- [API reference](api-reference.md)
- [Deploy from CI](ci-cd.md)
