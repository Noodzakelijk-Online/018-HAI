# Backup & Restore Procedures

018-HAI's state of record spans two Postgres databases: the automation ledger
and the identity provider. Media/uploads live under the configured
`IMAGE_SAVE_DIR`. A recovery set is incomplete unless it contains all three.

## What to back up

| Asset | Source | Method |
| --- | --- | --- |
| Automation ledger | Postgres (`AUTOMATION_DB_NAME`) | `pg_dump` logical backup |
| Accounts and authentication | Postgres (`IDP_DB_NAME`) | `pg_dump` logical backup |
| Uploaded media | `IMAGE_SAVE_DIR` | file archive (tar) |
| Configuration | `.env*` (secrets — store securely, never in git) | secure secret store |

## Database backup

On Windows 11, use the repository command. It validates Compose, briefly stops
the backend and IDP to prevent cross-database changes, dumps both databases,
archives `./images`, writes SHA-256 hashes and a non-secret manifest, removes
temporary container files, and restarts only services that were running:

```powershell
.\scripts\backup-windows.ps1 -ValidateOnly
.\scripts\backup-windows.ps1
```

Generated bundles are ignored under `./backups`. Move the completed bundle to
encrypted off-host storage. The script deliberately does not copy `.env.local`;
store that secret material separately in an approved credential vault.

Prove the bundle can restore without touching either live database:

```powershell
.\scripts\test-restore-windows.ps1 -BackupDirectory .\backups\hai-backup-YYYYMMDDTHHMMSSZ -ValidateOnly
.\scripts\test-restore-windows.ps1 -BackupDirectory .\backups\hai-backup-YYYYMMDDTHHMMSSZ
```

The drill verifies all manifest hashes and the media ZIP, restores both dumps
into randomly suffixed scratch databases, requires real public tables in each,
and drops only those scratch databases in `finally`.

For non-Windows operators, dump **both** databases:

```bash
# Logical, compressed, restorable dump.
pg_dump --host "$AUTOMATION_DB_HOST" --port "$DB_PORT" --username "$DB_USER" \
        --format=custom --file "hai-automation-$(date +%F).dump" "$AUTOMATION_DB_NAME"
pg_dump --host "$IDP_DB_HOST" --port "$DB_PORT" --username "$DB_USER" \
        --format=custom --file "hai-identity-$(date +%F).dump" "$IDP_DB_NAME"
```

Store dumps off-host. Keep a rolling window aligned with the retention policy
(`internal/retention`).

## Media backup

```bash
tar -czf "hai-media-$(date +%F).tgz" -C "$IMAGE_SAVE_DIR" .
```

## Restore (into a clean database)

```bash
createdb --host "$AUTOMATION_DB_HOST" --port "$DB_PORT" --username "$DB_USER" "$AUTOMATION_DB_NAME"
pg_restore --host "$AUTOMATION_DB_HOST" --port "$DB_PORT" --username "$DB_USER" \
           --dbname "$AUTOMATION_DB_NAME" --clean --if-exists "hai-automation-YYYY-MM-DD.dump"
createdb --host "$IDP_DB_HOST" --port "$DB_PORT" --username "$DB_USER" "$IDP_DB_NAME"
pg_restore --host "$IDP_DB_HOST" --port "$DB_PORT" --username "$DB_USER" \
           --dbname "$IDP_DB_NAME" --clean --if-exists "hai-identity-YYYY-MM-DD.dump"
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
