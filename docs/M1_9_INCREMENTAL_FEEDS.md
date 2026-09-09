# M1.9 Incremental feeds

M1.9 adds cursor pagination and `since` filtering to the event and alert APIs without changing their existing JSON array response shape.

## Event feed

`GET /api/v1/events`

`GET /api/v1/bbs/{id}/events`

New query parameters:

- `since` — exclusive lower bound for `occurred_at`; accepts RFC3339 or SQLite UTC form `YYYY-MM-DD HH:MM:SS`
- `cursor` — opaque continuation token returned by the previous page
- `limit` — unchanged; default 200, maximum 1000

Events remain ordered by `occurred_at DESC, id DESC`. The event cursor contains both values so multiple events created in the same second paginate deterministically without gaps or duplicates.

When another page exists the response includes:

`X-Next-Cursor: <opaque-token>`

Clients should send that value unchanged as the next request's `cursor` parameter. Invalid cursors and invalid `since` timestamps return HTTP 400.

## Alert feed

`GET /api/v1/alerts`

The alert feed now accepts the same `since` and `cursor` parameters in addition to the M1.8 filters:

- `severity`
- `category`
- `source`
- `bbs_id`
- `limit`

Alerts remain ordered by severity first, then newest occurrence, then stable alert key. The opaque alert cursor therefore captures the severity rank, occurrence timestamp, and alert key.

`X-Next-Cursor` is returned only when another alert page exists.

Because alerts are a live projection rather than an append-only table, their membership can change between page requests if a source or endpoint recovers. Consumers that need immutable history should use `/api/v1/events`.

Clients must preserve the same filters, including `since`, while following an alert cursor.

## Alert stats

`GET /api/v1/alerts/stats` accepts `since` as well as the existing severity/category/source/BBS filters. Cursor and limit are intentionally ignored for statistics so counts describe the full filtered alert set.

## Compatibility

The response bodies remain JSON arrays for event and alert list endpoints. Pagination metadata is carried in the `X-Next-Cursor` response header, avoiding a breaking envelope change for existing clients.
