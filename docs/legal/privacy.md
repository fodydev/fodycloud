# Privacy policy

**Effective date:** %EFFECTIVE_DATE% · **Version:** %LEGAL_VERSION%

This policy explains what personal data **%OPERATOR_NAME%** ("we") collects when you use
FodyCloud at `fodycloud.example`, why, how long we keep it, and your rights. Contact:
**%OPERATOR_EMAIL%**.

## What we collect

| Data | Why |
|---|---|
| Your email address | Your account: sign-in, confirmation and password-reset emails, notices about your apps and plan. |
| Your password, **only as a one-way hash** (bcrypt) | Checking your password at sign-in. We can't read it. |
| Session cookies and API tokens, stored as hashes | Keeping you signed in; letting your scripts and assistants act for you. |
| Your apps: repository address, settings, environment variables, build logs, runtime logs | Building and running your apps, and showing you their logs. |
| What your apps store in their persistent volume, database and cache (it may include personal data of your app's users) | Running your apps and backing up their data. You are responsible for that data; we process it only for you. |
| Your apps' metrics: CPU, memory, number of requests | Showing them to you, enforcing plan limits, detecting abuse. |
| IP addresses of requests to the platform and to your apps | Security (rate limits, abuse detection) and troubleshooting. |
| Your plan and its history; the date you accepted these policies | Running the service and keeping records of our agreements. |

We don't sell your data, we don't show ads, and we don't use your data for profiling. The code
of your apps comes from public repositories: it is public by definition.

Your apps' visitors are your responsibility: if your app collects their personal data, you must
inform them and protect it.

## Cookies

The platform uses only cookies needed to work: a session cookie that keeps you signed in, and a
cookie that tells the home page you are signed in. There are no analytics or advertising
cookies.

## How long we keep it

| Data | Kept |
|---|---|
| Account data | As long as your account exists; deleted on request. |
| Metrics | 7 days per minute, 30 days per hour. |
| Your apps, their settings and build logs | As long as the app exists; deleted with it. |
| Your apps' volumes and databases | As long as you keep them; deleted with them or with the app. |
| Your apps' caches | In memory only; emptied when the cache restarts. |
| Runtime logs and platform logs | Rotated by size (a few megabytes per app, tens of megabytes per platform service), so they are overwritten after a short time. |
| Backups (the platform's database, your apps' databases and volumes) | Up to 6 months, encrypted, then overwritten. |

## Where it is stored and who processes it

Your data is stored on servers located in **%OPERATOR_DATA_LOCATION%**. We use providers to run
the service, bound by confidentiality: our hosting provider, our email provider (to send you
emails), and our backup storage provider (encrypted backups they can't read). We disclose data to
authorities only when the law requires it.

## Security

Passwords and tokens are stored only as hashes; connections are encrypted (HTTPS); apps run
isolated from each other and from the platform; backups are encrypted. No system is perfectly
secure: if a breach affects your data, we will tell you as the law requires.

## Your rights

You can **correct** your email address and **delete** your account and its data yourself, on the
dashboard's **Account & API** page. You can also ask to **access**, **export** or **delete** your
personal data, or object to how we use it, by writing to %OPERATOR_EMAIL%. We answer within 30
days. You may also complain to
the data protection authority of %OPERATOR_COUNTRY%.

## Changes

We will announce significant changes to this policy by email or in the dashboard before they
apply.
