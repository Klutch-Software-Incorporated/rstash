# rstash — remoteStorage Server

A self-hosted [remoteStorage](https://remotestorage.io/) server written in C#/.NET 10, implementing [draft-dejong-remotestorage-26](https://datatracker.ietf.org/doc/html/draft-dejong-remotestorage-26). Single binary, web-based administration, and a "just run it and go" setup.

Built for people who want to run their own server — for themselves, or for a handful of family and friends.

## Features

- **Full remoteStorage protocol** — GET/PUT/DELETE/HEAD for documents and folders, ETags, conditional requests, folder listings with JSON-LD
- **WebFinger discovery** — `/.well-known/webfinger` endpoint for client bootstrapping
- **OAuth 2.0** — Built-in app-authorization flow (implicit + PKCE) with consent screen and scope management
- **Web-based admin** — Setup wizard, user management, settings, audit log, and file browser, all through the browser
- **Databases** — SQLite (default) and PostgreSQL via EF Core; MySQL and SQL Server are stubbed behind their factories
- **Pluggable blob storage** — SQLite (default), filesystem, Azure Blob, or any supported database; S3 is planned
- **Storage & egress limits** — Global caps plus per-user limits stamped at account creation
- **Reverse-proxy aware** — Opt-in `X-Forwarded-*` handling for running behind nginx/Caddy/Traefik with TLS terminated upstream
- **Password reset by email** — Optional; configure `RSTASH_EMAIL` (via Resend) to enable
- **Single binary** — Self-contained single-file publish, plus a container image

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
rstash seed [user]  Populate an account with sample modules/folders/files
```

## Configuration

Boot-critical configuration is via environment variables. Run `rstash env` for a documented template.

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address (host:port) |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public URL of the server. Every absolute URL rstash emits derives from this, never from the request |
| `RSTASH_TRUST_PROXY` | `false` | Honour `X-Forwarded-Proto/Host/For`. Enable only behind a reverse proxy — those headers are forgeable when rstash is directly exposed |
| `RSTASH_DB` | `sqlite:rstash.sqlite` | Metadata database DSN |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.sqlite` | Blob store DSN |
| `RSTASH_EMAIL` | | Email provider DSN (e.g. `resend:KEY?from=noreply@example.com`) |

rstash serves plain HTTP and expects TLS to be terminated by a reverse proxy; set `RSTASH_TRUST_PROXY=true` and `RSTASH_BASE_URL` to your public `https://` URL.

Everything else (registration mode, quotas, OAuth token lifetime, max upload size, legal pages, etc.) is a runtime setting managed through the admin web UI and stored in the database.

### Database DSN Formats

| Database | DSN Format | Example |
|----------|-----------|---------|
| SQLite | `sqlite:path` | `sqlite:rstash.sqlite` |
| PostgreSQL | `postgres:` + Npgsql connection string | `postgres:Host=localhost;Database=rstash;Username=rstash;Ssl Mode=Require` |
| MySQL | `mysql:dsn` | *(stubbed — not yet wired)* |
| SQL Server | `mssql:dsn` | *(stubbed — not yet wired)* |

For Azure Database for PostgreSQL with Entra ID authentication, append `;Auth=Entra`. Use `sqlite::memory:` for a wiped-on-restart development database.

### Blob Store DSN Formats

| Backend | DSN Format | Example |
|---------|-----------|---------|
| SQLite | `sqlite:path` | `sqlite:rstash-blobs.sqlite` |
| Filesystem | `fs:/path` | `fs:/var/lib/rstash/blobs` |
| Azure Blob Storage | `azureblob://{account}/{container}` | `azureblob://mystorage/rstash` |
| Database | Any database DSN | `postgres:Host=localhost;Database=blobs;Username=rstash` |
| S3-compatible | `s3:bucket?params` | *(planned — not yet wired)* |

> If both `RSTASH_DB` and `RSTASH_BLOB` use `sqlite::memory:`, they must name
> **distinct** in-memory databases — one cannot hold both schemas.

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
