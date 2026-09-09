# BBSIntel

BBSIntel is a self-hosted BBS discovery, verification, monitoring, and intelligence service.

It aggregates public BBS directories, normalizes and deduplicates entries, probes endpoints, stores availability history, and exposes the resulting data through an HTTP API.

## M0 scope

- Go service
- SQLite persistence
- source-adapter model
- Telnet/TCP endpoint probing
- status history
- REST API
- OCI image
- Docker Compose
- GitHub Actions CI

## Status model

- `online` — connection succeeded and application data was observed
- `tcp_only` — TCP connection succeeded but no application data was observed
- `offline` — connection failed or timed out
- `dns_fail` — hostname could not be resolved

Probing is intentionally passive: connect, read a bounded amount of data, then disconnect without logging in.

## Run locally

```sh
go run ./cmd/bbsintel
```

Defaults: listen on `:8080`, database at `./data/bbsintel.db`.

Environment variables:

- `BBSINTEL_LISTEN`
- `BBSINTEL_DB`
- `BBSINTEL_PROBE_CONCURRENCY` (default `8`)

## Import BBS directory data

```sh
go run ./cmd/bbsintel import telnetbbsguide
```

The import is idempotent for known source entries and endpoints.

## Probe imported endpoints

```sh
go run ./cmd/bbsintel probe
```

The worker probes Telnet endpoints concurrently and appends each result to `probe_result`. Re-running it builds availability history instead of overwriting prior checks.

## API

- `GET /healthz`
- `GET /api/v1/bbs` — BBS list including latest endpoint status
- `GET /api/v1/bbs/{id}` — BBS details and latest status for each endpoint
- `GET /api/v1/stats` — inventory, probe count, and latest-status totals

## Container

```sh
docker compose up --build
```

Persistent data lives under `/data` in the container.

## Roadmap

Planned next steps include scheduled imports and probing, richer Telnet negotiation, software fingerprinting, SSH/RLogin probes, uptime analytics, feeds, and a web UI.

## License

MIT
