# Sign-in with GitHub, GitLab or Codeberg

This page lets students log in to your platform with their GitHub, GitLab or Codeberg account,
in addition to email and password. You need a running platform and an account on each forge
you want to offer.

Each forge is **optional and off by default**: with none configured, the platform offers email
and password only, as before. Email and password always stay available, so you can turn a forge
off again at any time.

GitLab and Codeberg can also be **self-hosted instances**, for example your university's GitLab
or your own Forgejo or Gitea.

## How it works

- Students see **Continue with GitHub** (and so on) on the **Log in** and **Sign up** pages, and
  can connect or disconnect forges on their **Account & API** page.
- FodyCloud reads the forge account's ID, username and **verified primary email**, then drops
  the forge's access token. It never gets access to repositories.
- A new student accepts the terms before the account exists. The account's email is the forge's
  verified one, and it has no password until the student sets one.
- An email that a FodyCloud account already uses is **refused**, never merged: the student logs in
  with their password and connects the forge from the **Account & API** page. A forge account
  can't take over an existing account.
- Accounts are linked by the forge instance's address and the account's numeric ID, not its
  username, so a renamed forge account keeps working and two instances never mix.
- Admins never sign in with a forge: the admin panel keeps its own email and password sign-in.

## 1. Register an OAuth app on each forge

The callback URL is `https://dashboard.<base_domain>/auth/<forge>/callback`, with `<forge>`
being `github`, `gitlab` or `codeberg`.

| Forge | Where | Settings |
|---|---|---|
| GitHub | **Settings → Developer settings → OAuth Apps → New OAuth App** (on an organization's settings for a shared app) | **Homepage URL** `https://<base_domain>`; **Authorization callback URL** `https://dashboard.<base_domain>/auth/github/callback`. Then **Generate a new client secret**. |
| GitLab | **Edit profile → Applications** (or an admin's **Admin area → Applications** on a self-hosted instance) | **Redirect URI** `https://dashboard.<base_domain>/auth/gitlab/callback`; **Confidential** ticked; scope `read_user` only. |
| Codeberg, Forgejo, Gitea | **Settings → Applications → Manage OAuth2 applications** | **Redirect URIs** `https://dashboard.<base_domain>/auth/codeberg/callback`; **Confidential client** ticked. |

Each forge shows a **client ID** and a **client secret**. Keep the secret safe: it goes into
`terraform.tfvars`, never into the repository.

## 2. Configure the platform

In `terraform.tfvars`, add a block for each forge to offer:

```hcl
oauth_github = {
  client_id     = "Ov23li..."
  client_secret = "..."
}

oauth_gitlab = {
  client_id     = "..."
  client_secret = "..."
  # url   = "https://gitlab.example.edu"   # a self-hosted instance (default: https://gitlab.com)
  # label = "University GitLab"             # the button's name (default: GitLab)
}

oauth_codeberg = {
  client_id     = "..."
  client_secret = "..."
  # url   = "https://git.example.org"      # another Forgejo or Gitea (default: https://codeberg.org)
  # label = "Our Forgejo"                   # the button's name (default: Codeberg)
}
```

Then run `tofu apply`. The control plane restarts with the new settings, which takes a few
seconds. In its log, each forge appears as `sign-in with a forge enabled`.

## 3. Check

1. Open `https://dashboard.<base_domain>/login`: it shows **Continue with …** for each forge.
2. Select one, log in on the forge, and allow the app. You reach **Create your FodyCloud
   account**, with the forge account's email.
3. Accept the terms and select **Create account**: you're on the apps page.

If the forge shows an error about the redirect URI, its callback URL doesn't match step 1
exactly (`https`, `dashboard.`, no trailing slash).

## Turn a forge off

Remove its block from `terraform.tfvars` and run `tofu apply`. Its button disappears and its
sign-in stops; nothing is deleted:

- students with a password log in with it;
- students without one (who signed up with that forge) select **Forgot your password?** on the
  login page to set one, which needs [email delivery](install.md#1-configure) (`smtp_*`).
  Without SMTP, ask them to **Set a password** on their **Account & API** page before you turn
  the forge off;
- their link to the forge stays on their **Account & API** page, marked as no longer offered,
  and works again if you turn the forge back on.

Changing a self-hosted instance's `url` counts as a new forge: accounts linked to the old address
no longer match. Students connect the new one from the **Account & API** page.

## Privacy

The privacy policy (`docs/legal/privacy.md`) already covers this: it lists the forge account's
ID, username and email, "when this platform offers it". Nothing to change unless you edited the
policy.
