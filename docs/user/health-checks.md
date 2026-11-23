# Check your app's health before it goes live

A health check makes sure a new version of your app really works before it replaces the
current one. You choose a page of your app, such as `/healthz`. On each deployment, FodyCloud
asks the new version for that page, and switches visitors to it only when the page answers
correctly.

**You need:**

- an app on FodyCloud;
- a page in your app that answers quickly when the app is ready, for example `/healthz`
  returning `ok`. The home page `/` works too if it is light.

## Turn on the health check

1. Open your app's page and find the **Health check** card.
2. Type the path, for example `/healthz`, and select **Save**.

The dashboard shows **Health check saved: /healthz. It applies from the next deployment.**

3. Select **Deploy latest** (or **Restart**) to use it now.

## What happens on each deployment

1. The new version starts next to the current one.
2. FodyCloud asks the new version for the path every 2 seconds, for up to 60 seconds. The build
   log shows each attempt:

   ```text
   ==> Checking http://my-app.apps.fodycloud.example/healthz (up to 60s)
       GET /healthz → no HTTP answer (is the app listening on $PORT?)
   ==> Health check passed (200) after 4s
   ```

3. When the page answers with a status from 200 to 399, the new version goes live and the old
   one is removed.
4. If it never does, the deployment fails with
   **the app didn't answer /healthz with a 2xx or 3xx status within 60s (last: 503)**, and
   **the previous version keeps running**. Your visitors don't see the broken version.

Redirects count as an answer but aren't followed: a page that redirects to `/login` passes.

Without a health check, FodyCloud only checks that your app keeps running for 8 seconds after
it starts.

## Write a good health page

- Answer `200` only when the app can serve visitors, for example after connecting to its
  database.
- Keep it fast and light: no heavy queries.
- Don't protect it with a password.

Example with Flask:

```python
@app.get("/healthz")
def healthz():
    return "ok"
```

Example with Express:

```javascript
app.get("/healthz", (req, res) => res.send("ok"));
```

## Turn off the health check

Empty the path in the **Health check** card and select **Save**. It applies from the next
deployment.

## Troubleshooting

| Message | Cause and fix |
|---|---|
| the health-check path must start with / and contain only URL characters (no spaces), for example /healthz | Type only the path, starting with `/`, without `https://` or the domain. |
| the app didn't answer /healthz with a 2xx or 3xx status within 60s (last: 404) | The path doesn't exist in your app. Check its spelling, or add the page. |
| the app didn't answer /healthz with a 2xx or 3xx status within 60s (last: no HTTP answer (is the app listening on $PORT?)) | The app doesn't accept connections: it listens on another port, or only on `localhost`, or takes more than 60 seconds to start. Listen on `0.0.0.0` and the port in `$PORT`. |

## Related

- [Logs and troubleshooting](logs-and-troubleshooting.md)
- [Deploy your own app](deploy-an-app.md)
