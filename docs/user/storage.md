# Keep your app's files

Files your app writes are normally lost each time it is deployed or restarts. A **persistent
volume** keeps them: everything your app writes in the `/data` folder stays across deployments,
restarts, sleep and stops. This page shows how to add one and use it, for example for a SQLite
database or uploaded files.

**You need:** a plan that includes a volume (the free plan doesn't; see
[Plans and limits](plans-and-limits.md)).

## Add a volume

1. Open your app's page and find the **Storage and databases** card.
2. Next to **Persistent volume**, select **Add**.

The dashboard shows **Persistent volume added. The app restarts to use it.** After the restart,
the card shows the folder, `/data`, and how much of the volume is used.

## Use it in your app

Write the files you want to keep under `/data`. FodyCloud also sets the environment variable
`FODYCLOUD_DATA_DIR=/data`, so your code can use it rather than a fixed path:

```python
import os, sqlite3
db = sqlite3.connect(os.path.join(os.environ.get("FODYCLOUD_DATA_DIR", "."), "app.db"))
```

```javascript
const dir = process.env.FODYCLOUD_DATA_DIR ?? ".";
fs.writeFileSync(`${dir}/uploads/photo.jpg`, data);
```

Everything outside `/data` is still reset on each deployment.

## Limits

- **Size:** your plan sets it, for example 1 GB on the student plan. When the volume is full,
  writing a file fails with "No space left on device". Delete files you no longer need.
- **One volume per app**, always at `/data`.
- **Deployments:** for a few seconds during a deployment, the old and the new version of your
  app both run and share the volume. SQLite handles this with its own locking.
- **Smaller plan:** a volume keeps its size if your plan changes; a new volume gets the new
  plan's size.

## Backups

Volumes are copied every night to an encrypted off-site backup, kept up to 6 months. Restoring
one is done by the FodyCloud team: contact them with your app's name and the date you need.
Keep your own copy of anything important.

## Delete the volume

Deleting the volume deletes **all its files, for good**.

1. In the **Storage and databases** card, type your app's name next to **Persistent volume**.
2. Select **Delete with its data**.

The app restarts without the volume. Deleting the app also deletes its volume.

## Related

- [Databases for your app](databases.md)
- [Plans and limits](plans-and-limits.md)
