# Admin API

Everything in the admin panel can also be automated through the API, for example to assign
plans from a payment system. This page lists the admin endpoints.

Admin endpoints live under `https://api.fodycloud.example/v1/admin/` and need an **admin API
token**.

## Create an admin API token

1. In the admin panel, find **Admin API tokens**.
2. Type a name that says where the token is used, for example `payments-sync`, and select
   **Create admin token**.
3. Copy the token. **It is shown only once.** Admin tokens start with `fca_`.

Revoke a token from the same card as soon as you no longer need it, or if it may have leaked.

Admin tokens and students' tokens never cross:

| Token | On `/v1/admin/…` | On the other endpoints and MCP |
|---|---|---|
| Admin token (`fca_…`) | allowed | `403 forbidden: admin tokens only work on /v1/admin` (MCP: `401`) |
| Student token (`fc_…`) | `403 forbidden: admins only` | allowed |

## Endpoints

| Method | Path | Body | Does |
|---|---|---|---|
| `GET` | `/v1/admin/users` | | Lists users with plan, apps and status. |
| `PUT` | `/v1/admin/users/{email}/plan` | `{"plan": "student", "valid_until": "2027-06-30T23:59:59Z", "note": "…"}` | Assigns a plan. |
| `POST` | `/v1/admin/users/{email}/ban` | `{"reason": "…"}` | Suspends a user. |
| `DELETE` | `/v1/admin/users/{email}/ban` | | Reinstates a user. |
| `GET` | `/v1/admin/banned-repos` | | Lists banned repositories. |
| `POST` | `/v1/admin/banned-repos` | `{"repo_url": "…", "reason": "…"}` | Bans a repository. |
| `GET` | `/v1/admin/abuse` | | Lists open abuse flags. |

Example:

```bash
curl -X PUT -H "Authorization: Bearer $FC_ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d '{"plan":"student","valid_until":"2027-06-30T23:59:59Z","note":"paid by MoMo, ref 4F21"}' \
  https://api.fodycloud.example/v1/admin/users/student@univ.example/plan
```

Keep admin tokens for automation only, store them as secrets (for example a CI secret or a
password manager), and revoke them when the automation stops.

## Related

- [Admin overview](overview.md)
- [Authentication](../developer/authentication.md)
