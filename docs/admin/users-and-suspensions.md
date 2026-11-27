# Users and suspensions

This page shows how to find an account, and how to suspend and reinstate one.

## Find a user

The **Users** table of the admin panel lists every account with:

- its plan and the plan's end date;
- its number of apps and when it joined;
- labels: **admin**, **suspended**, **unverified** (email not confirmed yet).

The email fields of the panel's forms suggest existing accounts as you type.

## Suspend an account

Suspend an account that breaks the [Acceptable-use policy](../legal/acceptable-use.md):
cryptocurrency mining, attacks on other systems, illegal content, or repeated abuse.

1. In the admin panel, find **Suspend or reinstate**.
2. Enter the account's email and a **Reason**. The reason is sent to the user, so write it for
   them, for example `Cryptocurrency mining in app "tools" (abuse flag of 18 November)`.
3. Select **Suspend user**.

What happens immediately:

- all the user's apps are stopped;
- the user is signed out everywhere and all their API tokens are revoked;
- the user can no longer sign in, with a password or with GitHub, GitLab or Codeberg;
- the user receives an email titled **Your FodyCloud account has been suspended**, with your
  reason.

Admin accounts can't be suspended.

## Reinstate an account

1. In **Suspend or reinstate**, enter the email under the **Reinstate** form.
2. Select **Reinstate**.

The user can sign in again. **Their apps stay stopped** until they start them, and they need to
create new API tokens.

## Ban a repository instead

When the problem is a specific project rather than a person (for example a known phishing kit),
[ban the repository](abuse-and-bans.md#ban-a-repository): nobody can deploy it, whatever the
account.

## Related

- [Abuse and repository bans](abuse-and-bans.md)
- [Admin API](api.md)
