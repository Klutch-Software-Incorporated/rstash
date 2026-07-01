# rstash Roadmap

Forward-looking plan for work **after** the Go→C#/.NET rewrite (`rewrite/dotnet`)
merges to `main`. For the detailed, verified list of Go features not yet ported,
see [docs/PARITY-GAPS.md](docs/PARITY-GAPS.md).

## Status

The C# rewrite is functionally complete for core self-hosted remoteStorage use:
storage API (GET/PUT/DELETE/HEAD, implicit folders, ETags, conditionals),
WebFinger, OAuth authorization-code + PKCE **and** implicit grants, the web UI
(setup/login/account/file browser/admin), SQLite + filesystem/database blobs,
per-user **and** global storage **and** egress quotas, the public-write operator
policy, and an admin OAuth test tool. Single-file publish and the container build
work. Tests: unit (Core) + integration (WebApplicationFactory) all green.

## 1. Finalize & merge (now)

- [ ] Full `dotnet test Rstash.slnx` green.
- [ ] Release single-file publish smoke (`/healthz` on the self-contained exe).
- [ ] Tidy the working tree (drop the stray untracked `internal/billing` Go cruft).
- [ ] Merge `rewrite/dotnet` → `main`; retire the long-lived branch.

## 2. Parity catch-up (post-merge, as individual feature branches)

Ranked; details in [docs/PARITY-GAPS.md](docs/PARITY-GAPS.md).

- [ ] **Encryption at rest** (`RSTASH_BLOB_KEY`) — an encrypting blob-store wrapper.
- [ ] **Rate-limit enforcement** — wire the existing `rate_limit_*` / `user_rate_limit_*`
      settings to real middleware (per-IP and per-user).
- [ ] **OAuth refresh-token grant** — `grant_type=refresh_token` with rotation.
- [ ] **Admin JSON API** — API-key-authed `/api/admin/*` for external user/quota management.
- [ ] **Email verification** — verify-email token flow + guard.
- [ ] **Observability** — decide on `/metrics`, structured logging, and OpenTelemetry
      (see the observability follow-up) before building.
- [ ] **Abuse-report flow** — user report form + admin review (DMCA/takedown).

## 3. Backends & deployment (as needed)

- [x] Wire the **Postgres** provider (metadata DB + database blob backend; native Npgsql DSN; Azure Entra ID auth via `Auth=Entra`).
- [ ] Wire the stubbed **MySQL / SQL Server** providers (incl. Entra ID auth; seams in place).
- [x] Wire the **Azure Blob** backend (`azureblob://{account}/{container}`; shared-key / SAS / `DefaultAzureCredential`).
- [ ] Wire the stubbed **S3** backend.
- [ ] Hosting: the rstash.cloud deploy pipeline (`rstash-infra`) / Azure pivot.

## 4. Platform direction (sequenced after parity)

- [ ] **OIDC** — add OpenID Connect as a first-class identity/SSO path.
- [ ] **Billing via OIDC entitlement claims** — billing flows from an external
      billing-plane via OIDC claims (not in-process), sequenced after OIDC.

## Non-goals / deliberately not ported

- HTTP Range requests (not implemented in the Go version either — would be net-new).
- A pre-registered OAuth client registry (remoteStorage is registration-free by design).
- In-process webhooks (superseded by the OIDC-claims billing direction).
