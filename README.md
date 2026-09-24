# truenas-exporter

A small Prometheus exporter for TrueNAS SCALE pool capacity and health, using
the JSON-RPC 2.0 websocket API (TrueNAS 25.04+).

Built because TrueNAS 25.x's built-in Graphite reporting export doesn't include
pool/disk space (Netdata's `diskspace` plugin is disabled in the config
TrueNAS generates), and the REST API is deprecated.

- Queries TrueNAS on each scrape (no background polling, no state)
- Single static binary, multi-arch (amd64 + arm64) `FROM scratch` image, runs as non-root
- One dependency: [`coder/websocket`](https://github.com/coder/websocket)
- TLS certificate pinning for TrueNAS's default self-signed certificate

## Metrics

| Metric | Labels | Description |
|---|---|---|
| `truenas_up` | | 1 if the last query of the TrueNAS API succeeded |
| `truenas_scrape_duration_seconds` | | Time taken to query the TrueNAS API |
| `truenas_pool_status` | `pool`, `status` | Always 1; `status` is the ZFS state (`ONLINE`, `DEGRADED`, ...) |
| `truenas_pool_healthy` | `pool` | 1 if TrueNAS considers the pool healthy |
| `truenas_pool_used_bytes` | `pool` | Usable space used (root dataset), as shown in the TrueNAS UI |
| `truenas_pool_available_bytes` | `pool` | Usable space available (root dataset), as shown in the TrueNAS UI |
| `truenas_pool_raw_size_bytes` | `pool` | Raw capacity, including parity |
| `truenas_pool_raw_allocated_bytes` | `pool` | Raw space allocated, including parity |
| `truenas_pool_raw_free_bytes` | `pool` | Raw space free, including parity |

For "how full is my pool," use `truenas_pool_used_bytes / (truenas_pool_used_bytes + truenas_pool_available_bytes)`.
The raw metrics count parity, so they overstate capacity on RAIDZ/mirror pools.

If TrueNAS is unreachable, the scrape still succeeds with `truenas_up 0` and no pool metrics.

## Configuration

All configuration is via environment variables.

| Variable | Default | Description |
|---|---|---|
| `TRUENAS_URL` | *(required)* | Websocket endpoint, e.g. `wss://truenas.lan/api/current` |
| `TRUENAS_API_KEY_FILE` | | Path to a file containing the API key (preferred) |
| `TRUENAS_API_KEY` | | The API key itself, if not using a file |
| `TRUENAS_TLS_FINGERPRINT` | | SHA-256 fingerprint of the server certificate to pin (colons optional) |
| `TRUENAS_TLS_SKIP_VERIFY` | `false` | Disable certificate verification entirely (prefer pinning) |
| `TRUENAS_ALLOW_INSECURE` | `false` | Permit a plain `ws://` URL (see below) |
| `TRUENAS_TIMEOUT` | `10s` | Timeout for each collection |
| `HTTP_PORT` | `8080` | Port for `/metrics` and `/health` |
| `LOG_LEVEL` | `info` | `debug` for per-scrape logging |

With none of the TLS options set, the certificate is validated against the
system CA bundle, which suits a TrueNAS instance with a real certificate.

### Why `ws://` is refused by default

TrueNAS 25.04+ automatically revokes an API key that is used over an
unencrypted connection. Use `wss://`.

### Pinning the default self-signed certificate

TrueNAS ships a self-signed certificate for `localhost`, which fails normal
validation for any other hostname. Rather than disabling verification, pin
it:

```bash
echo | openssl s_client -connect truenas.lan:443 2>/dev/null | openssl x509 -noout -fingerprint -sha256
```

and set `TRUENAS_TLS_FINGERPRINT` to the result. If the certificate is ever
regenerated, collection fails with a `certificate fingerprint mismatch` log
line (showing the new fingerprint) until the pin is updated.

### API key

Create a key in the TrueNAS UI under your user's settings (**API Keys**). The
exporter only calls `pool.query` and `pool.dataset.query`. TrueNAS API keys
inherit the privileges of the user they belong to, so a dedicated user with a
read-only role is the least-privileged option.

## Running

```bash
docker run --rm -p 8080:8080 \
  -e TRUENAS_URL=wss://truenas.lan/api/current \
  -e TRUENAS_API_KEY_FILE=/run/secrets/api-key \
  -e TRUENAS_TLS_FINGERPRINT=B0:BF:... \
  -v ./api-key:/run/secrets/api-key:ro \
  jakerobb/truenas-exporter:latest
```

## Building

```bash
cd src
go test ./...
go build -o truenas-exporter ./cmd
```

Images are built and pushed to Docker Hub by GitHub Actions on every push to
`main`, tagged `latest` and `YYYYMMDD`.
