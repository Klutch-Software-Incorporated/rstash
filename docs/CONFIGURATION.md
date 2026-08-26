# Configuration

rstash reads nine environment variables at boot. Everything else is a runtime setting,
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
| `RSTASH_TLS_MODE` | *(auto)* | `off` for plain HTTP, `files` to serve HTTPS from the pair below. Empty auto-detects from whether both paths are set. |
| `RSTASH_TLS_CERT` | *(unset)* | Path to the PEM certificate chain (certbot's `fullchain.pem`). |
| `RSTASH_TLS_KEY` | *(unset)* | Path to the PEM private key (certbot's `privkey.pem`). |

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

Two supported shapes. Pick based on whether something else on that host is already
terminating TLS.

### Behind a reverse proxy

The default, and the right answer if you already run nginx, Caddy, or Traefik. rstash
speaks plain HTTP and the proxy holds the certificate:

```sh
RSTASH_TRUST_PROXY=true
RSTASH_BASE_URL=https://storage.example.com
```

`RSTASH_TRUST_PROXY=true` is what makes rstash honour `X-Forwarded-Proto`. Without it the
app believes every request arrived over plain HTTP and builds its links accordingly.

### Serving HTTPS directly

Point rstash at a PEM certificate and key:

```sh
RSTASH_TLS_MODE=files
RSTASH_TLS_CERT=/etc/letsencrypt/live/storage.example.com/fullchain.pem
RSTASH_TLS_KEY=/etc/letsencrypt/live/storage.example.com/privkey.pem
RSTASH_BASE_URL=https://storage.example.com
```

Leaving `RSTASH_TLS_MODE` unset auto-detects — setting both paths turns TLS on. Setting only
one is a boot-time error rather than a silent fall back to HTTP.

> **rstash does not obtain certificates itself.** It ships no ACME client, so something else
> has to issue and renew: certbot, acme.sh, Caddy, `tailscale cert`, or an internal CA.

Renewals apply without a restart. rstash re-reads the files when their timestamps change,
checking at most every five minutes; a renewal caught half-written is logged and retried
while the certificate already loaded stays in service.

`rstash check` loads the pair and reports it, so a wrong path or a key that doesn't match
its certificate fails *before* you restart into it:

```
[ok]   tls:        CN=storage.example.com — expires 2026-11-14 08:12:03Z (81d)
```

There is no HTTP→HTTPS redirect and no HSTS: rstash binds a single port, so a redirect
needs a second listener it doesn't have yet. Redirect at the proxy, or leave port 80 closed.
Automatic ACME is on the [roadmap](../ROADMAP.md) but deliberately not built yet — .NET has
no maintained ACME client, and an unmaintained one is an outage on a timer.

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
