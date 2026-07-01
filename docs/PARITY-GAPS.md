# Parity Gaps — Go → C# Rewrite

Capabilities present in the legacy Go server (`legacy/`) that are **missing or
stubbed** in the C# rewrite, to be tackled as individual features after the
`rewrite/dotnet` branch merges. Verified against the codebase on 2026-06-30.

> The C# rewrite is functionally complete for core remoteStorage use (storage
> API, WebFinger, OAuth authorization-code + PKCE, the web UI, SQLite +
> filesystem/database blobs, storage **and** egress quotas, public-write policy).
> The items below are the deltas, not blockers for personal/family use.

## Tier 1 — impactful and currently misrepresented (a setting/comment implies they exist)

- **Encryption at rest (`RSTASH_BLOB_KEY`).** Go wraps blobs in an encrypted
  store with a key provider (`legacy/internal/blob/encrypted.go`,
  `keyprovider.go`). C# only has a "layered on later" comment in
  `src/Rstash.Storage/StorageFactory.cs`. No encryption today.
- **Rate limiting enforcement.** Go enforces per-IP and per-user token buckets
  (`legacy/internal/api/ratelimit.go`). C# defines `rate_limit_*` /
  `user_rate_limit_*` settings but never reads or enforces them.
- **OAuth refresh-token grant.** Go rotates refresh tokens
  (`legacy/internal/api/token.go`). C# rejects every grant but
  `authorization_code` (`OAuthEndpoints.cs`), despite the `refresh_tokens` setting.

## Tier 2 — known/documented deferrals

- **Admin JSON API + API-key auth.** Go: `/api/admin/*` (`legacy/internal/api/admin.go`,
  `apikey.go`). C# has cookie-UI admin only. (Roadmap: "Admin API — next up.")
- **Multi-provider databases** (Postgres/MySQL/SQL Server) and the **S3 blob
  backend** — stubbed in C# (factories throw); SQLite + filesystem/database work.
  (**Azure Blob** is now wired — `azureblob://{account}/{container}` with
  shared-key/SAS/`DefaultAzureCredential` auth.)
- **`/metrics` + observability.** The `metrics_mode` setting references a
  `/metrics` endpoint that isn't wired; no OpenTelemetry.
- **Email verification** (verify-email token + AccountGuard). Forgot/reset
  password *are* ported; verification is not.
- **Abuse-report flow** (DMCA/takedown). Go: `legacy/internal/web/abuse.go`.

## Tier 3 — niche / Azure-specific

- **Entra ID Postgres auth** (`legacy/internal/db/entra_postgres.go`) — moot until
  Postgres is wired.
- **`encrypt-existing` CLI** and the `rs-upload` helper tools — auxiliary;
  `encrypt-existing` only matters once encryption at rest lands.
- **Bundled Swagger UI page.** C# serves the OpenAPI spec at `/openapi/v1.json`
  but doesn't bundle an interactive UI like Go did.

## Intentional / non-gaps (do not port without a reason)

- **Range requests (RFC 7233).** Not implemented in *either* version (Go lacks it
  too); listed as planned, not a regression.
- **OAuth client registry.** remoteStorage is registration-free by design
  (client_id == redirect origin); Go's registry was bookkeeping, not a spec need.
- **Webhooks** (`legacy/internal/webhooks/`). Superseded by the roadmap's move to
  OIDC entitlement claims for billing/integration.
