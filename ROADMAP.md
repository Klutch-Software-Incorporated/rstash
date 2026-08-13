# rstash Roadmap

rstash is a remoteStorage server for people who want to run their own, for themselves
or for a handful of family and friends. Everything below is judged against that: does
it make the thing better to *self-host*?

For the verified list of Go features not yet ported, see
[docs/PARITY-GAPS.md](docs/PARITY-GAPS.md).

## Status

Functionally complete for self-hosted remoteStorage use: the storage API
(GET/PUT/DELETE/HEAD, implicit folders, ETags, conditionals), WebFinger, OAuth
authorization-code + PKCE **and** implicit grants, the web UI (setup/login/account/file
browser/admin), SQLite + Postgres, filesystem/database/Azure blob backends, per-user
and global storage and egress quotas, and the public-write operator policy. Single-file
publish and the container build work. Tests are green.

## 1. Things a self-hoster hits first

- [x] **OAuth refresh-token grant** — `grant_type=refresh_token`, always on, both
      secrets rotating on each use. Not issued to the implicit flow.
- [ ] **Interop testing against real apps** — Litewrite, Laverna, and friends, as a
      recorded manual pass. Spec conformance and *app* compatibility are not the same
      thing, and only the second one is what a user experiences.
- [ ] **Backup & restore** — a documented, tested path for "move my server", covering
      the metadata DB and the blob store together. Currently implicit and unproven.
- [ ] **`rstash passwd <user>`** — reset a password from the CLI. Today the only route
      is email, which needs `RSTASH_EMAIL` configured; the datastore is not a viable
      fallback because Identity stores a PBKDF2 hash.
- [ ] **Email verification** — verify-email token flow + guard.

## 2. Operational hardening

- [x] **Rate limiting + account lockout** — per-IP throttling on sign-in, per-account
      throttling on storage, per-IP everywhere else; Identity lockout after 5 failed
      passwords. On by default.
- [ ] **TLS: operator-supplied certs *and* ACME** — decided (August 2026): rstash should
      serve HTTPS on its own, not only behind a proxy. `tls_mode` stays `off` by default
      so localhost and behind-a-proxy deployments are unaffected. `files` takes a cert
      and key from disk (Kestrel config, no dependency); `acme` obtains and renews a
      Let's Encrypt certificate automatically, needs port 80 reachable, and makes
      `tls_cache` state worth backing up. Derive the ACME hostname from
      `RSTASH_BASE_URL` rather than adding a `tls_domains` setting. Turning TLS on also
      implies an HTTP→HTTPS redirect and HSTS.
- [ ] **Observability** — decided (August 2026): an admin status page (uptime, version,
      counts, storage used, blob/node consistency, dependency health), an in-memory
      recent-errors view so diagnosing a problem doesn't need SSH, a runtime-adjustable
      `log_level`, and an **admin-only** Prometheus `/metrics`. Note `metrics_mode`
      currently defaults to `public`, which would expose operational detail to anyone;
      change that default when wiring it. `/healthz` already exists.
- [ ] **Admin JSON API** — API-key-authed `/api/admin/*` for scripting user and quota
      management without the browser.

## 3. Backends & deployment (as needed)

- [x] **Postgres** (native Npgsql DSN; Azure Entra ID auth via `Auth=Entra`).
- [x] **Azure Blob** (`azureblob://{account}/{container}`).
- [ ] **S3-compatible blob backend** — the one that matters for self-hosters, since it
      covers Minio, Backblaze B2, and Garage. Seam is in place.
- [ ] **MySQL / SQL Server** — stubbed behind their factories. Low priority; nobody has
      asked, and SQLite or Postgres covers the audience.

## Non-goals

- **Hosted multi-tenant rstash.** Considered and dropped (August 2026). If a managed
  offering ever happens it belongs in a separate service that *operates* rstash
  instances over the admin API — not as a second personality compiled into the binary
  every self-hoster downloads. See [docs/IDENTITY.md](docs/IDENTITY.md) for what this
  cost when it was tried, and why the embedded OIDC provider was removed.
- **rstash as its own OpenID Connect provider.** Optional *external* SSO (Authelia,
  Keycloak) is a reasonable future request; hosting a provider to log in our own users
  is not.
- **Billing, entitlement claims, abuse/DMCA review flows.** Operator-of-strangers
  concerns, not self-hosting ones.
- **App-level blob encryption at rest.** Reclassified as an intentional non-gap — see
  PARITY-GAPS for the reasoning and the Key Vault alternative.
- **A pre-registered OAuth client registry.** remoteStorage is registration-free by
  design.

## Maybe

- **HTTP Range requests (RFC 7233)** — net-new (the Go version lacked it too). Worth it
  only if large-file handling becomes a real complaint.
