# Environment variables

Environment variables give your app settings and secrets, such as a database address or an API
key, without writing them in your code. This page shows how to add, change and remove them.

## What they are for

- **Secrets:** passwords, API keys, tokens. Never commit them to your repository: since your
  repository is public, anyone could read them.
- **Settings that differ between your computer and FodyCloud:** `DEBUG=false`, the address of
  another service.

Your app reads them like any environment variable: `process.env.API_KEY` in Node.js,
`os.environ["API_KEY"]` in Python.

They are available **while your app is built** (for example to a build command that needs a
key) **and while it runs**. They are never written into the built app itself.

## Add them when you create the app

In the **New app** form, type one variable per line in **Environment variables**:

```
API_KEY=sk-1234
SECRET_KEY=change-me
DEBUG=false
```

Blank lines and lines starting with `#` are ignored. Quotes around a value are removed.

## Add or change a variable

1. Open your app's page and find the **Environment variables** card.
2. Type the name and the value, then select **Save**.

To change a value, save the same name with the new value. **Saving restarts a running app**, so
it picks up the new value within a few seconds.

Values are masked in the dashboard after you save them: only their first two characters show.
To be sure of a value, save it again.

## Remove a variable

In the **Environment variables** card, select **Delete** next to the variable. The app restarts
without it.

## Rules

| Rule | Detail |
|---|---|
| Names | Letters, digits and `_`, not starting with a digit. Example: `API_KEY_2`. |
| Number | At most 100 variables per app. |
| Size | At most 32 KB per value. |
| Reserved | `PORT` is set by FodyCloud. So are the variables of your app's [storage and databases](databases.md): `DATABASE_URL`, `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE`, `REDIS_URL` and `FODYCLOUD_DATA_DIR`. Names starting with `RAILPACK_` are reserved for the build; use the stack settings instead. |

## Good practices

- Use a different secret on FodyCloud than on your computer, and change it if it was ever
  committed to a repository.
- Prefer long random values for secrets, for example from a password manager.
- Delete variables you no longer use.

## Related

- [Deploy your own app](deploy-an-app.md)
- [Account and security](account-and-security.md)
