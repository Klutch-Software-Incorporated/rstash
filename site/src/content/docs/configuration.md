---
title: Configuration Reference
description: Complete environment variables reference.
order: 4
---

All configuration starts with environment variables. Many settings can also be changed at runtime via the [admin panel](/docs/web-ui/) — database overrides take precedence over env vars.

Run `gosilo env` to generate a documented `.env` template:

```bash
gosilo env > .env
```

## Server

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_ADDR` | `:8080` | Listen address (`host:port`) |
| `GOSILO_BASE_URL` | `http://localhost:8080` | Public-facing URL of the server |
| `GOSILO_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `GOSILO_LOG_FILE` | *(none)* | Path to log file (empty = stderr only) |

`GOSILO_BASE_URL` is important — it's used in WebFinger responses and OAuth redirects. Set it to your actual public URL in production.

## Database

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_DB` | `sqlite:gosilo.db` | Metadata database DSN |
| `GOSILO_BLOB` | `sqlite:gosilo-blobs.db` | Blob store DSN |

Both support these DSN prefixes:

- `sqlite:path` — SQLite (default, no external dependencies)
- `postgres:connection-string` — PostgreSQL
- `mysql:dsn` — MySQL / MariaDB
- `mssql:dsn` — SQL Server

The blob store also supports:

- `fs:/path/to/directory` — stores files on disk instead of in a database
- `s3:bucket?region=us-east-1` — S3-compatible object storage (AWS, DigitalOcean Spaces, MinIO, etc.)

## TLS

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_TLS_MODE` | *(auto-detect)* | TLS mode: `off`, `manual`, `auto` |
| `GOSILO_TLS_CERT` | | Path to TLS certificate file |
| `GOSILO_TLS_KEY` | | Path to TLS private key file |
| `GOSILO_TLS_CACHE` | `./certs` | Autocert certificate cache directory |

When `GOSILO_TLS_MODE` is empty, gosilo auto-detects:
- If `GOSILO_TLS_CERT` and `GOSILO_TLS_KEY` are set → `manual` mode
- Otherwise → TLS disabled

Set `GOSILO_TLS_MODE=auto` for automatic HTTPS via Let's Encrypt. Your `GOSILO_BASE_URL` must use a real domain and port 443 must be reachable.

## Runtime Settings

The following settings have sensible defaults and can be changed at any time through the admin panel (Settings page). Changes take effect immediately without restarting the server.

- **Registration mode** — `closed` (default), `open`, or `approval`
- **Rate limiting** — per-IP requests per second (default: 10 req/sec, burst 20)
- **Storage quotas** — off, global total (default: 50 GB), or per-user
- **Max upload size** — per-request limit (default: 50 MB)
- **OAuth token lifetime** — how long tokens last (default: 30 days)
- **Refresh tokens** — enabled/disabled and lifetime
- **Metrics** — public, admin-only, or off
- **Legal pages** — terms of service and privacy policy (built-in text, custom URL, or off)
- **Blocked MIME types** — content type filtering for uploads
- **Public writes** — whether unauthenticated writes to public paths are allowed

## Example Production Config

```bash
export GOSILO_ADDR=":443"
export GOSILO_BASE_URL="https://storage.example.com"
export GOSILO_DB="postgres:host=localhost dbname=gosilo sslmode=disable"
export GOSILO_BLOB="fs:/var/lib/gosilo/blobs"
export GOSILO_TLS_MODE="auto"

gosilo
```
