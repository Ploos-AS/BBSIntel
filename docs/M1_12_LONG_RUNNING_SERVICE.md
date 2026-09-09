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

## Retention

`rollup.PruneRaw` is implemented but is deliberately not scheduled automatically in M1.12. It removes `probe_result` rows older than a supplied retention duration and is safe to run after rollups have been refreshed.

A production retention value must be an explicit operator policy. The intended initial policy is approximately 90 days raw, with hourly/daily aggregates retained much longer, but no automatic deletion is enabled by this milestone.

## SQLite concurrency

Each BBSIntel process still uses a single database connection. In the production Compose topology this becomes one connection for the web process and one for the worker process, with WAL and busy timeout enabled. This is materially better than the previous single-process topology and preserves the assumptions already present in ingest/probe code.

Increasing the per-process connection pool is intentionally deferred until all PRAGMA behavior and write paths are made connection-safe. Public horizontal scale or multiple hosts is a PostgreSQL milestone, not a reason to weaken SQLite correctness.

## Remaining public-service hardening

Before calling the service fully production-hardened, subsequent milestones should add:

- explicit raw-retention command/configuration and maintenance scheduling;
- readiness separate from liveness;
- HTTP read/write/idle timeouts and maximum-header sizing;
- rate limiting / reverse-proxy deployment guidance;
- Prometheus/OpenMetrics metrics;
- additional materialized public statistics (inventory, software, protocol, source, country, new/disappeared systems);
- pagination on the main BBS inventory endpoint;
- optimized BBS analytics using rollups for long windows;
- backup/restore and database integrity operational documentation;
- a PostgreSQL storage backend before multi-host horizontal scaling.

## Release impact

M1.12 changes the v0.1.0 release candidate architecture. The `v0.1.0` tag must not point to the earlier M1.11/M1.12 README-only release candidate. Release qualification must be rerun on the final long-running-service HEAD before tagging.
