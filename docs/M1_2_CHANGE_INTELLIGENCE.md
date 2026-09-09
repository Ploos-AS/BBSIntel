# M1.2 Change Intelligence

BBSIntel records material discovery and observation changes in the append-only `change_event` table.

Recorded event kinds:

- `bbs_new` — a previously unknown BBS identity is created by directory import.
- `source_added` — a directory source begins reporting a BBS.
- `source_changed` — source URL, name, software, country, or description changes.
- `endpoint_added` — a new protocol/hostname/port is discovered.
- `status_changed` — the latest probe status changes for an endpoint.
- `software_changed` — live fingerprinting observes a different BBS software product than the previous non-empty observation.

Events keep the BBS and optional endpoint identity, event time, source, changed field, old/new values, and a compact detail string. Probe and source history remain authoritative; the event log is an intelligence feed rather than a replacement for those histories.

API:

- `GET /api/v1/events?limit=200` — global newest-first event feed.
- `GET /api/v1/bbs/{id}/events?limit=200` — newest-first events for one BBS.

`limit` defaults to 200 and is capped at 1000.
