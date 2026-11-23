# Plans and limits

Every account has a **plan** that sets how many apps you can run and how many resources each
one gets. This page lists the plans and the limits that apply to everyone.

## Plans

Every new account starts on the **free** plan.

<!-- fodycloud:plans -->
| Plan | Apps | CPU per app | Memory per app | Disk per app | Custom domains per app | Idle sleep | Stopped when unused | Volume | PostgreSQL | Valkey cache |
|---|---|---|---|---|---|---|---|---|---|---|
| free | 2 | 0.25 vCPU | 256 MB | 1 GB | 0 | after 30 min | after 30 days | no | 100 MB, 5 connections | 32 MB |
| student | 5 | 1 vCPU | 1 GB | 5 GB | 5 | never | never | 1 GB | 1 GB, 20 connections | 128 MB |
| research | 10 | 2 vCPU | 4 GB | 20 GB | 10 | never | never | 5 GB | 5 GB, 50 connections | 512 MB |

Your current plan, its limits and its end date are on the **Account & API** page.

## Get another plan

There is no online payment yet. Plans are assigned by the FodyCloud team: contact them to get
a student or research plan. A plan can have an end date.

- **A week before the end date**, you receive a reminder email, and another one the day before.
- **When the plan ends**, your account goes back to the free plan automatically. Your apps get
  the free plan's resources. If you have more apps than the free plan allows, the extra apps are
  **stopped, never deleted**: start them again after you get a bigger plan, or delete others.

## What the limits mean

- **CPU:** the share of a processor your app can use. A small website or API needs very little;
  a long computation will be slowed down to the limit.
- **Memory:** if your app uses more, it is stopped and restarted. Reduce memory use, for
  example by loading large files lazily.
- **Disk:** space for files your app writes while running. Files written by your app are
  **lost when it is redeployed**, except in its volume.
- **Volume:** the size of the folder `/data`, whose files are kept across deployments. See
  [Keep your app's files](storage.md).
- **PostgreSQL, Valkey cache:** the size of each app's database and how many connections it may
  open, and the memory of its cache. See [Databases for your app](databases.md).

## Idle sleep

On plans with idle sleep, an app that receives no visits for the time shown is put to sleep to
free resources. The next visitor sees a **Waking up…** page for a few seconds, then your app
loads normally.

To avoid the delay, use a plan without idle sleep.

## Unused apps

On plans with a "Stopped when unused" delay, an app that has had **no visits and no
deployments** for that long is stopped, to free resources for other students:

1. You receive an email titled **Your FodyCloud app … will be stopped on …**, and your app's
   page shows the date.
2. To keep the app, visit it or deploy it again before that date. The warning disappears within
   a minute.
3. Otherwise the app is stopped on that date, and you receive a second email.

**A stopped app is never deleted.** Start it again from its page whenever you need it.

## Watch your app's resources

Your app's page has a **Metrics** card with its CPU, memory and requests, per minute over the
last 24 hours, or per hour over 7 or 30 days. The dashed line is your plan's limit.

- Memory close to the limit: your app may be stopped and restarted when it goes over.
- CPU at the limit for a long time: your app is slowed down, and on the free plan it may be
  stopped (see Fair use below).

## Fair use

FodyCloud is shared by many students, so these limits apply to everyone:

| Limit | Value |
|---|---|
| Builds | 20 per hour per account |
| Build duration | 15 minutes |
| Repository size | 500 MB |
| API and AI-agent requests | 120 per minute per account |
| Outgoing email (port 25) | blocked: use an email service's API instead |

An app that uses all its CPU for a long time (30 minutes by default) is reported to the FodyCloud
team. On the free plan, an app reported twice within 24 hours is stopped, and you receive an
email. Cryptocurrency mining and attacks on other systems are forbidden and lead to suspension.

## Related

- [Account and security](account-and-security.md)
- [Use your own domain](custom-domains.md)
