---
title: Configuration Reference
description: Complete environment variables reference.
order: 4
---

All configuration starts with environment variables. Many settings can also be changed at runtime via the [admin panel](/docs/web-ui/) — database overrides take precedence over env vars.

Run `rstash env` to generate a documented `.env` template:

```bash
rstash env > .env
```

## Server

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address (`host:port`) |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public-facing URL of the server |
| `RSTASH_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `RSTASH_LOG_FILE` | *(none)* | Path to log file (empty = stderr only) |

`RSTASH_BASE_URL` is important — it's used in WebFinger responses and OAuth redirects. Set it to your actual public URL in production.

```bash
# Listen on all interfaces, port 443
RSTASH_ADDR=":443"

# Your public domain
RSTASH_BASE_URL="https://storage.example.com"
```

## Database

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_DB` | `sqlite:rstash.db` | Metadata database DSN |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.db` | Blob store DSN |

Both `RSTASH_DB` and `RSTASH_BLOB` accept a DSN (Data Source Name) with a scheme prefix indicating the database type:

- `sqlite:path` — SQLite (default, no external dependencies)
- `postgres:connection-string` — PostgreSQL
- `mysql:dsn` — MySQL / MariaDB
- `mssql:dsn` — SQL Server

The blob store (`RSTASH_BLOB`) also supports:

- `fs:/path/to/directory` — stores files on disk instead of in a database
- `s3:bucket?region=us-east-1` — S3-compatible object storage (AWS, DigitalOcean Spaces, MinIO, etc.)

### Database DSN Examples

```bash
# SQLite (default) — file in the current directory
RSTASH_DB="sqlite:rstash.db"
RSTASH_BLOB="sqlite:rstash-blobs.db"

# PostgreSQL
RSTASH_DB="postgres:host=localhost dbname=rstash user=rstash password=secret sslmode=disable"

# PostgreSQL with full connection URL
RSTASH_DB="postgres:postgresql://rstash:secret@localhost:5432/rstash?sslmode=require"

# MySQL
RSTASH_DB="mysql:rstash:secret@tcp(localhost:3306)/rstash?parseTime=true"

# SQL Server
RSTASH_DB="mssql:sqlserver://sa:secret@localhost:1433?database=rstash"

# Blob storage on the filesystem
RSTASH_BLOB="fs:/var/lib/rstash/blobs"

# Blob storage in S3
RSTASH_BLOB="s3:my-bucket?region=us-east-1"
```

A common production pattern is PostgreSQL for metadata with filesystem or S3 for blobs — this keeps large files out of the database:

```bash
RSTASH_DB="postgres:host=localhost dbname=rstash user=rstash password=secret sslmode=disable"
RSTASH_BLOB="fs:/var/lib/rstash/blobs"
```

## TLS

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_TLS_MODE` | *(auto-detect)* | TLS mode: `off`, `manual`, `auto` |
| `RSTASH_TLS_CERT` | | Path to TLS certificate file |
| `RSTASH_TLS_KEY` | | Path to TLS private key file |
| `RSTASH_TLS_CACHE` | `./certs` | Autocert certificate cache directory |

When `RSTASH_TLS_MODE` is empty, rstash auto-detects:
- If `RSTASH_TLS_CERT` and `RSTASH_TLS_KEY` are set → `manual` mode
- Otherwise → TLS disabled

Set `RSTASH_TLS_MODE=auto` for automatic HTTPS via Let's Encrypt. Your `RSTASH_BASE_URL` must use a real domain and port 443 must be reachable.

```bash
# Automatic TLS (Let's Encrypt)
RSTASH_TLS_MODE="auto"
RSTASH_TLS_CACHE="./certs"

# Manual TLS (your own certificates)
RSTASH_TLS_MODE="manual"
RSTASH_TLS_CERT="/etc/ssl/certs/storage.example.com.pem"
RSTASH_TLS_KEY="/etc/ssl/private/storage.example.com.key"

# Disable TLS (e.g. behind a reverse proxy)
RSTASH_TLS_MODE="off"
```

## Email

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_EMAIL` | *(none)* | Email provider DSN |

> **Note:** Only [Resend](https://resend.com) is supported as an email provider at this time. Additional providers may be added in the future.

DSN format:

- `resend:API_KEY?from=noreply@example.com`

```bash
RSTASH_EMAIL="resend:re_abc123?from=noreply@storage.example.com"
```

When configured, rstash enables email verification, password reset, and admin email tools. If a user has no email address on file, they'll be prompted to add one on login.

## Runtime Settings

The following settings have sensible defaults and can be changed at any time through the admin panel (Settings page). Changes take effect immediately without restarting the server.

- **Registration mode** — `closed` (default), `open`, or `approval`
- **Rate limiting** — per-IP requests per second (default: 10 req/sec, burst 20)
- **Storage quotas** — off, global total (default: 50 GB), or per-user
- **Max upload size** — per-request limit (default: 50 MB)
- **OAuth token lifetime** — how long tokens last (default: 30 days)
- **Refresh tokens** — enabled/disabled and lifetime (default: 90 days)
- **Metrics** — public, admin-only, or off
- **Legal pages** — terms of service and privacy policy (built-in text, custom URL, or off)
- **Blocked MIME types** — content type filtering for uploads
- **Public writes** — whether unauthenticated writes to public paths are allowed
- **Log level** — adjustable at runtime (debug, info, warn, error)

## Example Production Config

```bash
export RSTASH_ADDR=":443"
export RSTASH_BASE_URL="https://storage.example.com"
export RSTASH_DB="postgres:host=localhost dbname=rstash sslmode=disable"
export RSTASH_BLOB="fs:/var/lib/rstash/blobs"
export RSTASH_TLS_MODE="auto"
export RSTASH_EMAIL="resend:re_abc123?from=noreply@storage.example.com"

rstash
```
