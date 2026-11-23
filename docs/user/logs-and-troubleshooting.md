# Logs and troubleshooting

This page shows where to find what your app printed, and how to fix the most common problems.
Each error is listed with the exact message FodyCloud shows.

## Read the logs

FodyCloud keeps two kinds of logs.

| Log | What it contains | Where |
|---|---|---|
| **Build log** | Everything that happened during one deployment: download, dependency install, build command, start-up check, and how long each part took. | App page → **Latest deployment**, or **Deployments** → **Log** for older ones. |
| **Runtime logs** | What your running app prints (the last 200 lines), for example requests and errors. | App page → **Runtime logs**. |
| **Metrics** | CPU, memory and requests over time, against your plan's limits. | App page → **Metrics**. See [Watch your app's resources](plans-and-limits.md#watch-your-apps-resources). |

While a deployment runs, its log appears line by line as it is written (**Live**). Your app's
runtime logs refresh every 5 seconds. In both:

- **Reading older lines:** scroll up; the log keeps your place while new lines arrive. Select
  **Jump to latest** to go back to the end.
- **Pause** stops the refresh; **Resume** starts it again.
- **Download** saves the log as a text file, for example to share it when you ask for help.

A finished build log ends with how long it took, for example:

```text
==> Build finished in 1m 05s
==> Live at https://my-app.apps.fodycloud.example
==> App started in 9s

==> Deployment succeeded in 1m 14s
```

The **Deployments** list shows the same total in its **Duration** column.

**Start with the end of the build log.** The last lines usually say what failed, and the
deployment's error message sums it up.

## Problems when creating an app

| Message | Cause | Fix |
|---|---|---|
| repository not found or not public (private repositories are not supported) | The repository is private, or the URL is wrong. | Make the repository public, and copy its URL from the browser's address bar. |
| only public repositories on github.com, gitlab.com or codeberg.org are supported | The repository is hosted elsewhere. | Mirror it to GitHub, GitLab or Codeberg. |
| branch not found in repository | The branch doesn't exist, or the name differs (`main` vs `master`). | Check the branch name on the repository page. |
| app name must be 3-32 characters: lowercase letters, digits and single hyphens, starting with a letter | The name has capitals, spaces, `_` or is too short. | Example of a valid name: `my-app-2`. |
| the name "…" is already taken | Another app uses this name. | Choose another name. |
| the free plan allows 2 apps; delete an app or ask an admin for a bigger plan | You reached your plan's app limit. | Delete an app you no longer need, or see [Plans and limits](plans-and-limits.md). |
| confirm your email address first | Your email isn't confirmed yet. | Select the link in the confirmation email, or **Resend the link** in the dashboard. |
| this repository is not allowed on FodyCloud | The repository was blocked by the FodyCloud team. | Contact the team if you think it's a mistake. |

## Problems with the stack

| Message | Cause | Fix |
|---|---|---|
| We couldn't detect how to build this repository. | Nothing FodyCloud recognises at the root or in the first subfolders. | Choose the language and framework yourself. If your app is in a subfolder, set the working directory. See [Choose your stack](stacks.md). |
| Detected golang, which FodyCloud doesn't offer yet. | The language isn't supported. | Only static websites, Python and Node.js are supported for now. |
| review and confirm the detected stack first | You tried to deploy before confirming the stack. | Select **Confirm and deploy** on the review card. |
| custom commands need a run command that starts a server listening on $PORT | You chose **Other (custom commands)** without a run command. | Add the command that starts your server. |

## Problems during the build

| Message | Cause | Fix |
|---|---|---|
| could not detect how to start this app. | No run command was found for a server app. | Change the stack and set a run command, for example `gunicorn --bind 0.0.0.0:$PORT app:app` or `node server.js`. |
| could not prepare the build for this repository (see build log); check the app's stack | The chosen stack doesn't match the code, for example Python chosen for a Node.js project. | Read the end of the build log, then change the stack. |
| the working directory …/ does not exist in this branch | The working directory is misspelled, or exists only on another branch. | Fix the working directory in the stack settings. |
| repository is … MB, the limit is 500 MB | The repository is too big. | Remove large files (videos, datasets, build outputs) from the branch. |
| build timed out after 15m0s | The build took more than 15 minutes. | Check the build log for a step that hangs, such as a command waiting for input. |
| too many builds, try again in … | You started more than 20 builds in an hour. | Wait for the time shown. |
| A line in the build log mentions `npm ERR!`, `ModuleNotFoundError` or `error:` | Your build command or a dependency failed. | Fix it in your project, check it builds on your computer, push, then **Deploy latest**. |

## Problems after the build

| Symptom or message | Cause | Fix |
|---|---|---|
| the app exited during startup (see its output above) | The run command finished instead of keeping a server running. | Make sure the command starts a server and doesn't exit, for example `python app.py` must call `app.run(...)` or start a server. |
| the app crashed and was restarted … time(s) during startup | The app crashes on start. | Read the app's output just above this line in the build log: it shows the real error, often a missing environment variable or module. |
| PHP apps are served by FrankenPHP: they have no run command | A run command was given for a PHP app. | Leave the run command empty; set the **Publish folder** to the folder that contains `index.php`. |
| Laravel shows **500 Server Error**, and the runtime logs say "No application encryption key has been specified" | `APP_KEY` is missing. | Add it as an environment variable (see [PHP apps](stacks.md#php-apps)). |
| A Java app stops right after starting, or the runtime logs show `OutOfMemoryError` | The Java virtual machine needs more memory than your plan gives. | See [Java apps](stacks.md#java-apps): a lighter app, a smaller `JAVA_OPTS`, or a bigger plan. |
| the app didn't answer /healthz with a 2xx or 3xx status within 60s (last: 503) | Your app has a [health check](health-checks.md), and the new version didn't answer its path correctly. The log shows each attempt. | Open the path on your computer and make it answer `200` once the app is ready, check the path in the **Health check** card, or leave it empty to turn the check off. |
| The deployment succeeded but the address shows an error or never loads | The server listens on a fixed port instead of `PORT`, or only on `localhost`. | Listen on `0.0.0.0` and on the port in `$PORT`. |
| The page is blank and files under `/assets/` fail to load | A single-page app built for a sub-path (`base` in `vite.config.js`). | See [Blank page after deploying](stacks.md#blank-page-after-deploying). |
| **Waking up …** page | On plans with idle sleep, your app was stopped after a period without visits. | Nothing to do: the page reloads and the app is back in a few seconds. See [Idle sleep](plans-and-limits.md#idle-sleep). |
| **… is offline** page | The app is stopped, or its last deployment failed. | Start it from the dashboard, or fix and redeploy it. |

When a new deployment fails, **the previous version keeps running**, so your visitors aren't
affected while you fix it.

## Still stuck?

Download the build log and send it to the FodyCloud team with your app's name. Never send your
password or the values of your environment variables.
