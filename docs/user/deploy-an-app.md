# Deploy your own app

This guide shows how to put your own project online, update it after you change your code, and
manage it afterwards.

**You need:** a FodyCloud account with a confirmed email (see
[Getting started](getting-started.md)), and your project in a repository on GitHub, GitLab or
Codeberg.

## Prepare your repository

Check these points before you deploy. Most failed first deploys come from one of them.

- **The repository is public.** FodyCloud can't read private repositories. On GitHub, open
  *Settings → General → Danger Zone → Change visibility*.
- **Your dependencies are listed in the repository:** `requirements.txt` or `pyproject.toml` for
  Python, `package.json` (and its lock file) for Node.js. FodyCloud installs them for you.
- **A web server listens on the port given in the `PORT` environment variable.** FodyCloud
  sends visitors to that port. A server that listens on a fixed port, such as 3000 or 5000, isn't
  reachable. Static websites don't need a server.

  | Language | Example |
  |---|---|
  | Node.js (Express) | `app.listen(process.env.PORT \|\| 3000)` |
  | Python (Flask, FastAPI, Django) | use the run command FodyCloud suggests; it passes `$PORT` to the server |

- **No secrets in your code.** Put passwords and API keys in
  [environment variables](environment-variables.md) instead.

## Create the app

1. On the **Apps** page, fill in the **New app** form:
   - **Name:** the first part of your app's address, for example `my-portfolio` gives
     `https://my-portfolio.apps.fodycloud.example`.
   - **Public repository URL:** the address of the repository page, for example
     `https://github.com/you/my-portfolio`.
   - **Branch:** the branch to deploy, usually `main` or `master`.
   - **Working directory** (optional): the folder of your app inside the repository, if it isn't
     at the root. See [Apps in a subfolder](stacks.md#apps-in-a-subfolder).
   - **Environment variables** (optional): one `KEY=value` per line.
2. Select **Analyse repository**.

FodyCloud checks the repository and the branch, then analyses the code. After a few seconds the
**Review your stack** card shows what it found.

## Review and confirm the stack

The card shows the detected language, framework and version, and the commands FodyCloud will
use.

- If they are right, select **Confirm and deploy**.
- If they are wrong, change the language or framework in the card. The commands update for the
  framework you pick, and you can edit them. Then select **Confirm and deploy**.
- If nothing was detected, choose the language and framework yourself.

[Choose your stack](stacks.md) explains each option and the commands to use.

FodyCloud then builds your app. The **Latest deployment** card shows the build log live. When
it ends with **Deployment succeeded**, your app is online.

## Update your app after a change

FodyCloud doesn't redeploy automatically when you push. After you push a change to the branch:

1. Open your app's page.
2. Select **Deploy latest**.

FodyCloud builds the newest commit. **Your app stays online during the build:** the new
version replaces the old one only once it has started correctly. If the new version fails, the
previous one keeps running.

To deploy on every push without opening the dashboard, see
[Deploy from CI](../developer/ci-cd.md).

## Change the stack later

1. On your app's page, open the **Stack** card and select **Change stack**.
2. Change the language, framework, version, commands or working directory.
3. Select **Save and rebuild**.

## Manage a running app

All these actions are at the top of your app's page.

| Action | What it does |
|---|---|
| **Deploy latest** | Builds and deploys the newest commit of the branch. |
| **Restart** | Restarts the current version without rebuilding it. Useful after the app hangs. |
| **Roll back** | Goes back to the previous version that worked. |
| **Stop** | Stops the app. Visitors see a page saying the app is offline until you start it again. |
| **Start** | Starts a stopped app. |

## Delete an app

Deleting an app removes it, its deployments and its settings. **This can't be undone.** Your
repository isn't affected.

1. At the bottom of your app's page, find **Delete app**.
2. Type the app's name to confirm, then select **Delete permanently**.

The name becomes available again, to you or anyone else.

## If something goes wrong

Open the build log of the failed deployment: the last lines usually say what happened. Then see
[Logs and troubleshooting](logs-and-troubleshooting.md), which lists each error message with
its fix.
