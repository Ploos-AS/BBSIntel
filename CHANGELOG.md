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
- Build/version metadata exposed through `bbsintel version` / `--version` and `GET /api/v1/version`.
- Manual live qualification workflow covering both directory imports, bounded passive probes, and API smoke tests.

### Qualified

The pre-release live qualification on 2026-09-09 imported 909 Telnet BBS Guide entries and 356 Synchronet entries, producing 1,026 BBS identities and 1,132 endpoints after deduplication. A bounded 20-endpoint passive probe sample completed successfully and all API smoke checks passed.
