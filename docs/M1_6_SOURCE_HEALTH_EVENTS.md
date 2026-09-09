# M1.6 Source health transition events

M1.6 turns source freshness classifications into durable events suitable for dashboards, alerting, and audit history.

## Persisted state

`source_health` now stores:

- `current_state`
- `state_changed_at`

The state values are the M1.5 classifications: `unknown`, `fresh`, `degraded`, `stale`, and `failed`.

## Transition evaluation

The scheduler evaluates all known source-health rows:

- after the initial import
- after each scheduled import cycle
- after each probe cycle

This means age-driven transitions can be detected even when no new source import succeeds. For example, a source can progress from `fresh` to `degraded` and later `stale` solely because its last successful import is getting old.

A transition is persisted only when the calculated state differs from `current_state`.

## Events

Each transition appends one `change_event` with:

- `kind = source_health_changed`
- `source = <adapter name>`
- `field = freshness`
- `old_value = <previous state>`
- `new_value = <new state>`

Repeated evaluation of an unchanged state does not create duplicate events.

Recovery is represented by the same event mechanism, for example `failed -> fresh` after a successful import.

## API

`GET /api/v1/sources/health` continues to calculate live freshness and now also exposes:

- `persisted_state`
- `state_changed_at`

The live `freshness` value remains authoritative at request time. `persisted_state` is the latest state observed by the scheduler and is useful for correlating API state with transition events.

All source-health transition events are visible in the existing global event feed:

`GET /api/v1/events`
