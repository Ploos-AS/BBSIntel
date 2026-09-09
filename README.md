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

Environment variables: `BBSINTEL_LISTEN`, `BBSINTEL_DB`.

## API

- `GET /healthz`
- `GET /api/v1/bbs`
- `GET /api/v1/bbs/{id}`
- `GET /api/v1/stats`

## Container

```sh
docker compose up --build
```

Persistent data lives under `/data` in the container.

## Roadmap

Planned adapters include Telnet BBS Guide and other public BBS directories. Later milestones add scheduled imports, richer Telnet negotiation, software fingerprinting, SSH/RLogin probes, uptime analytics, feeds, and a web UI.

## License

MIT
