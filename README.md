# gosilo - remoteStorage Server

`gosilo` is a self-hosted [remoteStorage](https://remotestorage.io/) server written in Go, implementing [draft-dejong-remotestorage-26](https://datatracker.ietf.org/doc/html/draft-dejong-remotestorage-26). It provides WebFinger discovery, OAuth 2.0 authorization, and the full storage API as a single binary with multi-database support (SQLite, PostgreSQL, MySQL, SQL Server).

## Key Features

- **Full remoteStorage protocol**: GET/PUT/DELETE/HEAD for documents and folders, ETags, conditional requests, folder listings with JSON-LD
- **WebFinger discovery**: Automatic `/.well-known/webfinger` endpoint for client bootstrapping
- **OAuth 2.0 implicit grant**: Built-in authorization flow with consent screen and scope management
- **CLI-first**: All administration via CLI commands — user management, config, audit, health checks
- **Web mode gating**: Run with full web UI, minimal OAuth-only UI, or API-only (`--web=full|oauth|off`)
- **Multi-database support**: SQLite (default), PostgreSQL, MySQL, and SQL Server via GORM
- **Separate metadata and blob storage**: Metadata in one database, blobs in another (or on the filesystem)
- **Pluggable blob storage**: SQLite (default), filesystem, S3-compatible object storage, or any supported database backend via connection-string DSN
- **Per-IP rate limiting**: Token bucket with configurable rate and burst, 429 responses with Retry-After
- **Storage quotas**: Global or per-user quota enforcement with runtime configuration
- **Single binary**: All templates and static assets embedded via `go:embed`

## Installation

Requires Go 1.24+.

```sh
go install gosilo@latest
```

Or build from source:

```sh
task build
```

The binary is output to `./build/gosilo.exe`.

## Quick Start

```sh
# Initialize the database and create an admin user
gosilo init

# Start the server (listens on :8080 by default)
gosilo serve
```

## CLI Reference

Running `gosilo` with no subcommand defaults to `serve`.

```
gosilo [command]

Commands:
  serve              Start the server (default)
  init               Create database and first admin user
  user add <name>    Create a new user (--admin for admin)
  user list          List all users
  user passwd <name> Change a user's password
  user promote <name> Promote user to admin
  user disable <name> Disable a user account (--enable to re-enable)
  user delete <name> Delete a user (--force to skip confirmation)
  config list        List all settings with source (env/db)
  config get <key>   Get the resolved value for a setting
  config set <key> <value>  Set a runtime setting override
  config reset <key> Remove a setting override (revert to env default)
  audit tail         Show recent audit log entries (-n 25)
  audit export       Export audit log as JSON lines
  doctor             Run health checks on the database and configuration
  env                Print a documented .env template

Global Flags:
  --db <dsn>         Override metadata database DSN (e.g. --db sqlite:mydata.db)

Serve Flags:
  --web <mode>       Override web UI mode: full, oauth, off
```

## Configuration

All configuration is via environment variables. Run `gosilo env` to generate a documented `.env` template.

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_ADDR` | `:8080` | Listen address (host:port) |
| `GOSILO_BASE_URL` | `http://localhost:8080` | Public URL for WebFinger and OAuth redirects |
| `GOSILO_DB` | `sqlite:gosilo.db` | Metadata database DSN (see [Database DSN formats](#database-dsn-formats)) |
| `GOSILO_BLOB` | `sqlite:gosilo-blobs.db` | Blob store DSN (`sqlite:path`, `fs:/path/to/dir`, `s3:bucket?params`, or any database DSN) |
| `GOSILO_WEB_MODE` | `full` | Web UI mode: `full`, `oauth`, `off` |
| `GOSILO_REGISTRATION` | `closed` | Registration mode: `open`, `closed` |
| `GOSILO_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `GOSILO_RATE_LIMIT` | `10` | Per-IP rate limit in requests/sec (0 to disable) |
| `GOSILO_RATE_BURST` | `20` | Max burst size for rate limiting |
| `GOSILO_QUOTA_MODE` | `off` | Quota enforcement: `off`, `total`, `user` |
| `GOSILO_QUOTA_TOTAL` | | Global storage limit (e.g. `10GB`, `500MB`) |
| `GOSILO_QUOTA_USER` | | Per-user storage quota (e.g. `500MB`, `1GB`) |
| `GOSILO_MAX_UPLOAD` | `50MB` | Maximum upload size per request |

### Web Modes

- **`full`** — Full web UI: setup wizard, login, admin dashboard, file browser, account settings, OAuth consent
- **`oauth`** — Minimal UI: login page, setup wizard, and OAuth consent flow only. All administration via CLI
- **`off`** — API only: storage API, WebFinger, and OAuth token endpoint. No web pages served

### Database DSN Formats

| Database | DSN Format | Example |
|----------|-----------|---------|
| SQLite | `sqlite:path` | `sqlite:gosilo.db` or `sqlite::memory:` |
| PostgreSQL | `postgres:host=... port=... user=... password=... dbname=... sslmode=...` | `postgres:host=localhost port=5432 user=gosilo password=secret dbname=gosilo sslmode=disable` |
| MySQL | `mysql:user:password@tcp(host:port)/dbname?parseTime=true` | `mysql:gosilo:secret@tcp(localhost:3306)/gosilo?parseTime=true` |
| SQL Server | `mssql:sqlserver://user:password@host:port?database=dbname` | `mssql:sqlserver://sa:Password1@localhost:1433?database=gosilo` |

### Blob Store DSN Formats

The `GOSILO_BLOB` variable controls where file content is stored:

| Backend | DSN Format | Example |
|---------|-----------|---------|
| SQLite | `sqlite:path` | `sqlite:gosilo-blobs.db` |
| Filesystem | `fs:/path/to/dir` | `fs:/var/lib/gosilo/blobs` |
| S3-compatible | `s3:bucket?params` | `s3:my-bucket?region=us-west-2` |
| PostgreSQL | `postgres:connstring` | `postgres:host=localhost dbname=blobs` |
| MySQL | `mysql:dsn` | `mysql:gosilo:secret@tcp(localhost:3306)/blobs?parseTime=true` |
| SQL Server | `mssql:dsn` | `mssql:sqlserver://sa:Password1@localhost:1433?database=blobs` |

### S3-Compatible Blob Storage

The `s3:` backend works with AWS S3, MinIO, DigitalOcean Spaces, Backblaze B2, and any S3-compatible service.

**DSN format:** `s3:bucket-name?region=us-east-1&endpoint=s3.amazonaws.com&prefix=optional/prefix`

| Parameter | Default | Description |
|-----------|---------|-------------|
| `region` | `us-east-1` | AWS/provider region |
| `endpoint` | `s3.amazonaws.com` | Custom S3-compatible endpoint |
| `prefix` | *(empty)* | Key prefix for all objects |
| `access_key` | `$AWS_ACCESS_KEY_ID` | Access key (env var fallback) |
| `secret_key` | `$AWS_SECRET_ACCESS_KEY` | Secret key (env var fallback) |
| `tls` | `true` | Use HTTPS (`true`/`false`) |

**Important:** The bucket must already exist before starting gosilo. The server verifies the bucket exists at startup and will refuse to start if it does not. Create the bucket using your provider's console, CLI, or API.

**Examples:**

```sh
# AWS S3 (credentials via env vars)
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
export GOSILO_BLOB="s3:my-bucket?region=us-west-2"

# DigitalOcean Spaces
export GOSILO_BLOB="s3:my-space?region=nyc3&endpoint=nyc3.digitaloceanspaces.com"

# MinIO (local development)
export GOSILO_BLOB="s3:gosilo?endpoint=localhost:9000&tls=false&access_key=minioadmin&secret_key=minioadmin"

# Shared bucket with key prefix
export GOSILO_BLOB="s3:shared-bucket?prefix=gosilo/prod&region=eu-west-1"
```

Objects are stored with the key layout `{prefix}/{userID}/{path}` (prefix omitted if empty).

### Runtime Settings

Settings like `registration_mode`, `log_level`, `rate_limit_rate`, `rate_limit_burst`, `quota_mode`, `quota_total`, `quota_user`, and `max_upload_size` can be overridden at runtime via `gosilo config set` without restarting the server. Use `gosilo config list` to see current values and their source (env or db override).

## Development

[Task](https://taskfile.dev/) is used as the task runner:

```sh
task build    # Build the binary
task run      # Run the server via go run
task test     # Run all tests
task fmt      # Format source code
task vet      # Run go vet
task clean    # Remove build artifacts and local database
```

Source control is managed via [Fossil](https://fossil-scm.org/), not git.

## Architecture

```
main.go                 Entry point — delegates to internal/cli
internal/
  cli/                  Cobra CLI commands (serve, init, user, config, audit, doctor, env)
  config/               Environment variable loading and validation
  db/                   Database layer (GORM), migrations, Repository pattern
  model/                Domain types (User, OAuthClient, OAuthToken, Node)
  blob/                 Pluggable blob storage (SQLite, filesystem, S3, GORM)
  storage/              Storage service (document/folder CRUD, ETags, quotas)
  auth/                 Authentication service (sessions, passwords)
  settings/             Runtime settings with DB overrides
  api/                  Protocol handlers (storage API, WebFinger, CORS, rate limiting)
  web/                  Web UI handlers (login, setup, admin, OAuth, file browser)
  ui/                   Embedded templates and static assets
```

## License

MIT
