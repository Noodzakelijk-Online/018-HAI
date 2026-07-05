# Backup & Restore Procedures

018-HAI's state of record is Postgres. Media/uploads live under the configured
`IMAGE_SAVE_DIR`. Backing up both gives a complete, restorable snapshot.

## What to back up

| Asset | Source | Method |
| --- | --- | --- |
| Database | Postgres (`DB_NAME`) | `pg_dump` logical backup |
| Uploaded media | `IMAGE_SAVE_DIR` | file archive (tar) |
| Configuration | `.env*` (secrets — store securely, never in git) | secure secret store |

## Database backup

```bash
# Logical, compressed, restorable dump.
pg_dump --host "$DB_HOST" --port "$DB_PORT" --username "$DB_USER" \
        --format=custom --file "hai-$(date +%F).dump" "$DB_NAME"
```

Store dumps off-host. Keep a rolling window aligned with the retention policy
(`internal/retention`).

## Media backup

```bash
tar -czf "hai-media-$(date +%F).tgz" -C "$IMAGE_SAVE_DIR" .
```

## Restore (into a clean database)

```bash
createdb --host "$DB_HOST" --port "$DB_PORT" --username "$DB_USER" "$DB_NAME"
pg_restore --host "$DB_HOST" --port "$DB_PORT" --username "$DB_USER" \
           --dbname "$DB_NAME" --clean --if-exists "hai-YYYY-MM-DD.dump"
tar -xzf "hai-media-YYYY-MM-DD.tgz" -C "$IMAGE_SAVE_DIR"
```

## Verify a restore

1. Start the backend against the restored database.
2. `GET /readyz` must return ready (200).
3. Run `backend reconcile` — it scans memories for broken invariants and reports
   any records needing attention after the restore.
4. Spot-check a known memory and a workflow item.

## Cadence

- Automate a daily DB dump + weekly media archive.
- Test a restore into a scratch database at least monthly — an untested backup
  is not a backup.
