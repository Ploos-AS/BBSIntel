# M1.12 — Long-running Public Service Architecture

## Goal

Make BBSIntel suitable as the foundation for a long-running public web service whose probe history and statistics grow continuously over years.

## Service roles

Production Compose now runs two roles from the same image:

- `bbsintel-web`: HTTP/API only, with the embedded scheduler disabled.
- `bbsintel-worker`: scheduler/import/probe/rollup work only.

This prevents public HTTP work from sharing the same application process with network probes and directory imports. Both roles continue to share the SQLite database on the same host. SQLite remains the supported v0.1.0 storage backend; multi-host or horizontally scaled deployments should use a future PostgreSQL backend rather than sharing a SQLite file over network storage.

The historical single-process mode remains available outside Compose by running the binary normally with `BBSINTEL_SCHEDULER_ENABLED=true`.

## Statistics storage hierarchy

Raw `probe_result` rows are the high-resolution source of truth, but public long-term statistics must not depend on scanning the full raw table.

M1.12 adds:

- `endpoint_hourly`: per-endpoint hourly probe counters and latency sums.
- `endpoint_daily`: per-endpoint daily counters derived from hourly data.

Each bucket stores checks for `online`, `telnet_only`, `tcp_only`, `offline`, and `dns_fail`, plus connect-latency sum/count.

`rollup.Refresh` has bounded steady-state work:

1. On the first run it backfills existing raw history into hourly rows and derives daily rows.
2. Later runs rebuild only the most recent 48 hours of hourly data.
3. Later daily refreshes rebuild only the most recent 35 days.
4. Older daily rows remain immutable and can therefore be retained indefinitely.

The scheduler refreshes rollups after probe passes.

## Public statistics API

`GET /api/v1/statistics/daily?days=30`

- `days` defaults to 30 and is capped at 3650.
- Reads `endpoint_daily`, not `probe_result`.
- Returns daily check totals, strict `online_pct`, broader connectivity percentage (`online + telnet_only + tcp_only`), failures, and average connect latency.
- Sends `Cache-Control: public, max-age=60` so a reverse proxy/CDN can absorb repeated public statistics traffic.

This endpoint establishes the rule that public historical dashboards should read aggregate tables instead of rescanning raw probe history.

## Retention and maintenance

Raw retention is explicit and operator-controlled. It is not silently scheduled by the worker.

Available commands:

```sh
bbsintel maintenance rollup
bbsintel maintenance prune
bbsintel maintenance all
```

- `rollup` refreshes hourly/daily aggregates without deleting raw data.
- `prune` deletes raw `probe_result` rows older than `BBSINTEL_RAW_RETENTION`.
- `all` refreshes rollups first and only then prunes raw data.
- `BBSINTEL_RAW_RETENTION` defaults to `2160h` (90 days) when prune/all is explicitly invoked.

This ordering ensures that long-term daily history survives raw-data pruning.

## Liveness and readiness

- `GET /healthz` remains a cheap process-liveness check.
- `GET /readyz` verifies that the SQLite database is reachable and can execute a query. Database failure returns HTTP 503.

This keeps orchestration semantics clear: a live process is not necessarily ready to serve database-backed public requests.

## HTTP hardening

The public HTTP server now has explicit resource/time bounds. Defaults are configurable through environment variables:

- `BBSINTEL_HTTP_READ_HEADER_TIMEOUT=5s`
- `BBSINTEL_HTTP_READ_TIMEOUT=15s`
- `BBSINTEL_HTTP_WRITE_TIMEOUT=30s`
- `BBSINTEL_HTTP_IDLE_TIMEOUT=60s`
- `BBSINTEL_HTTP_MAX_HEADER_BYTES=1048576`

These are application-level safety limits. A public deployment should still use a reverse proxy or ingress for TLS, connection/rate limiting, request logging, and edge caching.

## SQLite concurrency

Each BBSIntel process still uses a single database connection. In the production Compose topology this becomes one connection for the web process and one for the worker process, with WAL and busy timeout enabled. This is materially better than the previous single-process topology and preserves the assumptions already present in ingest/probe code.

Increasing the per-process connection pool is intentionally deferred until all PRAGMA behavior and write paths are made connection-safe. Public horizontal scale or multiple hosts is a PostgreSQL milestone, not a reason to weaken SQLite correctness.

## Remaining public-service hardening

Before calling the service fully production-hardened, subsequent work should add:

- maintenance scheduling/operations guidance for retention;
- rate limiting / reverse-proxy deployment guidance;
- Prometheus/OpenMetrics metrics;
- additional materialized public statistics (inventory, software, protocol, source, country, new/disappeared systems);
- pagination on the main BBS inventory endpoint;
- optimized BBS analytics using rollups for long windows;
- backup/restore and database integrity operational documentation;
- a PostgreSQL storage backend before multi-host horizontal scaling.

## Release impact

M1.12 changes the v0.1.0 release candidate architecture. The `v0.1.0` tag must not point to the earlier M1.11/M1.12 README-only release candidate. Release qualification must be rerun on the final long-running-service HEAD before tagging.
