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

- [ ] **OAuth refresh-token grant** — `grant_type=refresh_token` with rotation. Real
      remoteStorage apps expect it; without it, long-lived sessions break.
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

- [ ] **Rate-limit enforcement** — wire the existing `rate_limit_*` settings to real
      per-IP middleware. Matters for anyone exposing rstash to the open internet.
- [ ] **Observability** — decide on `/metrics`, structured logging, and OpenTelemetry
      before building any of it. Currently bare-bones.
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
