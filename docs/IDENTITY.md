# Identity & Authorization

rstash has two separate things that hand out credentials. They live at similar URLs
and both say "OAuth" somewhere, so the distinction is worth stating plainly.

| | **Sign-in** | **App authorization** |
|---|---|---|
| Who is being authorized | a human | a third-party app (Litewrite, etc.) |
| Mechanism | ASP.NET Core Identity + cookie | remoteStorage's OAuth profile |
| Credential | `rstash.session` cookie | opaque bearer token (`TokenStore`) |
| Routes | `/login`, `/register`, `/setup` | `/oauth/*` |

They meet at exactly one seam: the app-consent screen can only be shown to someone
already signed in, so app authorization depends on sign-in having happened.

## Sign-in

A username and a password, checked by `SignInManager.PasswordSignInAsync`, setting a
cookie. That is the whole design. Identity owns password hashing, lockout, and the
token providers behind email verification and password reset.

Every account is one `ApplicationUser` row, quotas included. rstash owns its users
outright, so there is nothing to keep a second table in sync with.

## App authorization

The remoteStorage spec requires the *storage server* to issue app tokens — WebFinger
advertises its auth endpoint, and the scope dialect (`documents:rw`) is specific to
remoteStorage. So this is rstash's own lightweight authorization server, and it is not
replaceable by a general-purpose identity provider. PKCE S256 on the code flow.

## What was tried and reversed

Between July 2026 and this document, rstash authenticated humans by redirecting to an
OpenID Connect provider that rstash itself hosted (OpenIddict at `/connect/*`). The
password form set an Identity cookie, the app then bounced through its own authorize
endpoint, ran a PKCE code exchange against itself, and set a *second* cookie.

The goal was a single claims-shaped path into the user row, so that a hosted
multi-tenant deployment could swap the bundled provider for an external control plane
without touching anything downstream. It worked. It also cost:

- Two cookie schemes, because collapsing them looped the challenge forever.
- Hand-tuned `ResponseMode` and `SameSite`/`Secure` policies on the correlation and
  nonce cookies, because the handler's defaults assume `form_post` over HTTPS and
  rstash runs on plain HTTP locally and behind a TLS-terminating proxy.
- A `storage_users` table split from `AspNetUsers`, an `IEntitlementSource`
  indirection, a JIT provisioning path, persisted signing keys, and four OpenIddict
  tables.
- An integration-test harness that looped the OIDC back channel through `TestServer`,
  because there is no socket to reach in-process.

It was removed in August 2026 along with the hosted-multi-tenant direction it existed
to serve. Roughly 2,500 lines went with it.

**If external SSO is ever wanted** — and it is a reasonable self-hoster request, for
Authelia or Keycloak or Authentik — the thing to add is an *optional* OIDC login
alongside the password form. What should not come back is rstash hosting a provider
for itself in order to log its own users in.
