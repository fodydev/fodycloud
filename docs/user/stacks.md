# Choose your stack

Your app's **stack** tells FodyCloud how to build and run it: a **language**, a **framework**,
optionally a **version**, and the commands that go with them. This page explains how to pick the
right one, and what to do when your project doesn't fit a preset.

## How the stack is chosen

When you create an app, FodyCloud analyses your repository and suggests a stack. **Nothing is
deployed until you confirm it.** In the **Review your stack** card you can:

- confirm the suggestion with **Confirm and deploy**;
- pick another language or framework: the commands change to that framework's defaults;
- edit the commands;
- choose a version, or leave **Automatic**.

You can change the stack later from the **Stack** card of your app (**Change stack**). Saving
rebuilds the app.

## Available stacks

| Language | Framework | Build command | Run command |
|---|---|---|---|
| Static website | Static files (HTML, CSS, JS) | none | none: the files are served as they are |
| Python | Flask | `pip install gunicorn` | `gunicorn --bind 0.0.0.0:$PORT app:app` |
| Python | FastAPI | `pip install uvicorn` | `uvicorn main:app --host 0.0.0.0 --port $PORT` |
| Python | Django | `pip install gunicorn` | `python manage.py migrate && gunicorn mysite.wsgi --bind 0.0.0.0:$PORT` |
| Python | FastHTML | `pip install uvicorn` | `uvicorn main:app --host 0.0.0.0 --port $PORT` |
| Python | Other (custom commands) | yours | yours |
| Node.js | Node server (Express, Fastify…) | none | `npm start` |
| Node.js | Next.js | `npm run build` | `npm start` |
| Node.js | Nuxt | `npm run build` | `node .output/server/index.mjs` |
| Node.js | Vite / React single-page app | `npm run build` | none: the `dist` folder is served |
| Node.js | Astro (static) | `npm run build` | none: the `dist` folder is served |
| Node.js | Bun server | none | `bun run start` |
| Node.js | Other (custom commands) | yours | yours |
| PHP | PHP (plain PHP, Slim, Symfony…) | none | none: served by a PHP web server |
| PHP | Laravel | automatic | none: served by a PHP web server |
| Go | Go server (net/http, Gin, Echo…) | automatic (`go build`) | `./out` |
| Go | Other (custom commands) | yours | yours |
| Java | Spring Boot (Maven or Gradle) | automatic | `java -jar` on the built jar, with the port set |
| Java | Java application (JAR) | automatic | `java -jar` on the built jar |
| Java | Other (custom commands) | yours | yours |

**Versions:** Python 3.10 to 3.13, Node.js 18, 20, 22 and 24, Go 1.25 to 1.27, Java 17, 21 and
25. **Automatic** picks a recent version, or the one your project asks for (for example in
`package.json` → `engines`, the `go` line of `go.mod`, or the Java version of `pom.xml` or
`build.gradle`). For PHP, the version always comes from `composer.json` → `require.php`
(for example `"php": "^8.3"`); without it, a recent PHP 8 is used.

**Dependencies are always installed for you** before the build command runs: from
`requirements.txt` or `pyproject.toml` for Python, `package.json` for Node.js, `composer.json`
for PHP, `go.mod` for Go, and `pom.xml` or `build.gradle` for Java. The build command is only
for extra steps.

## Adapt the default commands

The defaults assume common file names. Change them to match your project:

- **Flask:** `app:app` means "the object `app` in the file `app.py`". If your file is `run.py`
  and the object is `server`, use `gunicorn --bind 0.0.0.0:$PORT run:server`.
- **FastAPI and FastHTML:** `main:app` means "the object `app` in `main.py`". For `api/server.py`,
  use `uvicorn api.server:app --host 0.0.0.0 --port $PORT`.
- **Django:** replace `mysite` with the name of the folder that contains `wsgi.py`.

Every run command must start a server that keeps running and listens on `$PORT`. FodyCloud
sets `PORT` for you; keep `$PORT` in the command rather than a number.

## Static websites

Choose **Static website** for plain HTML, CSS and JavaScript files. There is no build or run
command.

- **Publish folder:** the folder that contains your `index.html`. Use `.` when it is at the root
  of the repository (or of the working directory), or a folder such as `public` or `site`.

A project that serves its files with `python -m http.server` or a similar command is a static
website: choose **Static website** rather than Python. It is faster and uses less memory.

## Single-page apps (Vite, React, Astro)

These projects are built into static files. FodyCloud runs the build command, then serves the
output folder (`dist` by default). If your build writes somewhere else, change the **Publish
folder**.

For single-page apps, every address of the site (for example `/books`) opens your app, so the
browser's routing works.

### Blank page after deploying

If the page stays blank and the browser shows errors for files under `/assets/`, your build is
probably configured for a sub-path. This is common for projects first published on GitHub Pages,
whose `vite.config.js` contains `base: '/my-repo/'`.

FodyCloud serves your app at the root of its own address, so the base must be `/`. Either:

- change `base` to `'/'` in `vite.config.js`, or
- keep your file and change the **Build command** to `npm run build -- --base=/`.

## PHP apps

PHP apps are served by a PHP web server (FrankenPHP): there is no run command.

- **Plain PHP, Slim, Symfony:** set the **Publish folder** to the folder that contains your
  `index.php`, for example `public` or `web`, or `.` when it is at the root. Every address that
  isn't a file goes to that `index.php`, so front-controller frameworks work.
- **Laravel:**
  1. Generate a key on your computer with `php artisan key:generate --show`.
  2. Add it as the environment variable `APP_KEY` (see
     [Environment variables](environment-variables.md)).
  3. Deploy. The database migrations run each time the app starts.

  Without a database server, Laravel uses a SQLite file inside the app. **It is reset on each
  deployment**, like every file your app writes.

## Go apps

The **Go server** framework builds your module with `go build` into `./out` and runs it. Your
server must listen on the port in the `PORT` environment variable, for example:

```go
http.ListenAndServe(":"+os.Getenv("PORT"), nil)
```

When your `main` package is in a subfolder such as `cmd/server`, choose **Other (custom
commands)** with the build command `go build -o out ./cmd/server` and the run command `./out`.

## Java apps

Java apps are built with Maven or Gradle (the wrapper `mvnw` or `gradlew` when your repository
has one), then started with `java -jar` on the jar in `target/` (Maven) or `build/libs/`
(Gradle). For a build with several modules, choose **Other (custom commands)** and give the run
command, for example `java $JAVA_OPTS -jar server/build/libs/server.jar`.

The Java version comes from your build: `<java.version>` in `pom.xml`, or the toolchain's
`languageVersion` in `build.gradle`. Choose it in **Version** if the analysis didn't find it.

- **Spring Boot:** the port is set for you.
- **Java application (JAR):** your app must listen on the port in the `PORT` environment
  variable.

Java uses more memory than other languages: a Spring Boot app needs about 300 MB, more than the
free plan's 256 MB (see [Plans and limits](plans-and-limits.md)). The memory of the Java
virtual machine follows your plan's limit; to tune it, set the environment variable
`JAVA_OPTS`, for example `-XX:MaxRAMPercentage=60.0`.

## Bun

Choose **Node.js** → **Bun server** to run your project with Bun instead of Node.js. It runs
the `start` script of `package.json` with Bun; `Bun.serve` listens on `$PORT` by default.

## Apps in a subfolder

When your app isn't at the root of the repository (for example a `frontend/` folder next to a
`docs/` folder, or several apps in one repository), set the **Working directory** to that
folder, for example `frontend` or `apps/web`.

- The build and run commands execute in that folder.
- The publish folder of a static site is relative to it.
- Leave it empty for the root of the repository.

If nothing is found at the root, the analysis also looks in subfolders and suggests the one it
found, for example: *"Nothing to build at the repository root, but the folder `frontend/` looks
like Node.js · Vite / React single-page app."*

To analyse a specific folder before you confirm, type it next to **Analyse again** on the review
card.

## When your framework isn't listed

Choose your language, then **Other (custom commands)**, and give:

- a **build command** (optional), for steps after the dependencies are installed, such as
  `npm run build` or `python manage.py collectstatic --noinput`;
- a **run command** (required) that starts your server on `$PORT`, such as
  `python server.py` or `node dist/server.js`.

Other languages (Ruby, Rust, .NET…) aren't supported yet.

## Related

- [Environment variables](environment-variables.md): settings and secrets for the build and the
  app.
- [Logs and troubleshooting](logs-and-troubleshooting.md): what to do when a build fails.
