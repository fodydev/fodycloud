# Abuse and repository bans

FodyCloud watches for apps that use their CPU limit for a long time, and lets you block
repositories. This page explains what is detected automatically and what you decide.

## CPU abuse flags

Once a minute, FodyCloud measures every app's CPU use.

- An app that stays at **90 % or more of its CPU limit for 30 minutes** (the `abuse_cpu_minutes`
  setting) is **flagged**, and admins receive an email.
- On the **free** plan, an app flagged **twice within 24 hours is stopped automatically**, and
  its owner receives an email titled **Your FodyCloud app … was stopped**.
- Apps on other plans are only flagged: you decide.

A flag isn't proof of abuse: a student's data-processing job can legitimately use its whole
CPU. Cryptocurrency mining, password cracking and similar activities are abuse.

## Review a flag

The **Abuse flags** table lists open flags: when, which app, its owner, the kind, the measure
and the action taken automatically.

1. Look at the app: its repository, and its runtime logs (sign in to the dashboard as an admin,
   or ask the owner).
2. Decide:
   - **Legitimate use:** select **Resolve**. If it happens often, suggest a bigger plan.
   - **Abuse by one account:** [suspend the user](users-and-suspensions.md#suspend-an-account),
     then resolve the flag.
   - **A project nobody should deploy:** ban the repository, then resolve the flag.

## Ban a repository

A banned repository can't be used for new apps, and new builds of existing apps that use it
fail.

1. In the admin panel, find **Banned repositories**.
2. Enter the repository's URL and a reason, then select **Ban repository**.

Banning a repository doesn't stop apps that are already running: stop them by suspending their
owners, or ask the owners to delete them.

To allow a repository again, select **Allow again** next to it.

## Protections that work without you

| Protection | Effect |
|---|---|
| Egress filtering | Apps and builds can't send email on port 25, reach common mining-pool ports, private networks or the host. |
| Rate limits | Sign-in: 10 per minute per IP, plus a 15-minute lockout after 5 failures on an account. Sign-up and password reset: 5 per hour per IP. API and MCP: 120 per minute per user. Builds: 20 per hour per user. |
| Isolation | Each app runs in its own container with no extra privileges, and apps can't reach each other. |

## Related

- [Users and suspensions](users-and-suspensions.md)
- [Admin API](api.md)
