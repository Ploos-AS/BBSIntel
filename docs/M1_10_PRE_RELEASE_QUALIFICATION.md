# M1.10 Pre-release hardening and live qualification

M1.10 prepares the current BBSIntel codebase for a first tagged release by closing known release blockers and qualifying the real source/probe/API path against live data.

## Dependency reproducibility

`go.sum` is committed from a `go mod tidy` result produced by GitHub Actions. `go.mod` includes the tidy indirect dependencies.

CI now enforces:

```sh
go mod tidy && git diff --exit-code -- go.mod go.sum
```

A clean checkout therefore must build without modifying dependency metadata.

## Probe hardening

- Adaptive failure backoff never becomes shorter than `BBSINTEL_PROBE_INTERVAL`; fixed backoff stages are lower bounds rather than replacements for a larger configured base interval.
- Telnet sessions that return only Telnet negotiation bytes are classified as `telnet_only`, rather than `online`.
- `telnet_only` is considered reachable/healthy for backoff purposes.
- Telnet banner preview truncation preserves valid UTF-8.
- Passive SSH probing has a 4096-byte total pre-banner bound and no longer uses an unbounded line read.
- SSH remains passive: BBSIntel sends no client identification, authentication, or application command.

## Source and API hardening

- One-shot imports support both configured sources:

```sh
go run ./cmd/bbsintel import telnetbbsguide
go run ./cmd/bbsintel import synchronet
```

- API row iteration now checks scan and `rows.Err()` failures on the core BBS/status paths.
- The Synchronet HTML adapter was fixed after live qualification exposed that service hostnames can be inside `<a>` elements while the `(telnet)`/`(ssh)` suffix is a sibling text node. The parser now preserves `<br>` boundaries while joining adjacent text nodes.

## Live qualification

Successful GitHub Actions live qualification:

- workflow: `Live qualification`
- run: 3
- run ID: `34306780199`
- qualified commit: `37c5d8654cf085cabbb1a7c8f91945dbf3b0ee6d`

Live directory results on 2026-09-09:

| Metric | Result |
| --- | ---: |
| Telnet BBS Guide imported entries | 909 |
| Synchronet imported entries | 356 |
| Canonical BBS records | 1026 |
| Endpoints | 1132 |
| Telnet endpoints | 947 |
| SSH endpoints | 185 |
| Active directory sources | 2 |
| BBS identities present in multiple sources | 95 |

Both source-health records reported zero consecutive failures after import.

### Passive probe sample

The qualification workflow selected the first 20 Telnet/SSH endpoints and probed them with concurrency 4.

Results:

| Status | Count |
| --- | ---: |
| online | 9 |
| telnet_only | 8 |
| tcp_only | 2 |
| offline | 1 |

Five of the sampled probes produced a conservative `Synchronet` software fingerprint.

The sample is a qualification smoke test, not an availability estimate for the full BBS population.

### API smoke

With the scheduler disabled, the qualified database successfully served:

- `GET /healthz`
- `GET /api/v1/stats`
- `GET /api/v1/events?limit=5`
- `GET /api/v1/alerts?limit=5`
- `GET /api/v1/sources/health`

The API reported the 20 retained qualification endpoints and 20 probe results. Source health for both live sources was `fresh` and healthy.

## Qualification workflow policy

`.github/workflows/qualification.yml` is intentionally manual-only through `workflow_dispatch`. It reaches external BBS directories and public BBS endpoints, so it is kept separate from deterministic per-push CI.

The input `probe_limit` defaults to 20 and is capped at 100 by the workflow.

## Release assessment

M1.10 closes the immediate pre-v0.1.0 blockers identified before qualification: dependency metadata, explicit source import coverage, probe safety/bounds, the Synchronet live parser regression, and end-to-end source/probe/API smoke coverage.

Known post-M1.10 improvements remain, including stronger source-key modelling, schema-level case-insensitive endpoint identity, transactional event/probe writes, scheduler persistence, and additional source/parser quality work. They are not required to invalidate the successful M1.10 qualification above.
