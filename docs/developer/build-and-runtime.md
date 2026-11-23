# Builds and runtime

This page explains what happens between your `git push` and a running app: how the code is
analysed, built, started and replaced. Read it when you tune your build, or when you need to
know exactly what your app can rely on.

## The pipeline

1. **Validation.** The repository must be public on github.com, gitlab.com or codeberg.org, and
   the branch must exist.
2. **Analysis** (`detect` deployment). FodyCloud makes a shallow clone of the branch and runs
   [Railpack](https://railpack.com)'s planner on it, without building anything. The result is
   mapped to a FodyCloud stack. If the root holds nothing buildable, up to four subfolders that
   contain a project file (`package.json`, `requirements.txt`, `pyproject.toml`, `Pipfile`,
   `manage.py`, `index.html`) are tried, shallowest first.
3. **Review.** The app waits in `review` until a person (or a client acting for one) confirms
   the stack with `PUT /apps/{name}/stack`.
4. **Build** (`build` deployment). A fresh shallow clone of the latest commit is built into a
   container image with the confirmed stack.
5. **Start and health check.** The new version is started next to the current one. It must
   still be running, and must not have restarted, after a few seconds.
6. **Switch.** Traffic moves to the new version, then the old one is removed.

If a step fails, the deployment is `failed` and **the previous version keeps serving**.

## How the stack drives the build

FodyCloud passes the confirmed stack to Railpack:

| Stack field | Effect |
|---|---|
| `language` | Forces the Railpack provider (`python`, `node`, or `staticfile` for static sites). |
| `version` | Pins the Python or Node.js version. Empty lets Railpack choose, reading files such as `.python-version`, `.nvmrc` or `package.json` → `engines`. |
| `build_command` | Runs after dependencies are installed. |
| `start_command` | The container's command, run with `bash -c`, so `$PORT` and `&&` work. |
| `publish_dir` | For static frameworks, the folder served by a web server instead of a start command. Single-page apps fall back to `index.html` for unknown paths. |
| `root_dir` | The folder used as the build context: the commands run there, and files outside it aren't in the image. |

A `railpack.json` in your repository is overridden by the confirmed stack. Dependencies are
installed by the provider: `pip` or your Python package manager from `requirements.txt`,
`pyproject.toml` or `Pipfile`; `npm`, `pnpm`, `yarn` or `bun` according to your lock file.

## Environment variables during the build

Your app's environment variables are available to every build step, so a build command can use
an API key, for example to fetch content.

- They are passed as **build secrets**: they are **not written into the image layers**, and they
  don't appear in the image's configuration.
- Changing a variable invalidates the build cache for the steps that use it.
- At runtime, the same variables are set in the container's environment.

## Runtime environment

| Item | Value |
|---|---|
| `PORT` | `8080`. Your server must listen on `0.0.0.0:$PORT`. |
| Resources | CPU, memory, process count and disk from your plan (see [Plans and limits](../user/plans-and-limits.md)). |
| File system | Writable, but reset on every deployment. Store lasting data outside the app. |
| Privileges | No root capabilities, no privilege escalation. |
| Network | Outbound internet access. No access to private networks, to the host, to other apps, or to port 25 (SMTP). |
| HTTPS | Terminated by FodyCloud. Your app receives plain HTTP with `X-Forwarded-For` and `X-Forwarded-Proto` headers. |
| Restart policy | A crashed app is restarted automatically. |

## What counts as a successful deployment

The start-up check fails when the run command:

- **exits**, even with code 0: a script that prints something and stops isn't a server;
- **crashes and restarts** during the check.

The deployment's `error` then contains the app's output, for example:

```
the app exited during startup (see its output above); the run command must keep a server
running and listening on $PORT (8080)
```

A deployment that passes the check but listens on the wrong port is reported as `running`, yet
visitors get an error. Always use `$PORT`.

## Caching

Each app has its own build cache, never shared with other users. Dependency installation is
cached until your lock file or dependency list changes, so later builds are faster than the
first one.

## Rollback and restart

- **Restart** starts the same image again: no rebuild, no new commit.
- **Rollback** redeploys the image of the previous successful deployment.
- **Deploy** always builds the latest commit of the branch.

## Limits

| Limit | Value |
|---|---|
| Build duration | 15 minutes |
| Repository size (after clone, without history) | 500 MB |
| Builds per account | 20 per hour |
| Concurrent builds on the platform | shared queue; your build waits in `queued` when builders are busy |

## Related

- [Choose your stack](../user/stacks.md): the catalog and default commands.
- [API reference](api-reference.md)
