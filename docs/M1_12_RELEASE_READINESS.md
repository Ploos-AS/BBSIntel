# M1.12 release readiness

Date: 2026-09-09

BBSIntel v0.1.0 has completed the M1.12 long-running public-service hardening and an expanded live qualification on GitHub Actions run 34355071274.

## Live catalogue qualification

The qualification imported both configured public directory sources successfully:

- Telnet BBS Guide: 909 entries
- Synchronet official list: 356 entries
- canonical BBS identities after deduplication: 1,026
- endpoints after deduplication: 1,133
- Telnet endpoints: 947
- SSH endpoints: 186
- active directory sources: 2
- multi-source BBS identities: 94
- consecutive source import failures: 0 for both sources

These figures are a point-in-time qualification result, not a permanent catalogue-size guarantee.

## Passive probe qualification

The workflow retained a bounded sample of 20 endpoints and performed passive probes with concurrency 4.

Results:

- online: 8
- telnet_only: 8
- tcp_only: 2
- offline: 2
- connected states combined: 18/20 (90%)
- detected Synchronet fingerprints: 4

The probe step completed all 20 endpoints without authentication or login attempts.

## Public statistics and API

The workflow refreshed materialized rollups/snapshots and successfully smoke-tested:

- `/healthz`
- `/readyz`
- `/api/v1/version`
- `/api/v1/stats`
- paginated `/api/v1/bbs`
- `/api/v1/events`
- `/api/v1/alerts`
- `/api/v1/sources/health`
- `/api/v1/statistics/daily`
- software, protocol, country, and source dimension endpoints
- `/api/v1/statistics/lifecycle`
- `/metrics`

The sampled daily rollup reported 20 checks, 8 `online`, 18 connected, 2 offline, and 0 DNS failures.

## Observability

The metrics scrape successfully exposed build information, database availability, runtime snapshot age/timestamp, BBS/endpoint/probe counts, endpoint status counts, source import telemetry, and route-pattern HTTP request metrics.

The public Caddy configuration blocks `/metrics`; metrics are intended for a private monitoring path/network.

## SQLite integrity, backup, and recovery

Qualification successfully ran:

- `maintenance rollup`
- `maintenance check`
- `maintenance check full`
- online `maintenance backup`
- final full `integrity_check`

The unit/integration suite separately includes a real backup -> mutate -> restore -> integrity-check round trip.

SQLite remains the recommended persistence backend for the v0.1.0 single-host deployment. PostgreSQL is deferred until multi-host deployment, multiple concurrent writers, HA/replication, or measured SQLite contention requires it.

## Production deployment

The production Compose and Caddy configurations were validated during qualification.

Recommended topology:

```text
Internet
   |
   v
Caddy :80/:443
   |
   v
bbsintel-web :8080 (internal only)
   |
   +---- local SQLite named volume
   |
   +---- bbsintel-worker (separate internal network)
```

Caddy owns automatic TLS, compression, access logging, security headers, and the public edge. BBSIntel retains its application-level in-flight overload ceiling. Per-client rate limiting remains an edge-proxy concern.

## Release decision

No blocking defect was found by the expanded qualification. Subject to the normal CI gate remaining green on the final documentation-only release-readiness commit, the repository is technically ready to tag `v0.1.0`.

The release workflow will validate that the tag matches `VERSION`, run the Go validation suite, build Linux amd64/arm64 binaries with checksums, publish the multi-architecture OCI image to GHCR (and Docker Hub when secrets are configured), and create the GitHub Release.
