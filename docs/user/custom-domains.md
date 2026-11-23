# Use your own domain

Every app gets an address like `https://my-app.apps.fodycloud.example`. This page shows how to
also serve it on a domain you own, such as `www.my-portfolio.com`.

**You need:**

- a plan that includes custom domains: the free plan doesn't, and each plan sets how many each
  app may have (see [Plans and limits](plans-and-limits.md));
- a domain name, and access to its DNS settings at your registrar or DNS provider.

## 1. Add the domain to your app

1. Open your app's page and find the **Custom domains** card.
2. Type the domain, for example `www.my-portfolio.com`, and select **Add domain**.

The dashboard shows the DNS record to create.

## 2. Create the DNS record

At your DNS provider, create **one** of these records:

| Your domain | Record type | Name | Value |
|---|---|---|---|
| A subdomain, such as `www.my-portfolio.com` | `CNAME` | `www` | `my-app.apps.fodycloud.example` |
| The bare domain, such as `my-portfolio.com` | `A` | `@` | the IP address shown in the dashboard |

Replace `my-app` with your app's name. A `CNAME` is better when you can use one: it keeps
working if FodyCloud's address changes.

DNS changes can take from a few minutes to a few hours to spread.

## 3. Check the domain

In the **Custom domains** card, select **Check DNS** next to your domain.

- When the record is found, the domain is verified, your app restarts to use it, and HTTPS is
  set up automatically. The first visit can take a few extra seconds while the certificate is
  issued.
- When it isn't found yet, nothing changes: wait a little and check again.

Your app stays reachable at its FodyCloud address too.

## Rules

- Each domain can belong to only one app.
- Each app can have as many custom domains as your plan allows (for example 5 on the student
  plan). The **Custom domains** card shows how many you use.
- Addresses under `fodycloud.example` itself can't be added as custom domains.

## Remove a domain

1. In the **Custom domains** card, select **Remove** next to the domain.
2. Delete the DNS record at your DNS provider, unless you point it somewhere else.

The domain stops serving your app at once (your app restarts if the domain was verified). It
can then be added to another app.

## Troubleshooting

| Message | Cause and fix |
|---|---|
| custom domains are not included in the free plan | Your plan doesn't include custom domains. Ask the FodyCloud team for a plan that does. |
| the student plan allows 5 custom domains per app; remove one first | The app has reached your plan's limit. Remove a domain you no longer use. |
| invalid hostname | Type only the domain, without `https://` or a path: `www.my-portfolio.com`. |
| www.my-portfolio.com is already attached to an app | Another app uses this domain. |
| The browser warns about the certificate | The certificate is issued on the first visits after verification. Wait a minute and reload. If it persists, check that the DNS record still points to FodyCloud. |
