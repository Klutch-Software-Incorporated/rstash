---
title: Getting Started
description: Download rstash and get it running in under a minute.
order: 1
---

## Download a Binary

Grab the latest release for your platform:

```bash
# Linux (amd64)
curl -LO https://fossil.klutch.software/rstash/uv/rstash-linux-amd64
chmod +x rstash-linux-amd64
sudo mv rstash-linux-amd64 /usr/local/bin/rstash
```

[Pre-built binaries](https://fossil.klutch.software/rstash/uvlist) are available for Linux, macOS, and Windows.

## Build from Source

You'll need Go 1.22 or later:

```bash
fossil clone https://fossil.klutch.software/rstash rstash.fossil
fossil open rstash.fossil
go build -o rstash .
```

## First Run

Before starting rstash, decide if you want to change any startup configuration. These environment variables **must be set before the server starts** and cannot be changed at runtime:

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address (`host:port`) |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public URL (used in WebFinger, OAuth redirects) |
| `RSTASH_DB` | `sqlite:rstash.sqlite` | Metadata database — `sqlite:`, `postgres:`, `mysql:`, or `mssql:` |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.sqlite` | Blob storage — same DB prefixes, plus `fs:/path` or `s3:bucket` |
| `RSTASH_TLS_MODE` | *(auto-detect)* | `off`, `manual`, or `auto` (Let's Encrypt) |
| `RSTASH_EMAIL` | *(none)* | Email provider DSN (e.g. `resend:KEY?from=noreply@example.com`) |

By default, rstash uses SQLite for both metadata and blob storage — no external dependencies needed. This works great for personal and small-group use. If you want PostgreSQL, S3, or another backend, set the variables before first run. See the [configuration reference](/docs/configuration/) for all DSN formats and options.

> **Tip:** Run `rstash env` to generate a documented `.env` template with all available variables, or `rstash check` to validate your configuration and test database/blob store connectivity.

Once you're ready, start the server:

```bash
rstash
```

Visit `http://localhost:8080` in your browser. Since no users exist yet, rstash shows the setup wizard:

1. **Review settings** — rstash shows your current server configuration (database, storage backend, base URL, TLS). If you're using the SQLite defaults, a note explains what that means and how to change it.
2. **Create admin account** — pick a username, password, and email address for the first admin user.

That's it — you're in. Everything else (registration mode, quotas, rate limits, OAuth settings, etc.) can be configured at runtime through the [admin panel](/docs/web-ui/).

## Connect a remoteStorage App

Any [remoteStorage-compatible app](https://remotestorage.io/apps/) can connect to your server. When the app asks for a user address, enter:

```
yourname@your-server-hostname.com
```

The app discovers your server via WebFinger and walks you through the OAuth authorization flow to grant it access.

## Next Steps

- [Setup](/docs/setup/) — configure registration, quotas, and common settings
- [Web UI](/docs/web-ui/) — explore the dashboard, file browser, and admin panel
- [Deployment](/docs/deployment/) — run in production with TLS
