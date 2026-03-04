---
title: Configuration Reference
description: Complete environment variables reference.
order: 4
---

All configuration starts with environment variables. Most settings can also be changed at runtime via the [admin panel](/docs/web-ui/) or `gosilo config set` — database overrides take precedence over env vars.

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

The blob store also supports filesystem storage:

- `fs:/path/to/directory` — stores files on disk instead of in a database

## Web UI

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_WEB_MODE` | `full` | Web UI mode: `full`, `oauth`, `off` |

This setting requires a restart to change (it affects route registration at startup).

## Authentication & Registration

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_REGISTRATION` | `closed` | Registration mode: `open`, `approval`, `closed` |
| `GOSILO_TOKEN_LIFETIME` | `30d` | OAuth token lifetime (`30d`, `24h`, `0` = no expiry) |

These can also be changed at runtime via the admin panel.

## Rate Limiting

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_RATE_LIMIT` | `10` | Per-IP requests per second (`0` = disabled) |
| `GOSILO_RATE_BURST` | `20` | Rate limit burst size |

## Storage Quotas

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_QUOTA_MODE` | `total` | Quota mode: `off`, `total`, `user` |
| `GOSILO_QUOTA_TOTAL` | `50GB` | Global storage limit (when mode is `total`) |
| `GOSILO_QUOTA_USER` | *(none)* | Per-user default quota (when mode is `user`) |
| `GOSILO_MAX_UPLOAD` | `50MB` | Maximum upload size per request |

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

## Example Production Config

```bash
export GOSILO_ADDR=":443"
export GOSILO_BASE_URL="https://storage.example.com"
export GOSILO_DB="postgres:host=localhost dbname=gosilo sslmode=disable"
export GOSILO_BLOB="fs:/var/lib/gosilo/blobs"
export GOSILO_TLS_MODE="auto"
export GOSILO_REGISTRATION="closed"
export GOSILO_QUOTA_MODE="user"
export GOSILO_QUOTA_USER="1GB"
export GOSILO_LOG_LEVEL="info"

gosilo serve
```
