# M1.7 Alert query layer

M1.7 adds a read-only alert projection over BBSIntel's existing source-health, source-presence, probe, and event state. Alerts are derived at query time rather than persisted as a second source of truth.

## Endpoint

`GET /api/v1/alerts`

Query parameters:

- `limit` — default 200, maximum 1000
- `severity` — minimum severity: `info`, `warning`, `high`, or `critical`

Alerts are ordered by severity first and then newest occurrence.

## Alert categories

### source_health

Current source-health states generate one alert per source:

- `degraded` → warning
- `stale` → high
- `failed` → critical

Because the alert is projected from current state, recovery automatically removes the alert.

### source_missing

Each historical `source_entry` with `active=0` produces one warning keyed by source and source key. Returning entries are automatically removed because M1.3 sets `active=1` again.

### endpoint_status

The latest probe result for each endpoint is inspected:

- `offline` → high
- `dns_fail` → warning

Healthy later probes automatically clear the alert.

### software_change

The latest `software_changed` event per endpoint is exposed as an informational alert. Older software-change events for the same endpoint are deduplicated from the alert view while remaining available in `/api/v1/events`.

## Deduplication

Each alert has a stable `key` based on its subject, for example:

- `source-health:<source>`
- `source-missing:<source>:<source-key>`
- `endpoint:<endpoint-id>`
- `software-change:<endpoint-id>`

The event log remains append-only; the alert endpoint intentionally presents current or latest actionable state instead of repeating historical events.

## Runtime route cleanup

M1.7 also removes duplicate `registerIntelRoutes` registration. Intelligence routes are now registered once from `main`, avoiding a duplicate-pattern panic in Go's `http.ServeMux` during server startup.
