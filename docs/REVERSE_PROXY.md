# Reverse proxy deployment

BBSIntel is intended to run behind a reverse proxy for public deployments.

The application itself serves plain HTTP on port 8080. The reverse proxy should own public TLS termination, HTTP/2 or HTTP/3, client-IP aware rate limiting, request/body limits, access logging, compression, security headers, and any public hostname policy.

BBSIntel deliberately does not trust or interpret `X-Forwarded-For`, `Forwarded`, or similar client-IP headers. This avoids accepting spoofed client identity when the service is accidentally exposed directly. Client-IP based controls belong at the trusted edge proxy.

## Default Compose exposure

`compose.yaml` publishes the web process only on host loopback:

```text
127.0.0.1:8080 -> bbsintel-web:8080
```

A reverse proxy running on the host can therefore proxy to `127.0.0.1:8080` without exposing BBSIntel directly on the host's external interfaces.

A reverse proxy running in Docker can instead share a Docker network with BBSIntel and proxy directly to `bbsintel-web:8080`; in that setup the host port publication may be removed entirely.

## Caddy example

```caddyfile
bbs.example.org {
    encode zstd gzip

    @metrics path /metrics
    respond @metrics 404

    reverse_proxy 127.0.0.1:8080
}
```

Use Caddy's rate-limit facilities or an upstream edge/WAF if per-client throttling is required. Keep `/metrics` on a private monitoring path/network rather than the public virtual host.

## Nginx example

```nginx
limit_req_zone $binary_remote_addr zone=bbsintel_per_ip:10m rate=10r/s;

server {
    listen 443 ssl;
    server_name bbs.example.org;

    location = /metrics {
        return 404;
    }

    location / {
        limit_req zone=bbsintel_per_ip burst=30 nodelay;
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

The forwarded headers are useful to infrastructure and future application features, but BBSIntel does not currently use them to identify clients or make authorization/rate-limit decisions.

## Application overload protection

BBSIntel also has a proxy-independent in-process concurrency ceiling controlled by:

```text
BBSINTEL_HTTP_MAX_INFLIGHT=64
```

When the ceiling is reached, normal requests fail fast with HTTP `503 Service Unavailable` and `Retry-After: 1` instead of allowing an unbounded queue to build behind SQLite or expensive handlers. `/healthz` and `/readyz` remain exempt so orchestration and monitoring can still determine process/database health during saturation.

This limiter is intentionally not a per-IP rate limiter. Per-IP logic in the application would require a trusted-proxy configuration and careful parsing of forwarded-address chains. Keeping that responsibility at the reverse proxy gives a smaller and safer trust boundary.

## Recommended public topology

```text
Internet
   |
   v
Reverse proxy / TLS / edge rate limits
   |
   v
BBSIntel web process :8080
   |
   +---- SQLite (local host only)
   |
BBSIntel worker process
```

The SQLite database must remain on local storage shared by the web and worker processes on one host. Do not place the SQLite file on NFS or other network storage. A future multi-host/horizontally scaled deployment should move persistence to PostgreSQL rather than sharing SQLite across hosts.
