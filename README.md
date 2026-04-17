# rstash — remoteStorage Server

A self-hosted [remoteStorage](https://remotestorage.io/) server written in Go, implementing [draft-dejong-remotestorage-26](https://datatracker.ietf.org/doc/html/draft-dejong-remotestorage-26). Single binary, multi-database support (SQLite, PostgreSQL, MySQL, SQL Server), web-based administration.

## Features

- **Full remoteStorage protocol** — GET/PUT/DELETE/HEAD for documents and folders, ETags, conditional requests, folder listings with JSON-LD
- **WebFinger discovery** — `/.well-known/webfinger` endpoint for client bootstrapping
- **OAuth 2.0** — Built-in authorization flow (implicit + PKCE) with consent screen and scope management
- **Web-based admin** — Setup wizard, user management, settings, audit log, file browser, abuse reports — all through the browser
- **Multi-database support** — SQLite (default), PostgreSQL, MySQL, SQL Server via GORM
- **Pluggable blob storage** — SQLite (default), filesystem, S3-compatible, or any supported database
- **TLS support** — Manual certificate, automatic via Let's Encrypt (autocert), or off
- **Per-IP rate limiting** — Token bucket with configurable rate and burst
- **Storage quotas** — Global or per-user quota enforcement
- **Email integration** — Email verification, password reset, admin announcements (via Resend)
- **Single binary** — All templates and static assets embedded via `go:embed`

## Quick Start

```sh
# Download the latest binary (Linux amd64)
curl -LO https://fossil.klutch.software/rstash/uv/rstash-linux-amd64
chmod +x rstash-linux-amd64

# Start the server
./rstash-linux-amd64
```

[Pre-built binaries](https://fossil.klutch.software/rstash/uvlist) are available for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64).

On first run, rstash redirects to a setup wizard where you review settings and create the admin account. All further management happens through the web UI.

## CLI

The CLI is intentionally minimal:

```
rstash              Start the server (default command)
rstash env          Print a documented .env configuration template
rstash check        Validate configuration and test database/blob store connectivity
```

## Configuration

All configuration is via environment variables. Run `rstash env` for a documented template.

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address (host:port) |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public URL for WebFinger and OAuth |
| `RSTASH_DB` | `sqlite:rstash.sqlite` | Metadata database DSN |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.sqlite` | Blob store DSN |
| `RSTASH_LOG_LEVEL` | `info` | Log level: debug, info, warn, error |
| `RSTASH_LOG_FILE` | | Log file path (empty = stderr only) |
| `RSTASH_TLS_MODE` | *(auto-detect)* | TLS mode: `off`, `manual`, `auto` |
| `RSTASH_TLS_CERT` | | TLS certificate file (for `manual` mode) |
| `RSTASH_TLS_KEY` | | TLS private key file (for `manual` mode) |
| `RSTASH_TLS_CACHE` | `./certs` | Autocert certificate cache directory |
| `RSTASH_EMAIL` | | Email provider DSN (e.g. `resend:KEY?from=noreply@example.com`) |

Additional settings (registration mode, rate limits, quotas, OAuth token lifetime, max upload size, legal pages, etc.) are managed at runtime through the admin web UI and stored in the database.

### Database DSN Formats

| Database | DSN Format | Example |
|----------|-----------|---------|
| SQLite | `sqlite:path` | `sqlite:rstash.sqlite` |
| PostgreSQL | `postgres:connstring` | `postgres:host=localhost dbname=rstash` |
| MySQL | `mysql:dsn` | `mysql:user:pass@tcp(localhost:3306)/rstash?parseTime=true` |
| SQL Server | `mssql:dsn` | `mssql:sqlserver://sa:Pass@localhost:1433?database=rstash` |

### Blob Store DSN Formats

| Backend | DSN Format | Example |
|---------|-----------|---------|
| SQLite | `sqlite:path` | `sqlite:rstash-blobs.sqlite` |
| Filesystem | `fs:/path` | `fs:/var/lib/rstash/blobs` |
| S3-compatible | `s3:bucket?params` | `s3:my-bucket?region=us-west-2` |
| Azure Blob Storage | `azureblob:container?params` | `azureblob:rstash?account=mystorage` |
| Database | Any database DSN | `postgres:host=localhost dbname=blobs` |

The S3 backend works with AWS S3, MinIO, DigitalOcean Spaces, Backblaze B2, Cloudflare R2, and any S3-compatible service. The bucket must exist before starting the server.

The Azure Blob Storage backend talks natively to Azure. On Azure App Service / VM / Functions with a managed identity granted the `Storage Blob Data Contributor` role, set only `account=NAME` in the DSN — no secrets needed. For other environments, pass an explicit `key=` (shared key) or `sas=` (SAS token). See the `blob_dsn` setting's help text for full details.

## Development

Requires Go 1.24+. [Task](https://taskfile.dev/) is used as the task runner:

```sh
task build    # Build the binary
task run      # Run via go run
task test     # Run all tests
task fmt      # Format code
task vet      # Run go vet
task clean    # Remove build artifacts
```

Build with `-tags dev` to serve templates and assets from disk for hot reload during development:

```sh
go run -tags dev .
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

Source control is managed via [Fossil](https://fossil-scm.org/), not git. Repository: [fossil.klutch.software/rstash](https://fossil.klutch.software/rstash)

## Architecture

```
main.go                 Entry point — delegates to internal/cli
internal/
  cli/                  Cobra CLI (serve, env, check)
  config/               Environment variable loading and validation
  settings/             Runtime settings (DB overrides + env defaults)
  db/                   GORM database layer, Repository pattern, migrations
  model/                Domain types (User, OAuthClient, OAuthToken, Node, Session, AuditEntry)
  blob/                 Pluggable blob storage (SQLite, filesystem, S3, GORM)
  storage/              Storage service (document/folder CRUD, ETags, quotas)
  auth/                 Authentication (sessions, passwords)
  email/                Email delivery (Resend backend)
  api/                  Protocol handlers (storage API, WebFinger, OAuth, CORS, rate limiting)
  web/                  Web UI (setup, login, admin, OAuth, file browser, registration, account)
  ui/                   Embedded templates and static assets
```

## License

MIT
