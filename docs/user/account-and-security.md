# Account and security

This page covers your password, your email address, access tokens, deleting your account, and
what happens if your account is suspended. Everything is on the dashboard's **Account & API**
page.

If this platform offers it, you can also log in with your GitHub, GitLab or Codeberg account: see
[Log in with GitHub, GitLab or Codeberg](sign-in-with-a-forge.md).

## Confirm your email address

After you sign up, FodyCloud sends an email titled **Confirm your FodyCloud email**. Select the
link in it. Until you do, you can log in but you can't create or deploy apps.

If the email is lost, select **Resend the link** in the banner at the top of the dashboard. Check
your spam folder too.

## Reset a forgotten password

1. On the login page, select **Forgot your password?**
2. Enter your email address and select **Send a reset link**.
3. Open the email titled **Reset your FodyCloud password** and select the link. It works once,
   for one hour.
4. Choose a new password of 10 to 72 characters.

Resetting your password **logs you out everywhere and revokes all your API tokens**. This
protects your account if someone else knew your old password. Create new tokens afterwards.

## Change your password

1. On the **Account & API** page, find **Change password**.
2. Enter your current password, then the new one twice (10 to 72 characters).
3. Select **Change password**.

You stay signed in here; **your other sessions are signed out**, and you receive an email
titled **Your FodyCloud password was changed**. Your API tokens keep working.

If you receive that email without having changed your password, reset it right away (above).

## Change your email address

1. On the **Account & API** page, find **Change email**.
2. Enter the new address and your password, then select **Send the confirmation link**. If your
   account has no password (you signed up with GitHub, GitLab or Codeberg), there is no password
   field: log in again first if your last login was more than 15 minutes ago.
3. Open the email titled **Confirm your new FodyCloud email**, sent to the new address, and select
   the link. It works once, for one hour.

Nothing changes until you open the link: until then, you keep using your current address. Your
current address is told about the request, and again when the change is done.

An address already used by another FodyCloud account can't be used.

## Delete your account

Deleting your account removes it and **all your apps** right away: their containers, deployments,
settings, environment variables, domains and metrics. **It can't be undone.** Your repositories
aren't affected.

1. On the **Account & API** page, find **Delete my account**.
2. Enter your password, and type your email address to confirm. An account without a password
   has no password field, but needs a login within the last 15 minutes.
3. Select **Delete my account and my apps**.

You receive an email confirming the deletion. Encrypted backups of the platform age out within
6 months (see the [Privacy policy](../legal/privacy.md)).

## Keep your account safe

- Use a password you don't use anywhere else, ideally from a password manager.
- Never put a password, token or secret in your repository: your repository is public. Use
  [environment variables](environment-variables.md).
- After 5 wrong passwords, logging in to your account is blocked for 15 minutes.

## API tokens

Tokens let scripts, CI pipelines and AI assistants use FodyCloud on your behalf. You create
them on the **Account & API** page.

- A token is shown **only once**, when you create it. Copy it into a safe place.
- Untick **Allow changes** to create a read-only token, which can look at your apps but not
  change them.
- Revoke a token as soon as you no longer need it, or if it may have leaked.

The [developer guide](../developer/authentication.md) explains how to use tokens.

## If your account is suspended

The FodyCloud team can suspend an account that breaks the rules, for example for cryptocurrency
mining or attacks. You receive an email with the reason. While your account is suspended:

- your apps are stopped;
- you can't log in, and your tokens no longer work.

If you think it's a mistake, contact the FodyCloud team with your account's email address.

## Related

- [Plans and limits](plans-and-limits.md)
- [Authentication for developers](../developer/authentication.md)
