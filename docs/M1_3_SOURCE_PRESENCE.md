# M1.3 Source presence reconciliation

M1.3 makes directory imports snapshot-aware so BBSIntel can distinguish current source presence from historical provenance.

## Presence state

Each `source_entry` now stores:

- `active` — whether the entry was present in the most recent successful complete snapshot for that source
- `missing_since` — when the entry first disappeared from a successful snapshot
- `last_seen` — the most recent successful snapshot in which the entry was present

Historical source rows are never deleted merely because a directory stops listing them.

## Reconciliation safety

Presence reconciliation only happens after `Adapter.Fetch` succeeds and every returned entry validates. A fetch error leaves all previous presence state untouched.

An empty successful snapshot is rejected rather than interpreted as "the directory is now empty". This prevents a parser regression or unexpectedly empty response from marking an entire source as disappeared.

## Events

When an active source entry is absent from the next successful snapshot, BBSIntel records:

- `source_disappeared`

When that source key later reappears, BBSIntel records:

- `source_returned`

Both events are appended to `change_event` and are visible in the normal event feeds.

## Intelligence semantics

The provenance API continues to return both active and historical source rows, including `active` and `missing_since`.

Conflict/intelligence calculations only use active source rows. Stale historical claims therefore remain auditable without continuing to influence current disagreement metrics.

`GET /api/v1/intelligence/stats` also exposes `stale_source_entries`.
