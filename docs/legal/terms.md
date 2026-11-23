# Terms of service

**Effective date:** %EFFECTIVE_DATE% · **Version:** %LEGAL_VERSION%

These terms are an agreement between you and **%OPERATOR_NAME%** ("we"), which runs the
FodyCloud platform at `fodycloud.example`. By creating an account, you accept them and the
[Acceptable-use policy](acceptable-use.md). The [Privacy policy](privacy.md) explains how we
handle your data.

## 1. The service

FodyCloud hosts web applications built from public Git repositories: it builds them, runs them
in containers with limited resources, and serves them over HTTPS. It is meant for learning,
teaching and research projects.

The FodyCloud software is free software, licensed under the
[GNU Affero General Public License](https://www.gnu.org/licenses/agpl-3.0.html), version 3 or
later. These terms cover your use of this platform, not the software. You can download the source
code of the exact version this platform runs at
[https://fodycloud.example/source](https://fodycloud.example/source).

## 2. Your account

- You give a valid email address and keep your password secret. You are responsible for what
  happens with your account and your API tokens.
- One person, one account. You must be old enough to agree to these terms in your country, or
  have your parent's or guardian's permission.
- Tell us at %OPERATOR_EMAIL% if you think someone else uses your account.

## 3. Plans and limits

- Every account starts on the free plan. Other plans are assigned by our team; there is no
  online payment.
- Each plan has limits (apps, CPU, memory, disk) described in the documentation. Free apps may
  be put to sleep when idle, and stopped when unused for a long time; we warn you by email
  before stopping an unused app.
- We may change plans and limits. When a change reduces what you have, we tell you in advance
  when we reasonably can.

## 4. Your apps and your code

- You keep every right to your code and content. You allow us to copy, build, store and serve
  them only to provide the service.
- Your repositories must be public: anything in them is visible to everyone. Never put secrets
  in them; use environment variables.
- **Files written by your app while it runs are not kept** when it is redeployed or restarted,
  except in its persistent volume (`/data`). Your app's volume and database are backed up every
  night, but backups can fail: keep your own copy of important data, and keep your code in its
  repository.
- You are responsible for the data your app stores, including personal data of its users.

## 5. Acceptable use

You follow the [Acceptable-use policy](acceptable-use.md). We may stop an app, block a
repository or suspend an account that breaks it, without notice when the harm is serious or
urgent, and otherwise after telling you.

## 6. Availability and changes

We work to keep the service available, but we provide it **"as is", without guarantee** of
availability, performance or fitness for a particular purpose. We may change, suspend or stop
the service. If we stop it, we will tell you at least 30 days before, when we reasonably can, so
that you can move your apps.

## 7. Liability

To the extent the law allows, we are not liable for indirect damages, lost profits or lost data
resulting from the use of the service, or from its interruption. Nothing in these terms limits
liability that the law doesn't allow to be limited.

## 8. Ending your use

You can stop using the service at any time, and delete your account and its data yourself from
the dashboard's **Account & API** page. We may close an account that breaks these terms, as described in section 5.

## 9. Changes to these terms

We may update these terms. We will announce significant changes by email or in the dashboard
before they apply. Continuing to use the service after that means you accept them.

## 10. Law and contact

These terms are governed by the laws of **%OPERATOR_COUNTRY%**. Questions:
%OPERATOR_EMAIL%.
