# M1.4 Source health telemetry

M1.4 separates directory-source health from BBS presence so operators can tell whether a BBS disappeared from a source or whether the source itself has not been updating successfully.

## Stored telemetry

Each source has one row in `source_health` with:

- `last_attempt_at`
- `last_success_at`
- `last_failure_at`
- `last_duration_ms`
- `last_entry_count`
- `consecutive_failures`
- `last_error`

A successful committed snapshot updates attempt/success timestamps, duration and entry count, clears `last_error`, and resets `consecutive_failures` to zero.

A fetch, validation, transaction or commit failure updates attempt/failure timestamps, duration, error text, and increments `consecutive_failures`.

Failures do not reconcile source presence. This preserves the M1.3 guarantee that source outages cannot masquerade as BBS disappearances.

## API

`GET /api/v1/sources/health`

Returns one object per source, ordered by source name. In addition to the stored telemetry, the API exposes a computed `healthy` boolean which is true after at least one successful import when there are currently zero consecutive failures.

The telemetry is operational state and remains separate from per-BBS `source_entry` provenance.
