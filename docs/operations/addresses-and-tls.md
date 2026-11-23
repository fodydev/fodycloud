# Addresses, DNS and certificates

Each address of a platform has one role. This page lists them, the DNS records they need, and
how their HTTPS certificates are obtained.

## Addresses and DNS records

| Address | Role | DNS record |
|---|---|---|
| `<domain>` | Home page, and the user and developer guides at `/docs`. Signed-in visitors are sent to the dashboard. | `A <domain>` |
| `dashboard.<domain>` | Students' sign-in, sign-up and dashboard; the guides at `/docs` for signed-in students. | `A dashboard.<domain>` |
| `admin.<domain>` | Admin panel and admin guide (`/docs`), for admin accounts only. | `A admin.<domain>` |
| `api.<domain>` | REST API under `/v1`. | `A api.<domain>` |
| `mcp.<domain>` | MCP server at `/mcp`. | `A mcp.<domain>` |
| `<app>.apps.<domain>` | Student apps. | `A *.apps.<domain>` |

`tofu output dns_records` prints them with the server's address. A wildcard `A *.<domain>` plus
`A <domain>` also works.

Students' custom domains point to their app with a `CNAME` to `<app>.apps.<domain>` (or an `A`
record to the server for an apex domain); they configure that themselves.

## Certificates

Traefik obtains certificates with ACME. Three modes, set in `terraform.tfvars`:

| Mode | Settings | Certificates |
|---|---|---|
| Let's Encrypt, HTTP-01 (default) | `acme_email` | One per hostname, issued on the first visit after the router appears. |
| Let's Encrypt, DNS-01 | `acme_email`, `acme_dns_provider`, `acme_dns_env` | A wildcard for `*.apps.<domain>`: new apps get HTTPS without waiting. `acme_dns_provider` is a [lego provider code](https://doc.traefik.io/traefik/https/acme/#providers), such as `cloudflare`. |
| Private ACME CA | `acme_ca_server`, `acme_ca_root_pem` | From your own CA (step-ca, smallstep, …). |
| None | `acme_email = ""` | Self-signed: browsers warn. Only for tests. |

`tofu output tls_mode` shows the active mode. Students' custom domains always use HTTP-01.

**Let's Encrypt rate limits** apply per registered domain: 50 new certificates per week. With
HTTP-01, every student app is one certificate under your domain, so a busy platform should use
DNS-01 (one wildcard).

## Troubleshooting

| Symptom | Check |
|---|---|
| The browser warns about the certificate | `docker compose logs traefik \| grep -i acme` on the server. Common causes: DNS doesn't point to the server yet, port 80 is blocked upstream, or `acme_staging` is still `true`. |
| `NXDOMAIN` or the wrong server | `dig +short dashboard.<domain>` must return the server's address. |
| A new app has no certificate for a minute | Normal with HTTP-01: the certificate is requested on the first visit. |
| A name with no app shows a certificate warning | Normal: no app, no certificate. The "No app here" page is served with Traefik's default certificate. |
