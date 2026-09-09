# M1.8 Alert filtering and statistics

M1.8 extends the M1.7 alert projection with reusable filters and dashboard-oriented statistics. The alert set remains derived from current source health, source presence, endpoint state, and latest software-change events.

## Alert filters

`GET /api/v1/alerts` supports:

- `limit` — default 200, maximum 1000
- `severity` — minimum severity (`info`, `warning`, `high`, `critical`)
- `category` — exact alert category
- `source` — exact source name, case-insensitive
- `bbs_id` — exact BBS id

Filters can be combined. For example:

`GET /api/v1/alerts?severity=warning&category=source_missing&source=telnetbbsguide`

The package keeps the original `alerts.List` API for compatibility and implements new filtering through `alerts.Query` and `alerts.Filter`.

## Alert statistics

New endpoint:

`GET /api/v1/alerts/stats`

It accepts the same `severity`, `category`, `source`, and `bbs_id` filters. `limit` is ignored for statistics so counts always represent the complete filtered alert projection.

The response contains:

- `total`
- `by_severity`
- `by_category`
- `by_source`

Severity buckets always include `info`, `warning`, `high`, and `critical`, including zero counts. Category and source maps only include values present in the filtered result.

## Semantics

Statistics are calculated from the same deduplicated alert projection used by `/api/v1/alerts`. This avoids differences between list and dashboard semantics and avoids adding another persisted source of truth.
