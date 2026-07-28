# External Identity & Entitlements (OIDC) — Design

Status: **design only, not implemented.** Written 2026-07-13 to settle the
architecture before it constrains adjacent work (rate limiting, the OAuth
refresh grant, the admin API).

Supersedes the billing direction in the older `saas-roadmap` (in-process Stripe)
and the June 2026 in-process-Polar decision. Neither is current.

## Goal

Two deployment modes from one binary:

- **Self-hosted (default).** No external anything. Local ASP.NET Core Identity,
  cookie auth, local registration — exactly what ships today. A self-hoster runs
  `rstash` and never hears the word OIDC.
- **Hosted (rstash.cloud).** An external **control plane** owns user management:
  identity, plans, quotas, and revocation on past-due. rstash authenticates users
  against it via OIDC and enforces the limits it is told to enforce.

The control plane is a separate project (e.g. a billing subdomain). rstash is a
**relying party only** — it consumes OIDC and never becomes an OIDC provider.

## Decision 1 — rstash is *always* an OIDC relying party; the IdP is embedded or external

rstash speaks exactly one login protocol: OIDC, as a relying party. The only
question is where the provider lives.

```
                      ┌─ embedded IdP (self-host default) — OpenIddict over ASP.NET Identity
rstash (always RP) ───┤
                      └─ external IdP (rstash.cloud)      — the control plane
```

Selected at boot from config (`RSTASH_OIDC` unset = embedded). Everything
downstream of authentication — storage, quotas, audit — sees an authenticated
`ApplicationUser` and does not care which provider issued the claims.

**Why embedded rather than "local mode just uses Identity directly":** one login
code path, and — more importantly — **one shape for entitlements**. Claims are
always the transport into the user row, so quota/plan/rate enforcement has a
single upstream regardless of deployment. The alternative (local Identity in one
mode, OIDC in the other) means every entitlement consumer has two sources.

**What this honestly does *not* buy.** It unifies *login*, not *user management*.
Registration, password reset, and admin user CRUD still exist in embedded mode
and still must be hidden in external mode — that branch is about *who owns the
users*, not which protocol carries the login. See Ramifications; those conditionals
survive this decision. Accepted with eyes open.

### Two token-issuing subsystems will coexist — do not merge them

This is the single most important thing to keep straight, because both live at
OAuth-shaped URLs and both hand out tokens, but they authorize *different subjects*:

| | **Identity** (new) | **remoteStorage app-authorization** (already built) |
|---|---|---|
| Who is authorized | a **human** | a **third-party app** (Litewrite, etc.) |
| Protocol | OIDC | remoteStorage's own OAuth profile (spec-defined) |
| Token | ID token (signed, claims) | opaque bearer token (`TokenStore`) |
| Consent screen | "log in" | "Litewrite wants `documents:rw`" |
| Provider | bundled *or* external | **always rstash** — the spec requires the storage server to issue these |
| Routes | `/connect/*` (suggested) | `/oauth/*` (existing, unchanged) |

The remoteStorage AS is **not** replaceable by the external provider: a generic
IdP doesn't understand rS scope dialect, and WebFinger advertises the storage
server's own auth endpoint. It stays exactly as it is.

They touch at **one** seam: the app-consent screen can only be shown to a
logged-in human, so app-authorization depends on identity having happened first.

### Costs this decision takes on

- **Signing-key persistence.** ID tokens are signed; the key must persist across
  restarts (DB-backed) and rotate. *Lower stakes than it first appears:* login
  yields a normal auth cookie, so a regenerated signing key breaks only in-flight
  logins, not established sessions. Do not use OpenIddict's ephemeral dev key in
  production, but this is not a crisis.
- **The key that actually matters is Data Protection** — it encrypts the auth
  cookie and antiforgery tokens. It is **not persisted today** (no `PersistKeysTo`
  anywhere in `src/`), so in the container deploy every restart silently logs out
  every user. This is a **live bug, independent of OIDC**; fix it separately.
- **`RSTASH_BASE_URL` becomes the OIDC issuer.** A wrong value then breaks token
  validation, not just link cosmetics. `rstash check` should validate it.
- **Behind a reverse proxy, derive URLs from `RSTASH_BASE_URL`, never from the
  incoming request.** TLS-terminating proxies make the app believe it lives at
  `http://localhost:8080` while users are at `https://…`, which corrupts both the
  issuer and the redirect URI. `ForwardedHeaders` is also unconfigured today. One
  rule plus one middleware — a chore, not a swamp.

## Decision 2 — split the identity tables from the storage-side user record

**The most consequential decision here, and the cheapest one to get right now,
because it is a schema decision.**

The storage server keeps its own user record, **separate from the identity
provider's tables, even when both live in the same database in bundled mode.**

```
identity provider (bundled OR external)     remoteStorage server (always rstash)
├── accounts, credentials, password hashes   └── StorageUser
├── login / logout / forgot-password             ├── Subject   (the IdP's `sub` — the join key)
├── email verification                           ├── UserName  (the remoteStorage identity)
└── plan / billing (hosted only)                 │
                                                 ├── Entitlements ─ CACHED FROM CLAIMS.
                                                 │   │             Provider-owned. rstash
                                                 │   │             never authors or edits.
                                                 │   ├── MaxStorage, MaxEgress, RateLimit
                                                 │   ├── Plan, Disabled
                                                 │   └── SyncedAt, SourceIssuer  (provenance)
                                                 │
                                                 └── Usage ─────── rstash owns and observes.
                                                     ├── BytesStored    The provider never
                                                     └── EgressThisMonth sees these.
```

**Enforcement compares `Usage` against `Entitlements`.** The provider says what
you're *allowed*; rstash knows what you've *used*.

### Why `Entitlements` must be materialized locally, not read from claims

This is the crux of the whole design, and it is not a preference — it is forced.

When Litewrite sends `PUT /storage/curtis/documents/note.txt` at 3am, the request
carries **a storage bearer token and nothing else**. No human, no browser, no
session, no ID token. There is no claim to read. If the limit existed only as a
claim, rstash would have a usage number and nothing to compare it against.

The alternatives all collapse:

| Approach | Why it fails |
|---|---|
| Call the provider per storage request | Network round-trip per PUT; provider down ⇒ storage down. |
| Cache the provider's answer | This *is* local materialization. It's the actual design — the only open question was the refresh policy. |
| Put limits in the storage token | Rejected (Decision 3): stale on downgrade, and it's the app's token, not the human's. |

So `Entitlements` is a **materialized view of the last claims we saw**, stamped
with `SyncedAt` / `SourceIssuer` so staleness is visible and auditable. The naming
exists to make it impossible to mistake these for rstash-owned fields: in external
mode they are never user-editable and never admin-editable.

Refresh policy is Decision 4: **claims at web login** (routine), **admin API push**
(immediate revocation), **periodic reconcile** (backstop).

Why the storage side must have a record at all: the schema is keyed on a local
user id (`Node.OwnerId`, `OAuthToken.UserId`, `AuditEntry.ActorId`) and the
*protocol* is keyed on a local username (`acct:user@host` in WebFinger,
`/storage/{user}/…` in every request). Zero rows means you cannot serve a document.

Why it must be a **separate** record from the Identity tables: in bundled mode it
is tempting to hang `StorageQuota` off the same `ApplicationUser` row that holds
`PasswordHash`. Do that, and the day you switch to an external provider that table
has to be torn in half under a running system, and every storage-side query that
touched a provider-owned column becomes a migration. Keep them separate from day
one and **bundled and external mode run the same storage code** — swapping the
provider is a config change, not surgery.

So:

- **Bundled provider:** ASP.NET Identity owns accounts + credentials. `StorageUser`
  is created alongside, keyed on the local `sub`. Two tables, one database.
- **External provider:** there are no Identity tables at all — no accounts, no
  credentials, no local lifecycle. `StorageUser` is provisioned **JIT on first
  login** from claims. *This* is what "rstash keeps no local users" means, and it
  is now literally true of the storage server.

`StorageUser`'s entitlement fields are a **cache, never a source of truth**, in
external mode: never user-editable, and admin user-management screens go read-only.

### The username is the hard part

`sub` is the durable join key, but `UserName` is what appears in storage URLs and
WebFinger. It therefore cannot change after any data exists, and it must be
unique across the instance. Constraints to enforce at provisioning:

- Reject a login whose `preferred_username` collides with a different `sub`.

### The username is the hard part

`sub` is the durable join key, but `UserName` is what appears in storage URLs and
WebFinger. It therefore cannot change after any data exists, and it must be
unique across the instance. Constraints to enforce at provisioning:

- Reject a login whose `preferred_username` collides with a different `sub`.
- Treat username as immutable once the shadow row exists; if the IdP changes it,
  keep ours and log the divergence. Renaming a user is a data migration
  (every node path is namespaced by owner), not a claim update.
- The control plane must apply rstash's username validity rules at signup, or
  we bounce users at first login. **This is a contract between the two projects
  and should be specified before either ships.**

## Decision 3 — storage tokens stay opaque, even under scale-out

The remoteStorage spec (draft-26) does **not** constrain token format — it asks
only that tokens be hard to guess and not reused across clients, and is silent on
refresh entirely. Opaque vs. JWT is our call.

We keep them opaque (32-byte random, DB lookup — `TokenStore`), and we do **not**
put entitlement claims in them.

Rationale:

- The payoff of a JWT is stateless validation. But every storage request already
  hits the database for node metadata, ETag, and quota accounting. Skipping the
  token lookup saves a fraction of a query on a request that is going to the DB
  regardless. There is no meaningful win.
- **Scale-out does not change this.** Multiple instances share one database (they
  must — that's where the nodes live), so a DB-backed token lookup works fine
  horizontally. Statelessness would only matter if instances had no shared store,
  which is not a reachable state for this app.
- Entitlement in a bearer token goes **stale**. remoteStorage tokens are held by
  third-party apps for long periods; a token minted while a user was "pro" would
  keep asserting pro after they lapsed. Entitlement must be resolved from current
  user state at request time.
- Opaque tokens make `/oauth/revoke` a row delete. JWTs would need a denylist —
  reintroducing the very lookup they were meant to avoid.

### Claims go on the *identity* token, never the storage token

Entitlement rides claims — but on the human's ID token, not the app's storage
token. The reason is mechanical: when Litewrite issues
`PUT /storage/curtis/documents/note.txt` at 3am, the request carries **only** a
storage bearer token. The human isn't there, the browser isn't involved, and no
ID token is anywhere near it. There are no claims to read at enforcement time.

So the claim is the **pipe**, and the `StorageUser` row is the **source of truth
at enforcement time**:

```
control plane knows the plan
  → ships `storage_quota` as a claim on the ID token
    → rstash reads it at web login
      → writes it onto StorageUser
        → the 3am quota check reads StorageUser
```

### Claims carry *limits*, never *usage*

| Claims (from the provider) | Server-side state (rstash owns) |
|---|---|
| max storage, max egress, rate limit | bytes stored, egress consumed this month |
| plan tier, enabled/disabled | `EgressUsage`, node sizes |

Only rstash can observe usage, so only rstash tracks it. The provider says what
you're *allowed*; rstash knows what you've *used*; enforcement compares the two.

This is also why the claim→record sync is not fragile: limits change rarely (an
upgrade, a revocation), so a sync at login plus a push channel for revocation is
entirely adequate. No claim needs refreshing on a timer.

## Decision 4 — entitlement propagation is push-primary

Login-time claims alone are insufficient: a user who lapses may not log into the
rstash *web UI* for months while their remoteStorage apps keep using a valid
storage token. Revocation would never land.

Three channels, in order of authority:

1. **Push (primary).** Control plane → rstash **admin JSON API** (API-key authed):
   set quota, set plan, disable/enable, delete. Takes effect immediately.
   *This makes the already-planned Admin API a hard prerequisite for the hosted
   billing story, not an independent roadmap item.*
2. **Login-time claim sync.** Cheap correction on every OIDC login. Handles drift.
3. **Periodic reconcile (backstop).** A background job re-reads entitlements for
   active users, so a dropped webhook self-heals instead of silently granting
   free service forever.

All three write the same shadow-user fields. All three must write **audit
entries** — with the control plane as actor — which is currently impossible
(see Ramifications).

### Bounding the revocation window

Even with push, a disabled user's *already-issued* storage token must stop
working. Two enforcement points:

- **Every storage request** already resolves the owner; check `Disabled` there.
  This closes the window to zero and is the real answer.
- **Short-lived access tokens + the refresh grant** bound it structurally: the
  refresh call is where rstash re-checks user state, so a revoked user's app dies
  at next refresh even if the per-request check were ever bypassed.

This is an independent argument *for* building the refresh grant, beyond the
"the setting lies" argument. It is the lifecycle hook for a long-lived
third-party token.

## Decision 5 — rate limiting is per-user override over a global setting

```
limit(user) = user.RateLimitOverride ?? settings.UserRateLimit
```

- **Self-host:** override is always null; the admin's global setting governs.
  Behaves exactly like the existing `rate_limit_*` settings promise.
- **Hosted:** the control plane writes the per-user override (via the admin API,
  same channel as quotas). The global setting becomes the default for anyone
  unclassified.

This mirrors how `StorageQuota` / `EgressQuota` already work — per-user column,
0 = fall back to unlimited. **Build the limiter against a resolver, not a global
constant.** That is the one thing that must be right before the limiter ships;
everything else about OIDC can land later without touching it.

Scale-out note: the token-bucket state should sit behind an interface. In-memory
per-instance is correct for a single instance and *approximately* correct for a
small fleet (each instance enforces 1/N). A shared store is a later swap, not a
redesign — do not build it now.

## Ramifications for existing features

Most of these are **not** "branch on OIDC mode" — under Decision 1 they *move to
the identity provider*, which in bundled mode still ships in the binary. The
distinction matters: the code doesn't get conditionals, it gets relocated behind
a boundary, and in external mode it simply isn't reached.

Things that assume a local account, and therefore belong to the provider:

- **First-run setup flow.** The setup guard redirects to `/setup` when no users
  exist and creates a local admin. In OIDC mode there is no local admin to create
  and no first user — setup must be skipped entirely, and admin-ness comes from a
  claim. `SetupState` needs to be mode-aware.
- **Registration, login, password reset, email verification.** All local-mode-only.
  The control plane owns them in hosted mode. Notably this makes the outstanding
  **email-verification parity gap** hosted-irrelevant — it only matters for
  self-hosters, which lowers its priority.
- **Account page.** Email/password/username editing must be hidden in OIDC mode.
- **Admin user management.** Read-only in OIDC mode; mutations belong to the
  control plane.
- **Audit log.** Needs an actor that is not a user id (the control plane, acting
  via API key), and needs to actually record admin/auth/OAuth events — today it
  records only `storage.put` and `storage.delete`. Control-plane-driven quota and
  revocation changes are exactly the events an audit log exists for.
- **`rstash seed` / `rstash check`.** Resolve users by username; behavior in a
  no-local-users deployment needs a decision.
- **Unaffected:** the storage API, WebFinger, `/oauth/*` (app authorization),
  public paths, ETags, folder semantics. The whole remoteStorage surface is
  indifferent to how the human logged in.

## Sequencing

Nothing here forces OIDC to be built now. The dependencies point the other way —
these are prerequisites for it, and each stands on its own merit:

1. **Rate limiting** — build with the per-user resolver (Decision 5). Currently a
   setting that lies.
2. **OAuth refresh grant** — the token-lifecycle hook (Decision 4). Currently a
   setting that lies.
3. **Audit coverage + actor model** — so control-plane actions are recordable.
4. **Admin JSON API + API-key auth** — the push channel. Promoted from
   "nice-to-have" to *prerequisite* by Decision 4.
5. **OIDC RP + embedded IdP** — the biggest single step, and it is worth doing in
   two passes:
   - **5a. Embedded IdP (OpenIddict over Identity) + rstash as RP against itself.**
     Ships with *zero* external dependency and is independently verifiable: a
     self-hoster logs in, and the login now happens to be an OIDC round-trip.
     Carries the signing-key and issuer work from Decision 1.
   - **5b. External IdP mode.** Swap the provider, add JIT shadow provisioning
     and claim sync. By this point the RP side is already proven by 5a.

Steps 1–4 are all things the codebase already advertises or has queued. OIDC does
not add work to them; it constrains their shape. That constraint is the point of
this document.

Note that 5a is the step that *earns* Decision 1: if the embedded IdP turns out to
be a swamp (key management, redirect handling in the single-binary case), the
fallback is local Identity in embedded mode and OIDC only for external — which
costs a second entitlement source and the branching Decision 1 was meant to avoid,
but is not fatal. Treat 5a as the go/no-go on always-RP.

## Open questions for the control-plane project

- Username rules must match rstash's, enforced at signup (see Decision 2).
- Claim names for entitlements (`storage_quota`, `egress_quota`, `rate_tier`,
  `plan`) — needs a fixed schema both sides agree on.
- Does deleting a user in the control plane delete their *data* in rstash, or
  disable and retain? (Retention/GDPR question, not a technical one.)
- Does the control plane need to read usage back out of rstash (for metered
  billing or a usage display)? That would make the admin API bidirectional.
