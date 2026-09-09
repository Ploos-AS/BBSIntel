# BBSIntel

BBSIntel is a self-hosted BBS discovery, verification, monitoring, and intelligence service.

It aggregates public BBS directories, normalizes and deduplicates entries, probes endpoints, stores availability history, and exposes the resulting data through an HTTP API.

## v0.1.0 scope

- Go service
- SQLite persistence
- source-adapter model
- Telnet BBS Guide and Synchronet directory sources
- cross-source endpoint deduplication
- Telnet and SSH endpoint probing
- Telnet banner cleanup and software fingerprinting
- full per-source metadata provenance
- source disagreement intelligence
- source health and change events
- derived alerts with filtering, statistics, `since`, and cursor pagination
- reported vs observed software provenance
- status history and rollups
- materialized public statistics
- built-in scheduler with separate worker mode
- adaptive probe backoff
- REST API with bounded cursor pagination
- Prometheus-compatible metrics
- SQLite backup, restore, integrity, and retention maintenance
- Caddy-fronted production Compose deployment
- OCI image
- GitHub Actions CI, live qualification, and release workflow

## Directory sources

The built-in scheduler currently imports:

- Telnet BBS Guide — advertised Telnet and SSH endpoints
- Synchronet official BBS list — advertised Telnet and SSH endpoints from `synchro.net/sbbslist.html`

Each source keeps its own `source_entry` provenance. When two directories advertise the same protocol/hostname/port, BBSIntel reuses the existing BBS identity instead of moving the endpoint or creating an orphaned duplicate. Hostname matching is case-insensitive. Multiple terminal protocols from one source row may share one source key and attach to the same BBS.

Directory imports are independent: one source failing does not prevent the other configured sources from being attempted.

## Source provenance

Canonical BBS metadata is kept separately from source-specific claims. Each `source_entry` stores:

- source and source key
- source URL
- reported name
- reported software
- reported country
- reported description
- last-seen timestamp
- active/missing state

Canonical values on the BBS record are filled conservatively and are not overwritten simply because another source imports later. All source claims remain queryable through `GET /api/v1/bbs/{id}/sources`.

This makes disagreements explicit. For example, one directory may report Mystic while another reports Synchronet, and live probing may independently observe Synchronet. BBSIntel preserves all three pieces of evidence.

## Source intelligence

BBSIntel derives an intelligence summary from provenance instead of storing a second competing truth. `GET /api/v1/bbs/{id}/intelligence` exposes:

- `source_count`
- `name_conflict`, `software_conflict`, `country_conflict`, and `description_conflict`
- `conflict_count`
- normalized distinct names, reported software values, and countries
- `source_agreement_pct` across non-empty name/software/country claims
- latest observed software and fingerprint confidence
- whether the observed software matches at least one directory source

The global `GET /api/v1/intelligence/stats` endpoint reports counts of multi-source BBS identities and conflicts by metadata field.

`source_agreement_pct` is an agreement metric, not a claim that a particular source is correct. Live observations remain separate evidence.

## Status model

- `online` — connection succeeded and application/banner data was observed
- `telnet_only` — a Telnet endpoint responded with negotiation bytes but no cleaned application banner was observed
- `tcp_only` — TCP connection succeeded but no application data was observed
- `offline` — connection failed or timed out
- `dns_fail` — hostname could not be resolved

`online`, `telnet_only`, and `tcp_only` are treated as healthy connectivity states for probe cadence. Probing is intentionally passive. Telnet probes connect and read a bounded banner without logging in or replying to Telnet negotiation. SSH probes read a bounded server identification stream and disconnect without authentication or key exchange.

## Run locally

```sh
go run ./cmd/bbsintel
```

The default server mode starts both the HTTP API and the scheduler. On startup the scheduler performs directory imports followed by a probe pass, then repeats imports every 24 hours and considers endpoints for probing every 30 minutes.

Defaults: listen on `:8080`, database at `./data/bbsintel.db`.

Environment variables include:

- `BBSINTEL_LISTEN`
- `BBSINTEL_DB`
- `BBSINTEL_SCHEDULER_ENABLED` (default `true`)
- `BBSINTEL_IMPORT_INTERVAL` (default `24h`)
- `BBSINTEL_PROBE_INTERVAL` (default `30m`)
- `BBSINTEL_PROBE_CONCURRENCY` (default `8`)
- `BBSINTEL_RAW_RETENTION` (maintenance default `90d`)
- `BBSINTEL_HTTP_MAX_INFLIGHT` (default `64`)

## Adaptive probe cadence

Healthy endpoints (`online`, `telnet_only`, or `tcp_only`) use the configured probe interval. Repeated `offline` or `dns_fail` results progressively reduce probe frequency:

- first failure: normal interval
- second consecutive failure: at least `1h`
- third consecutive failure: at least `6h`
- fourth or later consecutive failure: at least `24h`

Backoff never shortens a custom base probe interval. A healthy result immediately resets the endpoint to the normal cadence.

## Software provenance

Directory metadata remains separate from live observations:

- `reported_software` — canonical software metadata supplied by directory import
- `observed_software` — software inferred from the latest live banner
- `software_confidence` — confidence score for the observed fingerprint
- `software_evidence` — banner token that produced the match
- `software_mismatch` — true when reported and observed software disagree

Observed values never overwrite source-reported values. Per-source software claims are available through the source provenance API.

## Uptime and public statistics

BBSIntel calculates sample-based uptime from stored probe results. `uptime_pct` is the percentage of completed probes in the requested window whose status was `online`; it is not presented as continuous time-weighted monitoring between probes.

Raw probe history is summarized into hourly and daily rollups. Public inventory and runtime statistics are materialized so read-heavy public API traffic does not repeatedly scan the raw probe table. Public dimensions currently cover software, protocol, country, and source, plus BBS lifecycle statistics.

## One-shot and maintenance commands

Import directory data:

```sh
go run ./cmd/bbsintel import telnetbbsguide
go run ./cmd/bbsintel import synchronet
```

Probe endpoints currently due:

```sh
go run ./cmd/bbsintel probe
```

Run only the scheduler without HTTP:

```sh
go run ./cmd/bbsintel scheduler
```

Refresh rollups/materialized statistics and prune retained raw probes:

```sh
go run ./cmd/bbsintel maintenance rollup
go run ./cmd/bbsintel maintenance prune
go run ./cmd/bbsintel maintenance all
```

Check and back up SQLite:

```sh
go run ./cmd/bbsintel maintenance check
go run ./cmd/bbsintel maintenance check full
go run ./cmd/bbsintel maintenance backup ./backups/bbsintel.db
```

Restore is an offline operation; stop web/worker first:

```sh
go run ./cmd/bbsintel maintenance restore ./backups/bbsintel.db
```

See `docs/BACKUP_RESTORE.md` for the production recovery procedure.

Show build/release information:

```sh
go run ./cmd/bbsintel --version
# or
go run ./cmd/bbsintel version
```

## API

- `GET /healthz` — process liveness
- `GET /readyz` — database-backed readiness
- `GET /metrics` — Prometheus-compatible operational metrics; keep private in public deployments
- `GET /api/v1/version` — version, commit, and build date
- `GET /api/v1/bbs` — bounded cursor-paginated BBS inventory with search/filter support
- `GET /api/v1/bbs/{id}` — BBS details and latest status/fingerprint for each endpoint
- `GET /api/v1/bbs/{id}/sources` — source-specific metadata and presence state
- `GET /api/v1/bbs/{id}/intelligence` — source disagreement summary and comparison with observed software
- `GET /api/v1/intelligence/stats` — aggregate counts of multi-source identities and metadata conflicts
- `GET /api/v1/bbs/{id}/analytics` — first/last observations, status changes, and 24h/7d/30d sample-based uptime
- `GET /api/v1/bbs/{id}/history?limit=200` — newest probe history across the BBS endpoints; limit is capped at 1000
- `GET /api/v1/events` and `GET /api/v1/bbs/{id}/events` — change feeds with `since` and opaque cursor pagination
- `GET /api/v1/alerts` — deduplicated current alerts with severity/category/source/BBS filters, `since`, and cursor pagination
- `GET /api/v1/alerts/stats` — alert totals grouped by severity, category, and source
- `GET /api/v1/sources/health` — import telemetry, freshness classification, and persisted scheduler-observed state
- `GET /api/v1/stats` — materialized inventory/runtime status snapshot
- `GET /api/v1/statistics/daily?days=N` — daily probe rollups
- `GET /api/v1/statistics/dimensions/{software|protocol|country|source}` — public inventory dimensions
- `GET /api/v1/statistics/lifecycle?days=N` — new/disappeared/returned BBS statistics

## Production Compose with Caddy

BBSIntel is intended to run behind a reverse proxy for public deployments. The included Compose stack uses Caddy as the only public service and keeps `bbsintel-web:8080` internal.

```sh
cp .env.example .env
# edit BBSINTEL_DOMAIN, for example bbs.example.org
docker compose up -d --build
```

Caddy publishes ports 80 and 443, provides automatic TLS, zstd/gzip compression, JSON access logging, security headers, and blocks public access to `/metrics`. Web and worker share a local named SQLite volume; the worker is isolated on its own internal Docker network.

SQLite is the recommended v0.1.0 database for this single-host topology. Do not put the database on NFS/network storage. PostgreSQL is intentionally deferred until multi-host deployment, multiple concurrent writers, HA/replication, or measured SQLite contention makes it useful.

See `docs/REVERSE_PROXY.md` and `docs/BACKUP_RESTORE.md` for deployment and recovery details.

Release tags publish a multi-architecture image for `linux/amd64` and `linux/arm64` to `ghcr.io/ploos-as/bbsintel`. When `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN` repository secrets are configured, the release workflow also publishes `${DOCKERHUB_USERNAME}/bbsintel` to Docker Hub.

## v0.1.0 qualification

The final pre-release live qualification on 2026-09-09 imported 909 Telnet BBS Guide entries and 356 Synchronet entries, producing 1,026 BBS identities and 1,133 endpoints after deduplication (947 Telnet, 186 SSH). A bounded 20-endpoint passive sample produced 8 `online`, 8 `telnet_only`, 2 `tcp_only`, and 2 `offline` results. Materialized statistics, API/metrics smoke tests, SQLite integrity/backup, production Compose validation, and Caddy validation all passed.

See `docs/M1_12_RELEASE_READINESS.md` for the final release-readiness record and `docs/M1_10_PRE_RELEASE_QUALIFICATION.md` for the earlier probe-hardening qualification.

## Roadmap

Planned next steps include additional directory adapters, RLogin/raw-TCP support, richer feeds, web UI work, and PostgreSQL support if deployment scale eventually requires it.

## License

MIT
