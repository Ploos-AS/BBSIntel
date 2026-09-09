# Reverse proxy deployment

BBSIntel is intended to run behind a reverse proxy for public deployments.

The application itself serves plain HTTP on port 8080. The reverse proxy should own public TLS termination, HTTP/2 or HTTP/3, client-IP aware rate limiting, request/body limits, access logging, compression, security headers, and any public hostname policy.

BBSIntel deliberately does not trust or interpret `X-Forwarded-For`, `Forwarded`, or similar client-IP headers. This avoids accepting spoofed client identity when the service is accidentally exposed directly. Client-IP based controls belong at the trusted edge proxy.

## Production Compose with Caddy

The repository's `compose.yaml` uses Caddy as the only public entry point. `bbsintel-web` is exposed only on the private Compose `edge` network and has no host-published port. Caddy publishes TCP 80/443 and UDP 443 for automatic HTTPS and HTTP/3.

Set the public hostname and start the stack:

```sh
BBSINTEL_DOMAIN=bbs.example.org docker compose up -d --build
```

The hostname must resolve to the host and TCP ports 80/443 must be reachable for normal public ACME certificate issuance. Caddy stores certificate state in the persistent `caddy-data` volume.

The stack uses these named volumes:

- `bbsintel-data` — shared local SQLite storage for web and worker
- `caddy-data` — certificates and Caddy state
- `caddy-config` — Caddy runtime configuration state

`bbsintel-web` and `bbsintel-worker` share only the local data volume; they run on separate Docker networks. The worker is not reachable from Caddy.

## Public Caddy policy

The checked-in `Caddyfile`:

- enables automatic HTTPS
- enables zstd/gzip response compression
- adds conservative security headers
- removes Caddy's `Server` response header
- performs active readiness checks against `/readyz`
- writes JSON access logs to stdout
- returns 404 for the public `/metrics` path

BBSIntel itself remains responsible for API `Cache-Control` headers. Standard Caddy does not cache proxied responses by default, so the application's existing 30/60/300-second cache policies pass through unchanged. A CDN or caching proxy can be added later and should honor those upstream headers.

`/metrics` is intentionally not exposed through the public Caddy virtual host. Prometheus should scrape `bbsintel-web:8080/metrics` from a trusted internal Docker network or another explicitly private monitoring path.

## Standalone reverse proxy

If Caddy or another reverse proxy runs directly on the host rather than in this Compose stack, publish BBSIntel only on loopback and proxy to `127.0.0.1:8080`. Do not expose the application port directly to the Internet.

## Application overload protection

BBSIntel also has a proxy-independent in-process concurrency ceiling controlled by:

```text
BBSINTEL_HTTP_MAX_INFLIGHT=64
```

When the ceiling is reached, normal requests fail fast with HTTP `503 Service Unavailable` and `Retry-After: 1` instead of allowing an unbounded queue to build behind SQLite or expensive handlers. `/healthz` and `/readyz` remain exempt so orchestration and monitoring can still determine process/database health during saturation.

This limiter is intentionally not a per-IP rate limiter. Per-IP logic in the application would require a trusted-proxy configuration and careful parsing of forwarded-address chains. Keep client-IP rate limiting at the trusted reverse proxy or upstream WAF/CDN.

## Database boundary

SQLite remains the recommended v0.1.0 database for a single-host deployment. Keep the database on local storage; do not place it on NFS or another network filesystem.

A future multi-host or horizontally scaled deployment should move persistence to PostgreSQL rather than attempting to share SQLite across hosts. PostgreSQL is not required for the current one-host web/worker architecture.

## Recommended topology

```text
Internet
   |
   v
Caddy :80/:443
TLS / compression / edge policy
   |
   v
bbsintel-web :8080 (private Docker network)
   |
   +---- bbsintel-data (local SQLite volume)
   |
bbsintel-worker (separate Docker network)
```
