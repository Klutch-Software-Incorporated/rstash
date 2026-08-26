# rstash Roadmap

rstash is a remoteStorage server for people who want to run their own, for themselves
or for a handful of family and friends. Everything below is judged against that: does
it make the thing better to *self-host*?

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
- [x] **TLS from operator-supplied certificates** — `tls_mode=files` serves HTTPS from a PEM
      certificate and key, reloading in place when an external renewer replaces them, so
      renewal never needs a restart. `off` remains the default, leaving localhost and
      behind-a-proxy deployments unchanged. No new dependency.
- [ ] **TLS via ACME** — deferred (August 2026), not dropped. .NET ships no ACME client and
      Microsoft maintains none; LettuceEncrypt was archived in April 2025. The credible
      candidate is the LettuceEncrypt-Archon fork — net10.0, released August 2026 — but it is
      one company's fork and its ARI (RFC 9773) support is unconfirmed. That matters more
      every year: Let's Encrypt's `tlsserver` profile is already 45 days, and the CA/Browser
      Forum ceiling drops to 100 days in March 2027 and 47 in March 2029, so an unmaintained
      ACME client becomes an outage on a timer rather than a stale dependency. Revisit when
      the fork's renewal story is provable. Derive the hostname from `RSTASH_BASE_URL` rather
      than adding a `tls_domains` setting, and note it needs a port-80 listener — the same one
      an HTTP→HTTPS redirect wants, which is why neither exists yet.
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
  every self-hoster downloads. It was tried: an embedded OIDC provider, a second user
  table, and an entitlement indirection, all removed again at a cost of ~2,500 lines.
- **rstash as its own OpenID Connect provider.** Optional *external* SSO (Authelia,
  Keycloak) is a reasonable future request; hosting a provider to log in our own users
  is not.
- **Billing, entitlement claims, abuse/DMCA review flows.** Operator-of-strangers
  concerns, not self-hosting ones.
- **App-level blob encryption at rest.** Decided against (July 2026). An app-held key
  only defends one narrow seam — a leaked storage credential *without* the application
  environment — and buys it with an unrotatable key that loses every file if mislaid.
  Object stores already encrypt at rest, which covers the physical-media threat. If
  app-managed keys are ever genuinely required, the answer is an infrastructure-level
  customer-managed key with real rotation, not a key held by rstash.
- **A pre-registered OAuth client registry.** remoteStorage is registration-free by
  design.

## Maybe

- **HTTP Range requests (RFC 7233)** — net-new (the Go version lacked it too). Worth it
  only if large-file handling becomes a real complaint.
