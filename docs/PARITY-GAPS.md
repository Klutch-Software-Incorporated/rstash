# Parity Gaps — Go → C# Rewrite

Capabilities present in the legacy Go server (`legacy/`) that are **missing or
stubbed** in the C# rewrite, to be tackled as individual features after the
`rewrite/dotnet` branch merges. Verified against the codebase on 2026-06-30.

> The C# rewrite is functionally complete for core remoteStorage use (storage
> API, WebFinger, OAuth authorization-code + PKCE, the web UI, SQLite +
> filesystem/database blobs, storage **and** egress quotas, public-write policy).
> The items below are the deltas, not blockers for personal/family use.

## Tier 1 — closed (August 2026)

Both items here were settings that described behavior the code did not have.

Closed: **rate limiting** is now enforced (August 2026) via ASP.NET's built-in
limiter — strict per-IP on password endpoints, per-account on the storage API,
per-IP elsewhere, with static assets and `/healthz` exempt. Identity's account
lockout, previously switched off at the call site, is on as well.

Closed: the **OAuth refresh-token grant** is now implemented (August 2026) —
`grant_type=refresh_token`, always on, rotating both secrets on each use, and
withheld from the implicit flow. The `refresh_tokens` on/off setting was deleted
rather than wired, since a self-hoster gains nothing by turning it off.

## Tier 2 — known/documented deferrals

- **Admin JSON API + API-key auth.** Go: `/api/admin/*` (`legacy/internal/api/admin.go`,
  `apikey.go`). C# has cookie-UI admin only. (Roadmap: "Admin API — next up.")
- **MySQL / SQL Server database providers** and the **S3 blob backend** — stubbed in
  C# (factories throw); the wiring seams are in place. (**Postgres** is now wired —
  native Npgsql DSN + Azure Entra ID auth; see below. **Azure Blob** is wired —
  `azureblob://{account}/{container}` with shared-key/SAS/`DefaultAzureCredential` auth.
  SQLite + filesystem/database work.)
- **`/metrics` + observability.** The `metrics_mode` setting references a
  `/metrics` endpoint that isn't wired; no OpenTelemetry.
- **Email verification** (verify-email token + AccountGuard). Forgot/reset
  password *are* ported; verification is not.
- **Abuse-report flow** (DMCA/takedown). Go: `legacy/internal/web/abuse.go`.

## Tier 3 — niche / Azure-specific

- **Entra ID Postgres auth** (`legacy/internal/db/entra_postgres.go`) — ported: append
  `Auth=Entra` to a native `postgres:` DSN; the password is a `DefaultAzureCredential`
  access token (periodic-refresh data source at runtime, one-shot token for migrations).
- **`encrypt-existing` CLI** and the `rs-upload` helper tools — auxiliary;
  `encrypt-existing` only matters once encryption at rest lands.
- **Bundled Swagger UI page.** C# serves the OpenAPI spec at `/openapi/v1.json`
  but doesn't bundle an interactive UI like Go did.

## Intentional / non-gaps (do not port without a reason)

- **App-level blob encryption at rest (`RSTASH_BLOB_KEY`).** Go wrapped blobs in
  an app-managed AES store (`legacy/internal/blob/encrypted.go`, `keyprovider.go`).
  Deliberately **not** ported (decision 2026-07-06): it only defends a "leaked
  storage credential *without* the app env" seam that the single-container hosted
  topology doesn't have, and it carries an unrotatable-key / lose-the-key-lose-all-
  data footgun. Azure Storage already encrypts every blob at rest (AES-256) by
  default, covering the physical-media threat for free. If app-managed keys are ever
  required (e.g. an honest "we hold the encryption keys" claim), use a Key Vault
  customer-managed key — infra-only, with real rotation — not an app-held key.
  `StorageFactory`'s docstring now says the same.
- **Range requests (RFC 7233).** Not implemented in *either* version (Go lacks it
  too); listed as planned, not a regression.
- **OAuth client registry.** remoteStorage is registration-free by design
  (client_id == redirect origin); Go's registry was bookkeeping, not a spec need.
- **Webhooks** (`legacy/internal/webhooks/`). Existed to drive billing/integration for
  a hosted offering; that direction was dropped (see ROADMAP non-goals).
