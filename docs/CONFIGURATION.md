# Configuration

rstash reads six environment variables at boot. Everything else is a runtime setting,
stored in the database and edited in the admin UI: registration mode, quota defaults,
token lifetime, upload size limit, rate limits, legal pages, and so on.

Run `rstash env` to print a documented template of the boot variables.

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address, `host:port`. An empty host binds all interfaces. |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public URL of the server. Every absolute URL rstash emits is built from this and never from the incoming request. Validated at boot. |
| `RSTASH_TRUST_PROXY` | `false` | Honour `X-Forwarded-Proto`, `-Host`, and `-For`. Turn this on only behind a reverse proxy: the headers are forgeable by anyone who can reach the server directly. |
| `RSTASH_DB` | `sqlite:rstash.sqlite` | Metadata database DSN. |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.sqlite` | Blob store DSN. |
| `RSTASH_EMAIL` | *(unset)* | Email provider DSN. Without it, password-reset mail is silently dropped. |

> **Set `RSTASH_BASE_URL` before anyone else uses the server.** WebFinger responses and
> OAuth redirects are built from it, so a server reachable at `https://storage.example.com`
> that still has the default will hand out `http://localhost:8080` URLs and no app will be
> able to connect.

## Database DSNs

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

## Blob store DSNs

| Backend | Format | Example |
|---------|--------|---------|
| SQLite | `sqlite:path` | `sqlite:/var/lib/rstash/blobs.sqlite` |
| Filesystem | `fs:/path` | `fs:/var/lib/rstash/blobs` |
| Azure Blob | `azureblob://{account}/{container}` | `azureblob://mystorage/rstash` |
| Database | any database DSN | `postgres:Host=db;Database=blobs;Username=rstash` |
| S3-compatible | `s3:bucket?params` | not wired yet |

Azure Blob authenticates with a shared key, a SAS token, or `DefaultAzureCredential`.

For throwaway local runs, `sqlite::memory:` gives you a database wiped on restart. If both
DSNs use it they have to name *different* in-memory databases, since one cannot hold both
schemas.

## Email

`RSTASH_EMAIL` takes a provider DSN. Only Resend is implemented:

```
resend:re_yourapikey?from=noreply@example.com
```

Leave it unset and outbound mail goes to a no-op sender. The only thing that currently
needs it is password reset, so an admin who can still sign in does not need it configured.

## TLS

rstash speaks plain HTTP today. Terminate TLS at nginx, Caddy, or Traefik, then set:

```sh
RSTASH_TRUST_PROXY=true
RSTASH_BASE_URL=https://storage.example.com
```

Serving HTTPS directly — operator-supplied certificates and automatic ACME — is planned;
see [ROADMAP.md](../ROADMAP.md).

## CLI

Running `rstash` with no arguments starts the server. The subcommands exit before the web
host starts.

```
rstash              start the server
rstash env          print a documented environment-variable template
rstash check        validate configuration, test database and blob connectivity
rstash seed [user]  fill an account with sample modules, folders, and files
```

`rstash check` exits non-zero on failure, which makes it usable in a healthcheck or a
deploy gate. The running server also answers `/healthz` with per-dependency status,
returning 503 when the database or blob store is unreachable.
