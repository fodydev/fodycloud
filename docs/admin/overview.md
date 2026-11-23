# Admin overview

This guide is for the FodyCloud team: the people who assign plans, handle abuse and support
students. It explains the admin panel and the admin API.

**You need:** an admin account. The first admin account is created when the platform is
installed. To give another person an admin account, ask the platform's operator.

## Admin accounts are not student accounts

An admin account is only for administering FodyCloud:

- it signs in to the admin panel only, never to the dashboard;
- it can't deploy apps or create student API tokens;
- an email address belongs either to an admin or to a student, never both.

To try FodyCloud as a student, for example to reproduce a student's problem, sign up on the
dashboard with a different email address.

## Sign in to the admin panel

1. Go to [admin.fodycloud.example](https://admin.fodycloud.example).
2. Sign in with your FodyCloud email and password.

The admin panel has its own sign-in, separate from the dashboard:

- Only admin accounts can sign in. A student's email gets "Invalid email or password", the
  same answer as a wrong password.
- An admin session lasts **8 hours**. Select **Log out** when you leave a shared computer.
- Being signed in to the dashboard doesn't sign you in to the admin panel, and the reverse.
- The **Storage and databases** card sums the apps' persistent volumes, PostgreSQL databases and
  Valkey caches, with how many databases are full (read-only) or blocked. A blocked database also
  sends you an email: its app kept writing past its size limit.
- The **Admins** card at the bottom of the panel lists every admin account. Admins are added and
  removed by the platform's operator.

## What you can do

| Task | Where |
|---|---|
| Give a student or research plan, set its end date | [Plans](plans.md) |
| Suspend or reinstate an account | [Users and suspensions](users-and-suspensions.md) |
| Review apps flagged for CPU abuse | [Abuse and repository bans](abuse-and-bans.md) |
| Block a repository from being deployed | [Abuse and repository bans](abuse-and-bans.md#ban-a-repository) |
| Automate any of these | [Admin API](api.md) |

Every change takes effect immediately on running apps, and plan changes are recorded in an audit
log.

## Two ways to administer

| Tool | Best for |
|---|---|
| **Admin panel** (`admin.fodycloud.example`) | Day-to-day work. |
| **Admin API** (`https://api.fodycloud.example/v1/admin/…`) | Automation, for example assigning plans from a payment system, with an admin API token. |

Admin actions are never available over MCP, so an AI assistant can't change plans or suspend
users.

## What students see

To answer students' questions, read the [user guide](../user/getting-started.md) they use.
Running the servers (installation, backups, upgrades) is the operator's job and isn't covered
here.
