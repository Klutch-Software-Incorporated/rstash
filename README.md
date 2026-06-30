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
- **Storage & egress limits** — Global caps plus per-user limits stamped at account creation
- **Email integration** — Email verification, password reset, admin announcements (via Resend)
- **Single binary** — All templates and static assets embedded via `go:embed`

## Quick Start

```sh
# Download the latest binary (Linux amd64)
curl -LO https://github.com/Klutch-Software-Incorporated/rstash/releases/latest/download/rstash-linux-amd64
chmod +x rstash-linux-amd64

# Start the server
./rstash-linux-amd64
```

[Pre-built binaries](https://github.com/Klutch-Software-Incorporated/rstash/releases) are available for Linux (amd64, arm64), macOS (amd64, arm64), and Windows (amd64).

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

> **Current support:** SQLite, filesystem, and database blob stores are wired
> today. The **S3** and **Azure Blob** backends, and the **PostgreSQL / MySQL /
> SQL Server** database providers, are stubbed pending their packages — the DSN
> formats above document the intended configuration. See
> [docs/PARITY-GAPS.md](docs/PARITY-GAPS.md).

## Development

Requires the [.NET 10 SDK](https://dotnet.microsoft.com/download):

```sh
dotnet build Rstash.slnx                 # Build the solution
dotnet run --project src/Rstash.Server    # Run the server (http://localhost:8080)
dotnet test Rstash.slnx                   # Run all tests
```

Use `dotnet watch --project src/Rstash.Server` for hot reload during development.

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

Source is hosted on GitHub: [Klutch-Software-Incorporated/rstash](https://github.com/Klutch-Software-Incorporated/rstash). Contributions are welcome — fork, branch, and open a pull request.

## Architecture

rstash is a C#/.NET 10 solution (`Rstash.slnx`); dependency arrows point inward.

```
src/
  Rstash.Model          Protocol domain + pure rules (Node, ETag, Scope, StoragePath) — no IO
  Rstash.Services       Use cases: RemoteStorageService, SettingsService, TokenStore, EgressTracker, AuditService
  Rstash.Storage        Blob backends + IStorage (filesystem, database; S3/Azure stubbed)
  Rstash.Database       EF Core: RstashDbContext (Identity), NodeStore, entity configs, migrations
  Rstash.Notifications  Outbound email + IEmailSender (Resend, no-op)
  Rstash.Web            Blazor (RCL) + MudBlazor: setup/login/account/browser/admin/OAuth consent
  Rstash.Server         Executable host: minimal-API endpoints, Blazor root, DI/middleware/auth, CLI
tests/
  Rstash.Core.Tests         Unit tests over Model/Services/Storage/Database
  Rstash.IntegrationTests   End-to-end over the host (WebApplicationFactory)
```

See [CLAUDE.md](CLAUDE.md) for the full Solution Layout and conventions, and
[docs/PARITY-GAPS.md](docs/PARITY-GAPS.md) for features not yet ported from the
original Go implementation (preserved under `legacy/`).

## License

MIT
