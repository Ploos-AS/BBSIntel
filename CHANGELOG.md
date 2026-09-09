# Changelog

All notable changes to BBSIntel are documented here.

## 0.1.0 - 2026-09-09

Initial public release.

### Added

- Go service with SQLite persistence, REST API, scheduler, OCI image, and Docker Compose deployment.
- Public-directory adapters for Telnet BBS Guide and the official Synchronet BBS list.
- Cross-source BBS and endpoint deduplication with source-specific provenance retained separately from canonical metadata.
- Passive Telnet and SSH endpoint probing with bounded reads and no login or authentication attempts.
- Telnet banner cleanup, conservative BBS software fingerprinting, and explicit `telnet_only` handling for negotiation-only sessions.
- Historical probe results, sample-based uptime analytics, status/software change events, and alert views.
- Source presence reconciliation, import telemetry, freshness classification, source-health state transitions, and source-health alerting.
- Source disagreement intelligence across names, software, country, and descriptions.
- Adaptive probe backoff that never shortens the configured base probe interval.
- Opaque cursor/keyset pagination and bounded filtering for the public BBS inventory, event, and alert APIs.
- Materialized hourly/daily probe rollups, public runtime snapshots, lifecycle statistics, and software/protocol/country/source dimensions.
- Prometheus/OpenMetrics-compatible `/metrics` with build, database, source, endpoint, snapshot-freshness, and route-pattern HTTP metrics.
- Readiness endpoint, HTTP timeouts/header limits, and application-level in-flight overload protection.
- Split web/worker production topology with Caddy as the public Compose frontend, automatic TLS, compression, access logging, security headers, and public `/metrics` isolation.
- SQLite maintenance commands for rollup, retention pruning, quick/full integrity checks, verified online backups, and guarded offline restore.
- Build/version metadata exposed through `bbsintel version` / `--version` and `GET /api/v1/version`.
- CI validation for Go, Docker image, Compose configuration, and Caddy configuration.
- Expanded live qualification workflow covering directory imports, bounded passive probes, materialized statistics, observability, SQLite integrity/backup, Compose/Caddy, and API smoke tests.

### Qualified

The final pre-release live qualification on 2026-09-09 imported 909 Telnet BBS Guide entries and 356 Synchronet entries, producing 1,026 BBS identities and 1,133 endpoints after deduplication (947 Telnet, 186 SSH). Both source imports completed with zero consecutive failures.

A bounded 20-endpoint passive probe sample produced 8 `online`, 8 `telnet_only`, 2 `tcp_only`, and 2 `offline` results, for 18/20 connected endpoints in the sample. Four Synchronet fingerprints were detected. Public statistics, API/metrics smoke tests, online backup, full SQLite integrity checks, production Compose validation, and Caddy configuration validation all passed.

See `docs/M1_12_RELEASE_READINESS.md` for the full release-readiness record.
