# Backup, integrity checks, and restore

BBSIntel v0.1.0 uses SQLite on local storage. The database is operational state and historical data, so a public installation should have tested backups before release.

## Integrity checks

A lightweight online check can run while web and worker are active:

```sh
docker compose run --rm bbsintel-web maintenance check
```

This runs SQLite `PRAGMA quick_check`.

For a deeper check:

```sh
docker compose run --rm bbsintel-web maintenance check full
```

This runs `PRAGMA integrity_check` and is more expensive. It is suitable for periodic maintenance and before/after recovery work.

## Online backup

`maintenance backup` uses SQLite `VACUUM INTO`, producing a transactionally consistent standalone database while the normal web and worker processes remain online. The resulting file is immediately validated with a full integrity check before success is reported.

Without an explicit destination, backups are written below the database directory:

```sh
docker compose run --rm bbsintel-web maintenance backup
# /data/backups/bbsintel-YYYYMMDDTHHMMSSZ.db
```

A backup stored only in the same Docker volume does not protect against host or volume loss. For production, write or copy backups to separate storage. One simple host-side pattern is:

```sh
mkdir -p backups
STAMP=$(date -u +%Y%m%dT%H%M%SZ)
docker compose run --rm \
  -v "$PWD/backups:/backup" \
  bbsintel-web maintenance backup "/backup/bbsintel-${STAMP}.db"
```

The destination is deliberately create-only. An existing backup file is never overwritten.

After backup, move/copy the file to storage outside the BBSIntel host according to your normal retention policy.

## Restore

Restore is intentionally an offline operation. Stop both processes that can hold the SQLite database open:

```sh
docker compose stop bbsintel-web bbsintel-worker
```

Then run restore from a one-off container. For a host-side backup directory:

```sh
docker compose run --rm \
  -v "$PWD/backups:/backup:ro" \
  bbsintel-web maintenance restore /backup/bbsintel-YYYYMMDDTHHMMSSZ.db
```

The restore command runs before BBSIntel opens the destination database. It performs a full integrity check on the backup first. If the current database exists, it is preserved as:

```text
/data/bbsintel.db.pre-restore-YYYYMMDDTHHMMSSZ
```

The validated backup is then copied into place and stale `-wal` / `-shm` sidecars are removed. Start the services again:

```sh
docker compose up -d bbsintel-web bbsintel-worker caddy
```

Verify recovery:

```sh
docker compose run --rm bbsintel-web maintenance check full
curl -fsS https://YOUR_DOMAIN/readyz
```

Do not run restore while another BBSIntel process has `/data/bbsintel.db` open. SQLite remains a single-host design here; do not place the database or its live WAL files on NFS/network storage.

## Suggested policy

For an initial public deployment:

- online backup at least daily;
- retain multiple daily and weekly recovery points;
- copy backups off-host;
- run `quick_check` routinely and `integrity_check` periodically;
- perform an actual restore drill after deployment changes and before relying on backups operationally.

Retention scheduling is intentionally left to the host/orchestrator for v0.1.0 rather than adding another scheduler inside BBSIntel. A future deployment layer can provide a dedicated backup sidecar or host timer without changing the database format.
