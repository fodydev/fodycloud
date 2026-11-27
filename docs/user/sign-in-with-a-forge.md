# Log in with GitHub, GitLab or Codeberg

This page shows how to sign up and log in with your GitHub, GitLab or Codeberg account instead of
a password, and how to connect or disconnect one later. You need an account on that site with a
**verified primary email address**.

Each platform chooses which of these sites it offers. If the login page has no **Continue
with …** button, this platform offers email and password only.

## Sign up with GitHub, GitLab or Codeberg

1. On the **Log in** or **Sign up** page, select **Continue with GitHub** (or GitLab, or
   Codeberg).
2. Log in on that site if it asks, and allow FodyCloud to read your profile and email address.
   You come back to FodyCloud's **Create your FodyCloud account** page, which shows the email of
   your account on that site.
3. Accept the terms and select **Create account**.

Your account is created with that email address, already confirmed, and you're on your apps
page. It has no password yet: you log in with the same button. To also log in with a password,
[set one](#set-a-password).

FodyCloud only reads your account's ID, username and email. It never gets access to your
repositories or your other data on that site, and doesn't keep the site's access key.

## Log in

On the **Log in** page, select **Continue with …** for the site you signed up with. If you're
still logged in on that site, you're back on your apps page right away.

## Connect GitHub, GitLab or Codeberg to an existing account

If you already have a FodyCloud account with a password, connect the site from your account
instead of signing up again:

1. Log in with your email and password.
2. On the **Account & API** page, find **Sign-in methods** and select **Connect** next to the
   site.
3. Log in on that site and allow FodyCloud.

You come back to the **Account & API** page with **Codeberg connected. You can now log in with
it.** (or GitHub, or GitLab), and receive an email about it. The emails of your two accounts
don't need to match.

## Set a password

An account created with GitHub, GitLab or Codeberg has no password. To add one:

1. Log out, then log in again with the site's button. For your security, the next step works only
   within 15 minutes of logging in.
2. On the **Account & API** page, find **Set a password**.
3. Enter a password of 10 to 72 characters twice, then select **Set password**.

You can now log in with your email and password too. **Forgot your password?** on the login page
also sets a first password, through a link sent to your email address.

## Disconnect a site

On the **Account & API** page, under **Sign-in methods**, select **Disconnect** next to the
site. You can't log in with it any more, and you receive an email about it.

If your account has no password and this is its only connected site, set a password first: you
need at least one way to log in.

## Errors

| Message | Cause | Fix |
|---|---|---|
| **An account already uses you@example.org. Log in with your password, then connect GitHub on the Account page.** | A FodyCloud account already has the email of your account on that site. | Log in with that account's email and password (or reset it), then [connect the site](#connect-github-gitlab-or-codeberg-to-an-existing-account). |
| **Your GitHub account has no verified primary email address. Verify one on GitHub, then try again.** | The site hasn't confirmed your main email address. | Confirm it in the site's email settings, then try again. |
| **Sign-in with GitHub was cancelled.** | You declined on the site's page. | Try again and allow FodyCloud. |
| **This sign-in link is invalid or has expired. Try again.** | The sign-in took more than 10 minutes, or started in another browser. | Start again from the login page. |
| **This sign-up has expired. Start again.** | More than 15 minutes passed before you accepted the terms. | Start again from the **Sign up** page. |
| **This GitHub account is already connected to a FodyCloud account.** | You (or someone) connected it to another FodyCloud account. | Log in with that account, or disconnect the site there first. |
| **Your account is already connected to GitHub. Disconnect it first.** | An account can have one account per site. | Disconnect the current one, then connect the other. |
| **Set a password before you disconnect GitHub: it is your only way to log in.** | Your account has no password and no other connected site. | [Set a password](#set-a-password) first. |
| **for your security, log out and log in again, then retry within 15 minutes** | Your account has no password, and you logged in more than 15 minutes ago. | Log out, log in again with the site's button, and retry. |
| **This account is suspended.** | The FodyCloud team suspended your account. | See [Account and security](account-and-security.md). |

## If the platform stops offering a site

You can still log in: select **Forgot your password?** on the login page and set a password
through the link sent to your email address. Your account and your apps don't change.
