# BBSIntel

BBSIntel is a self-hosted BBS discovery, verification, monitoring, and intelligence service.

It aggregates public BBS directories, normalizes and deduplicates entries, probes endpoints, stores availability history, and exposes the resulting data through an HTTP API.

## M0 scope

- Go service
- SQLite persistence
- source-adapter model
- multiple public BBS directory sources
- cross-source endpoint deduplication
- Telnet and SSH endpoint probing
- Telnet banner cleanup and software fingerprinting
- reported vs observed software provenance
- status history
- sample-based uptime analytics
- built-in scheduler
- adaptive probe backoff
- REST API
- OCI image
- Docker Compose
- GitHub Actions CI

## Directory sources

The built-in scheduler currently imports:

- Telnet BBS Guide — advertised Telnet and SSH endpoints
- Synchronet official BBS list — advertised Telnet and SSH endpoints from `synchro.net/sbbslist.html`

Each source keeps its own `source_entry` provenance. When two directories advertise the same protocol/hostname/port, BBSIntel reuses the existing BBS identity instead of moving the endpoint or creating an orphaned duplicate. Multiple terminal protocols from one source row may share one source key and attach to the same BBS.

Directory imports are independent: one source failing does not prevent the other configured sources from being attempted.

## Status model

- `online` — connection succeeded and application data was observed
- `tcp_only` — TCP connection succeeded but no application data was observed
- `offline` — connection failed or timed out
- `dns_fail` — hostname could not be resolved

Probing is intentionally passive. Telnet probes connect and read a bounded banner without logging in. SSH probes read only the server identification line (`SSH-2.0-...`) and disconnect without authentication or key exchange.

## Run locally

```sh
go run ./cmd/bbsintel
```

The default server mode starts both the HTTP API and the scheduler. On startup the scheduler performs directory imports followed by a probe pass, then repeats imports every 24 hours and considers endpoints for probing every 30 minutes.

Defaults: listen on `:8080`, database at `./data/bbsintel.db`.

Environment variables:

- `BBSINTEL_LISTEN`
- `BBSINTEL_DB`
- `BBSINTEL_SCHEDULER_ENABLED` (default `true`)
- `BBSINTEL_IMPORT_INTERVAL` (default `24h`)
- `BBSINTEL_PROBE_INTERVAL` (default `30m`)
- `BBSINTEL_PROBE_CONCURRENCY` (default `8`)

## Adaptive probe cadence

Healthy endpoints (`online` or `tcp_only`) use the configured probe interval. Repeated `offline` or `dns_fail` results progressively reduce probe frequency:

- first failure: normal interval
- second consecutive failure: `1h`
- third consecutive failure: `6h`
- fourth or later consecutive failure: `24h`

A healthy result immediately resets the endpoint to the normal cadence. The normal interval is taken from `BBSINTEL_PROBE_INTERVAL`, so custom probe cadences still work with backoff.

## Software provenance

Directory metadata remains separate from live observations:

- `reported_software` — canonical software metadata supplied by directory import
- `observed_software` — software inferred from the latest live banner
- `software_confidence` — confidence score for the observed fingerprint
- `software_evidence` — banner token that produced the match
- `software_mismatch` — true when reported and observed software disagree

Observed values never overwrite source-reported values. When multiple directory sources resolve to the same BBS, an existing non-empty canonical value is preserved rather than being overwritten by whichever source happens to run last.

## Uptime analytics

BBSIntel calculates sample-based uptime from stored probe results. `uptime_pct` is the percentage of completed probes in the requested window whose status was `online`; it is not presented as continuous time-weighted monitoring between probes.

The analytics endpoint exposes:

- `first_seen` and `last_seen`
- `first_probe` and `last_probe`
- `last_online`
- `status_changes`
- `uptime_24h`, `uptime_7d`, and `uptime_30d`
- check counts and online-check counts for each uptime window

Status-change counts are calculated independently per endpoint so multiple protocols on one BBS do not create artificial transitions.

## One-shot commands

Import Telnet BBS Guide data:

```sh
go run ./cmd/bbsintel import telnetbbsguide
```

Probe endpoints that are currently due:

```sh
go run ./cmd/bbsintel probe
```

Run only the scheduler without the HTTP API:

```sh
go run ./cmd/bbsintel scheduler
```

The scheduler imports all configured directory adapters, including Synchronet. Imports are idempotent for known source entries and endpoints. Probe runs append to `probe_result`, building availability history instead of overwriting previous checks.

## API

- `GET /healthz`
- `GET /api/v1/bbs` — BBS list including latest endpoint status and software provenance
- `GET /api/v1/bbs/{id}` — BBS details and latest status/fingerprint for each endpoint
- `GET /api/v1/bbs/{id}/analytics` — first/last observations, status changes, and 24h/7d/30d sample-based uptime
- `GET /api/v1/bbs/{id}/history?limit=200` — newest probe history across the BBS endpoints; limit is capped at 1000
- `GET /api/v1/stats` — inventory, probe count, latest-status totals, and software mismatch count

## Container

```sh
docker compose up --build
```

Persistent data lives under `/data` in the container. Compose enables the built-in scheduler with the default 24-hour import and 30-minute probe cadence.

## Roadmap

Planned next steps include RLogin/raw-TCP probes, additional BBS directory adapters, feeds, and a web UI.

## License

MIT
