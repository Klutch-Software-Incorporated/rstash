# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

rstash is a remoteStorage server (draft-dejong-remotestorage-26) written in **C# / .NET 10**.
It implements the remoteStorage protocol including WebFinger discovery, OAuth 2.0 + PKCE
authorization, and the storage API (GET/PUT/DELETE/HEAD for documents and folders).

The target audience is technical self-hosters running personal or small family/friends
servers. The design prioritizes a "just run it and go" experience — run `rstash` to
start the server and complete setup through the web UI.

> **History:** this codebase was rewritten from Go to idiomatic C#/.NET. The Go tests were
> ported to xUnit as the correctness baseline. The Go tree was deleted in August 2026 once
> it had stopped earning its keep as a parity oracle; the `go-final` tag preserves it.

Source control is Git, hosted on GitHub (Klutch-Software-Incorporated/rstash).

> **Direction:** self-hosting is the product. A hosted multi-tenant offering was
> explored through mid-2026 and **dropped** in August 2026 — the embedded OIDC
> provider, the `storage_users` split, and the entitlement indirection it required were
> all removed. Weigh new work against "does this make rstash better to self-host?"; see
> [ROADMAP.md](ROADMAP.md) for the non-goals.

## Build & Run Commands

- **Build:** `dotnet build Rstash.slnx`
- **Run:** `dotnet run --project src/Rstash.Server` (or `dotnet run --project src/Rstash.Server -- env|check`)
- **Run all tests:** `dotnet test Rstash.slnx`
- **Run one test project:** `dotnet test tests/Rstash.Core.Tests`
- **Run a single test:** `dotnet test --filter "FullyQualifiedName~TestName"`
- **Schema migrations:** hand-written [FluentMigrator](https://fluentmigrator.github.io/) migrations in `src/Rstash.Database/Migrations/` (one agnostic set for all dialects). Add a new `[Migration(<version>)]` class; it is applied automatically at startup. No `dotnet-ef` codegen.
- **Single-file publish:** `dotnet publish src/Rstash.Server -c Release -r <rid> --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true`

## CLI

Running `rstash` starts the server (default). Subcommands short-circuit before the web host:

- `rstash` / `rstash serve` — start the HTTP server
- `rstash env` — print a documented env-var template (generated from the setting registry)
- `rstash check` — validate configuration and test database/blob connectivity (non-zero exit on failure)
- `rstash seed [user]` — populate an account with sample modules/folders/files (defaults to the first account)

All server management (users, settings, etc.) is done through the web UI.

## Configuration

Boot-critical config is via environment variables (`rstash env` prints a template). Everything
else is a runtime setting managed in the admin UI and stored in the DB.

| Variable | Default | Description |
|----------|---------|-------------|
| RSTASH_ADDR | :8080 | Listen address (host:port) |
| RSTASH_BASE_URL | http://localhost:8080 | Public URL of the server. Validated at boot; every absolute URL rstash emits derives from it, never from the request |
| RSTASH_TRUST_PROXY | false | Honour `X-Forwarded-Proto/Host/For`. Enable only behind a reverse proxy — those headers are forgeable when rstash is directly exposed |
| RSTASH_DB | sqlite:rstash.sqlite | Metadata database DSN (sqlite:, postgres:, mysql:, mssql:; `sqlite::memory:` for a wiped-on-restart in-memory dev DB) |
| RSTASH_BLOB | sqlite:rstash-blobs.sqlite | Blob store DSN (sqlite:path, fs:/path, database DSN; s3:/azureblob: planned). In-memory dev: a *named* `sqlite::memory:blobs`, distinct from RSTASH_DB |
| RSTASH_EMAIL | | Email provider DSN (e.g. resend:API_KEY?from=noreply@example.com) |

(The setting registry also defines TLS, log level/file, etc. Wired today: SQLite and
**Postgres** databases (Postgres via a native Npgsql connection string, e.g.
`postgres:Host=…;Database=…;Username=…;Ssl Mode=Require`, plus Azure Entra ID auth by
appending `;Auth=Entra`); filesystem, database, and **Azure Blob**
(`azureblob://{account}/{container}`) blob backends. MySQL/SQL Server providers and the
S3 blob backend are stubbed behind their factories (wiring seams in place) pending their
NuGet packages.)

## Solution Layout

`Rstash.slnx` — `src/` (7 projects) + `tests/` (2 projects). Dependency arrows point inward;
ports live with their implementations, not in a central project.

- **Rstash.Model** — protocol domain + pure rules, no IO: `Node`, `AuditEntry`, `ETag`,
  `FolderDescription`/`FolderItem` (JSON-LD), `Scope`, `StoragePath`.
- **Rstash.Services** — use-case orchestration: `RemoteStorageService` (Put/Get/Head/Delete/
  GetFolder + quota + conditionals + tx), `SettingsService` + the setting registry/validator,
  `TokenStore` (OAuth tokens + auth codes), `AuditService`, `SetupState`.
- **Rstash.Storage** — swappable blob backends + the `IStorage` contract: `FileSystemStorage`,
  `DatabaseStorage` (+ `BlobDbContext`), `StorageFactory`.
- **Rstash.Database** — EF Core: `RstashDbContext : IdentityDbContext<ApplicationUser, …, long>`,
  `ApplicationUser`, `NodeStore` (implicit-folder queries), entity configs, migrations, the
  multi-provider `UseRstashDatabase` opener + case-sensitive-LIKE interceptor.
- **Rstash.Notifications** — outbound email + the `IEmailSender` contract: `ResendEmailSender`,
  `NoOpEmailSender`, `EmailSenderFactory`.
- **Rstash.Web** — Blazor (Razor Class Library) + MudBlazor: layout, setup/login/register,
  account, file browser, admin (settings/users/audit), OAuth consent.
- **Rstash.Server** — the executable host: minimal-API endpoints (storage, WebFinger, OAuth,
  file browser, admin user ops) in `Endpoints/`, Blazor root components in `Components/`,
  DI/middleware/auth wiring, the CLI.

Tests: **Rstash.Core.Tests** (xUnit unit tests over Model/Services/Storage/Database) and
**Rstash.IntegrationTests** (`WebApplicationFactory` end-to-end over the host).

## Key Conventions

- ASP.NET Core minimal APIs for the protocol surface; Blazor Web App (SSR-by-default, interactive
  islands opt-in) + MudBlazor for the human UI. Auth forms are static-SSR `EditForm` + `<InputText>`.
- **EF Core** (multi-provider; SQLite wired) for runtime queries + Identity. **DDL is owned by
  FluentMigrator**, not EF: one database-agnostic migration set (`SchemaMigrator.MigrateUp`,
  tracked in `VersionInfo`) is applied at startup; EF trusts the resulting schema and its model
  must match it (guarded by `SchemaMigratorTests`). Access via `RstashDbContext` (and
  `NodeStore`/stores) — no repository pattern. The `blobs` aux table is the one exception, still
  created via EF `EnsureCreated()`.
- **ASP.NET Core Identity** (`ApplicationUser : IdentityUser<long>`) + cookie auth; `LoginPath=/login`.
- remoteStorage **app-authorization** (storage bearer tokens, `/oauth/*`) is a custom lightweight
  OAuth AS, kept distinct from user-identity auth. PKCE S256 on the code flow.
- Interfaces live with their implementations (`IStorage` in Storage, `IEmailSender` in
  Notifications). Idiomatic C# throughout — async/await, LINQ, records, nullable refs.
- SQLite path-prefix queries are case-sensitive via a connection interceptor setting
  `PRAGMA case_sensitive_like = ON`. Note: SQLite can't `ORDER BY` a `DateTimeOffset` — order by Id.
- Runtime settings: DB overrides merged over registry defaults, atomic snapshot swap, validated on write.
- Audit logging on state-changing storage operations (extendable to auth/OAuth).
- Single-binary deployment via self-contained single-file publish; container via `Dockerfile`.

## Setup Flow

On first run (no users), a guard redirects all non-exempt routes to `/setup`. The setup page
creates the first admin account (real Identity user), signs them in via cookie, and redirects home.

## remoteStorage Protocol

- Spec: draft-dejong-remotestorage-26
- WebFinger: `GET /.well-known/webfinger?resource=acct:user@host`
- OAuth: `GET/POST /oauth/authorize` (consent), `POST /oauth/token` (authorization_code + PKCE), `POST /oauth/revoke`
- Storage: `GET/PUT/DELETE/HEAD /storage/{user}/{path...}` (bearer token + scopes)
- Public paths (`/public/`) — documents readable without auth
- Folders end with `/`, documents don't; ETags on all storage responses
- Folder listings: JSON-LD with `@context: "http://remotestorage.io/spec/folder-description"`
