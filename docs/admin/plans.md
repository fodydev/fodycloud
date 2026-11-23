# Plans

Plans set how many apps a student can run and how many resources each app gets. There is no
online payment: admins assign plans by hand, for example after a payment by mobile money or
for a research project. This page shows how.

## The plans

<!-- fodycloud:plans -->
| Plan | Apps | CPU per app | Memory per app | Disk per app | Custom domains per app | Idle sleep | Stopped when unused | Volume | PostgreSQL | Valkey cache |
|---|---|---|---|---|---|---|---|---|---|---|
| free | 2 | 0.25 vCPU | 256 MB | 1 GB | 0 | after 30 min | after 30 days | no | 100 MB, 5 connections | 32 MB |
| student | 5 | 1 vCPU | 1 GB | 5 GB | 5 | never | never | 1 GB | 1 GB, 20 connections | 128 MB |
| research | 10 | 2 vCPU | 4 GB | 20 GB | 10 | never | never | 5 GB | 5 GB, 50 connections | 512 MB |

Every account starts on **free**. The limits are set when the platform is installed (the
`plans` setting of the OpenTofu configuration).

## Assign a plan

1. In the admin panel, find **Assign a plan**.
2. Fill in the form:
   - **User email:** the student's account email.
   - **Plan:** the plan to give.
   - **Valid until:** the last day of the plan. Leave it empty for a plan without an end date.
   - **Note:** why you assign it, for example `paid by MoMo, ref 4F21`. It is kept in the audit
     log.
3. Select **Save plan**.

The plan applies **immediately**: running apps get the new CPU and memory limits within
seconds, without a restart. The disk limit applies at the app's next deployment.

## End dates and reminders

- Seven days before the end date, the student receives an email, and another one the day
  before.
- On the end date, the account goes back to **free** automatically (checked every 10 minutes),
  and the student receives an email.
- If the student has more running apps than the free plan allows, the extra apps are
  **stopped, never deleted**. The student can delete some or ask for a plan again, then start
  them.

To extend a plan, assign it again with a later **Valid until** date.

## Revoke a plan

To put an account back on free before its end date, assign the **free** plan to it in
**Assign a plan**, with a note explaining why.

## Related

- [Admin API](api.md): assign plans automatically
- What students see: [Plans and limits](../user/plans-and-limits.md)
