# M1.5 Source freshness classification

M1.5 turns raw source import telemetry into an operational freshness state.

## States

- `fresh` — last successful import is at most 36 hours old and there are no consecutive failures.
- `degraded` — last success is between 36 and 72 hours old, or there are one or two consecutive failures.
- `stale` — last successful import is more than 72 hours old.
- `failed` — there has never been a successful import and the source is failing, or there are at least three consecutive failures.
- `unknown` — no successful import and no recorded failure yet.

Failure state takes precedence over freshness age at three or more consecutive failures. Staleness takes precedence over one or two failures when the last successful snapshot is already older than 72 hours.

## API

`GET /api/v1/sources/health` now includes:

- `freshness`
- `success_age_seconds`
- `healthy`

`healthy` is retained as a compatibility convenience and is true only for `fresh` sources.

The thresholds are intentionally more tolerant than the default 24-hour import interval. A single slightly late scheduled import therefore does not immediately classify a source as stale.
