# rstash

A [remoteStorage](https://remotestorage.io/) server you host yourself. Implements
[draft-dejong-remotestorage-26](https://datatracker.ietf.org/doc/html/draft-dejong-remotestorage-26).

remoteStorage lets apps keep their data in storage you control instead of theirs. rstash
is that storage. It runs as one binary against a SQLite file, and everything past the
first launch is configured in a web UI. The intended user runs one server for themselves,
or for a few family and friends.

Written in C#/.NET 10. MIT licensed.

## Running it

```sh
curl -LO https://github.com/Klutch-Software-Incorporated/rstash/releases/latest/download/rstash-linux-amd64
chmod +x rstash-linux-amd64
./rstash-linux-amd64
```

Binaries are published for Linux (amd64, arm64), macOS (amd64, arm64), and Windows
(amd64). There is also a `Dockerfile` in the repository root.

Then open `http://localhost:8080`. While no account exists, every page redirects to a
setup wizard that creates the first admin. After that, sign in and use the web UI.

By default this writes two files in the working directory: `rstash.sqlite` for metadata
and `rstash-blobs.sqlite` for file contents. Point `RSTASH_DB` and `RSTASH_BLOB`
elsewhere before you have data worth keeping.

## What it does

The protocol surface is complete: `GET`/`PUT`/`DELETE`/`HEAD` on documents and folders,
ETags and conditional requests, JSON-LD folder listings, WebFinger discovery at
`/.well-known/webfinger`, and the OAuth app-authorization flow (authorization code with
PKCE, plus the implicit grant that older clients still use). Folders are implicit,
derived from document paths rather than stored. Paths under `/public/` are readable
without a token.

The web UI covers setup, sign-in, account settings, a file browser, the app-consent
screen, and an admin area with server settings, user management, and an audit log.

Accounts have storage and egress quotas, stamped at creation from a configurable
default. There are also server-wide caps. An operator setting controls whether apps may
write under `/public/`.

Sign-in is a username and a password, held by ASP.NET Core Identity. See
[docs/IDENTITY.md](docs/IDENTITY.md) for how that relates to the OAuth tokens issued to
apps, which are a separate thing despite the similar URLs.

## Not implemented yet

Worth knowing before you deploy:

- **TLS.** rstash speaks plain HTTP. Terminate TLS at nginx, Caddy, or Traefik, set
  `RSTASH_TRUST_PROXY=true`, and set `RSTASH_BASE_URL` to the public `https://` URL.
- **Rate limiting.** The settings exist in the admin UI. Nothing enforces them yet.
- **Refresh tokens.** Only `grant_type=authorization_code` is accepted.
- **Email verification.** Password reset by email works if `RSTASH_EMAIL` is set;
  verifying an address at signup does not.
- **S3 blob storage.** Planned. Azure Blob and the filesystem work today.
- **Range requests.** Whole documents only.

[ROADMAP.md](ROADMAP.md) has the ordering and the things that are deliberately not
planned.

## Configuration

Six environment variables are read at boot. Everything else is a runtime setting stored
in the database and edited in the admin UI: registration mode, quota defaults, token
lifetime, upload size limit, legal pages, and so on.

Run `rstash env` to print a documented template.

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address, `host:port`. An empty host binds all interfaces. |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public URL of the server. Every absolute URL rstash emits is built from this and never from the incoming request. Validated at boot. |
| `RSTASH_TRUST_PROXY` | `false` | Honour `X-Forwarded-Proto`, `-Host`, and `-For`. Turn this on only behind a reverse proxy: the headers are forgeable by anyone who can reach the server directly. |
| `RSTASH_DB` | `sqlite:rstash.sqlite` | Metadata database DSN. |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.sqlite` | Blob store DSN. |
| `RSTASH_EMAIL` | *(unset)* | Email provider DSN. Without it, password-reset mail is silently dropped. |

### Database DSNs

| Database | Format | Example |
|----------|--------|---------|
| SQLite | `sqlite:path` | `sqlite:/var/lib/rstash/rstash.sqlite` |
| PostgreSQL | `postgres:` then an Npgsql connection string | `postgres:Host=db;Database=rstash;Username=rstash;Ssl Mode=Require` |
| MySQL | `mysql:dsn` | not wired yet |
| SQL Server | `mssql:dsn` | not wired yet |

Append `;Auth=Entra` to a Postgres DSN to authenticate to Azure Database for PostgreSQL
with Entra ID instead of a password.

If you put the SQLite file on an SMB share such as Azure Files, add
`?journal_mode=delete`. WAL journaling corrupts SQLite over SMB.

### Blob store DSNs

| Backend | Format | Example |
|---------|--------|---------|
| SQLite | `sqlite:path` | `sqlite:/var/lib/rstash/blobs.sqlite` |
| Filesystem | `fs:/path` | `fs:/var/lib/rstash/blobs` |
| Azure Blob | `azureblob://{account}/{container}` | `azureblob://mystorage/rstash` |
| Database | any database DSN | `postgres:Host=db;Database=blobs;Username=rstash` |
| S3-compatible | `s3:bucket?params` | not wired yet |

Azure Blob authenticates with a shared key, a SAS token, or `DefaultAzureCredential`.

For throwaway local runs, `sqlite::memory:` gives you a database wiped on restart. If
both DSNs use it they have to name *different* in-memory databases, since one cannot
hold both schemas.

### Email

`RSTASH_EMAIL` takes a provider DSN. Only Resend is implemented:

```
resend:re_yourapikey?from=noreply@example.com
```

Leave it unset and outbound mail goes to a no-op sender. The only thing that currently
needs it is password reset, so an admin who can still sign in does not need it
configured.

## CLI

Running `rstash` with no arguments starts the server. The subcommands exit before the
web host starts.

```
rstash              start the server
rstash env          print a documented environment-variable template
rstash check        validate configuration, test database and blob connectivity
rstash seed [user]  fill an account with sample modules, folders, and files
```

`rstash check` exits non-zero on failure, which makes it usable in a healthcheck or a
deploy gate. The running server also answers `/healthz`.

## Development

You need the [.NET 10 SDK](https://dotnet.microsoft.com/download).

```sh
dotnet build Rstash.slnx
dotnet run --project src/Rstash.Server     # http://localhost:8080
dotnet test Rstash.slnx
```

`dotnet watch --project src/Rstash.Server` gives you hot reload.

Schema changes are hand-written [FluentMigrator](https://fluentmigrator.github.io/)
migrations in `src/Rstash.Database/Migrations/`, one database-agnostic set applied at
startup. EF Core owns runtime queries and Identity but not DDL, and a test guards the
two against drifting apart. There is no `dotnet-ef` codegen step.

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

## Layout

Seven projects under `src/`, two under `tests/`. Dependencies point inward, and
interfaces live beside their implementations rather than in a shared contracts project.

```
src/
  Rstash.Model          Protocol types and pure rules: Node, ETag, Scope, StoragePath. No IO.
  Rstash.Services       Use cases: RemoteStorageService, SettingsService, TokenStore,
                        EgressTracker, AuditService
  Rstash.Storage        Blob backends behind IStorage: filesystem, database, Azure Blob
  Rstash.Database       EF Core context, Identity, NodeStore, entity configs, migrations
  Rstash.Notifications  Outbound email behind IEmailSender: Resend, no-op
  Rstash.Web            Blazor class library + MudBlazor: every page
  Rstash.Server         The executable: minimal-API endpoints, DI and middleware, CLI
tests/
  Rstash.Core.Tests         Unit tests over Model, Services, Storage, Database
  Rstash.IntegrationTests   End-to-end against the real host via WebApplicationFactory
```

[CLAUDE.md](CLAUDE.md) has the longer version, plus the conventions worth knowing before
you change anything.

## History

rstash was written in Go first. It was rewritten in C# in mid-2026, and the Go source is
kept under `legacy/` as a reference for behaviour rather than something that builds or
ships. [docs/PARITY-GAPS.md](docs/PARITY-GAPS.md) tracks what has not been carried
across, and what was left behind deliberately.

## License

MIT. See [LICENSE](LICENSE).
