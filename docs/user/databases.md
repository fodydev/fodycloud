# Databases for your app

FodyCloud can give your app a **PostgreSQL database** and a **Valkey cache** (compatible with
Redis). This page shows how to add them, connect your app, and what happens when the database is
full.

**You need:** an app on FodyCloud. Every plan includes a database and a cache; their sizes
depend on your plan (see [Plans and limits](plans-and-limits.md)).

## Add a PostgreSQL database

1. Open your app's page and find the **Storage and databases** card.
2. Next to **PostgreSQL database**, select **Add**.

The dashboard shows **PostgreSQL database added. The app restarts to use it.** Your app now has
these environment variables, at build time and while it runs:

| Variable | Example |
|---|---|
| `DATABASE_URL` | `postgresql://app_12:…@172.30.0.3:5432/app_12` |
| `PGHOST`, `PGPORT`, `PGUSER`, `PGPASSWORD`, `PGDATABASE` | the same details, one per variable |

Select **Show the connection URL** in the card to see the full address with its password.

Your app is the only one that can use this database. It isn't reachable from the internet, so
you can't connect to it from your computer: create your tables with your framework's migrations.

### Connect your framework

- **Django:** install `dj-database-url` and `psycopg`, then in `settings.py`:

  ```python
  import dj_database_url
  DATABASES = {"default": dj_database_url.config()}
  ```

  The default Django run command already runs `python manage.py migrate` at each start.
- **Flask / FastAPI with SQLAlchemy:** use `os.environ["DATABASE_URL"]` as the engine URL
  (replace `postgresql://` with `postgresql+psycopg://` if you use psycopg 3).
- **Node.js:** `new pg.Pool({ connectionString: process.env.DATABASE_URL })`, or Prisma with
  `url = env("DATABASE_URL")`.
- **Laravel:** add the environment variables `DB_CONNECTION=pgsql` and `DB_URL` with the same
  value as `DATABASE_URL` (copy it from the card). Migrations run at each start.
- **Spring Boot:** add `SPRING_DATASOURCE_URL=jdbc:postgresql://<PGHOST>:5432/<PGDATABASE>`,
  `SPRING_DATASOURCE_USERNAME` and `SPRING_DATASOURCE_PASSWORD` with the values from the card.

### Limits

- **Size:** your plan sets it, for example 1 GB on the student plan. The card shows the size,
  measured every 10 minutes.
- **Connections:** your plan sets how many connections your app can open at once, for example 20.
  Keep your connection pool below that number.

### When the database is full

When the database grows past your plan's size, it becomes **read-only**: your app can still read
its data, but changing it fails with "cannot execute INSERT in a read-only transaction". You
receive an email titled **The database of my-app is full**.

To make room:

1. In your app (for example in a one-off command or an admin page), open a connection and run
   `SET default_transaction_read_only = off;`.
2. Delete the data you no longer need with `DELETE`.
3. Run `VACUUM FULL;` to give the space back: deleted rows keep their space until then.

Within 10 minutes of the database fitting its size again, writes are allowed again and you
receive an email.

A database that keeps growing past its size is **blocked**: your app's connections are refused.
Contact the FodyCloud team.

## Add a Valkey cache

1. In the **Storage and databases** card, next to **Valkey cache**, select **Add**.
2. Your app gets the variable `REDIS_URL`, for example `redis://default:…@valkey:6379/0`.

Use it with any Redis client: `redis.from_url(os.environ["REDIS_URL"])` in Python,
`createClient({ url: process.env.REDIS_URL })` in Node.js, or `REDIS_URL` in Laravel.

The cache keeps its data **in memory only**: it is emptied when it restarts, so don't store
anything you can't rebuild. When it is full, the least recently used keys are removed to make
room.

## Backups

Databases are copied every night to an encrypted off-site backup, kept up to 6 months. Restoring
one is done by the FodyCloud team: contact them with your app's name and the date you need.
Caches aren't backed up.

## Delete a database or a cache

Deleting deletes **all its data, for good**.

1. In the **Storage and databases** card, type your app's name next to the database or cache.
2. Select **Delete with its data**.

The app restarts without it. Deleting the app also deletes its database and cache.

## Troubleshooting

| Message | Cause and fix |
|---|---|
| the free plan doesn't include a persistent volume | Your plan doesn't include it. Ask the FodyCloud team for a plan that does. |
| this app already has a PostgreSQL database | Each app has at most one database, one cache and one volume. |
| cannot execute INSERT in a read-only transaction | The database is full. See [When the database is full](#when-the-database-is-full). |
| too many connections for role "app_12" | Your app opens more connections than your plan allows. Make its connection pool smaller. |
| DATABASE_URL is set by FodyCloud for the app's storage and databases | You tried to set a variable FodyCloud manages. Remove it from your environment variables. |

## Related

- [Keep your app's files](storage.md)
- [Environment variables](environment-variables.md)
