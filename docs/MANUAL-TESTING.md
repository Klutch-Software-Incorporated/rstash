# rstash — Manual Testing Playbook

Work through these top to bottom. Each step has a **Do** and an **Expect**. Check the box
when it passes. Commands assume the repo root on Windows (PowerShell). For HTTP calls use
real curl — in PowerShell that's **`curl.exe`** (plain `curl` is an alias for
`Invoke-WebRequest`). On macOS/Linux use `curl`.

Throughout, the server runs at **http://localhost:8080** unless you set `ASPNETCORE_URLS`.

---

## 0. Build & sanity

- [ ] **Do:** `dotnet build Rstash.slnx`
      **Expect:** builds with 0 errors, 0 warnings.
- [ ] **Do:** `dotnet test Rstash.slnx`
      **Expect:** all tests pass (159 unit + 33 integration).

## 1. CLI

- [ ] **Do:** `dotnet run --project src/Rstash.Server -- env`
      **Expect:** a commented env-var template (RSTASH_ADDR, RSTASH_BASE_URL, RSTASH_DB, …).
- [ ] **Do:** `dotnet run --project src/Rstash.Server -- check`
      **Expect:** `[ok] database` + `[ok] blob store` + `Configuration OK.` (exit code 0).
- [ ] **Do:** set `RSTASH_DB=postgres:bogus` then run `check`
      **Expect:** a `[FAIL] database …` line and `Configuration has errors.` (non-zero exit).
- [ ] **Do (after §3 setup):** `dotnet run --project src/Rstash.Server -- seed admin`
      **Expect:** writes a spread of sample modules/folders/files and prints a per-file log
      plus a summary. Omit the username to seed the first account. Browse them in §6.

## 2. Start the server (fresh DB)

- [ ] **Do:** from a clean dir (or delete `rstash*.sqlite`), run
      `dotnet run --project src/Rstash.Server`
      **Expect:** it migrates the DB, loads settings, and listens on :8080.
- [ ] **Do:** `curl.exe http://localhost:8080/healthz`
      **Expect:** `Healthy`.

## 3. First-run setup

- [ ] **Do:** open `http://localhost:8080/` in a browser.
      **Expect:** redirected to **/setup** ("Create the first admin account").
- [ ] **Do:** create an admin (e.g. username `admin`, a strong password, optional email).
      **Expect:** you're signed in and land on the **dashboard**. The cream top bar shows the brand
      and an account pill + sign-out; the left sidebar links **Dashboard**, **Files**,
      **Connected apps**, and (admin) **Settings**, **Users**, **Audit log**.
- [ ] **Do:** refresh `http://localhost:8080/`
      **Expect:** the dashboard renders (no more setup redirect): your storage address, the
      **Usage** card (On disk + Bandwidth meters), and **Connected apps**.

## 4. Auth & account

- [ ] **Do:** click **Sign out**, then visit `/` while signed out.
      **Expect:** sign-out lands on the signed-out view; visiting `/` redirects to a centered,
      themed **sign-in card** at `/login` (brand mark, gold top accent, full-width Sign in button).
- [ ] **Do:** sign back in from that card.
      **Expect:** returns to the dashboard.
- [ ] **Do:** `/login` with a wrong password.
      **Expect:** "Invalid username or password." shown in the card.
- [ ] **Do:** click your username (→ `/account`). Change your password (current + new).
      **Expect:** "Password updated."; you stay signed in. Sign out and back in with the new password.
- [ ] **Do:** on `/account`, set/change your email.
      **Expect:** "Email updated."

### Registration modes (admin → Settings)

- [ ] **Do:** `/admin/settings`, set `registration_mode` = `open`.
      **Do:** in a private window, go to `/register`, create a second user.
      **Expect:** the account is created and signed in.
- [ ] **Do:** set `registration_mode` = `closed`; visit `/register`.
      **Expect:** "Registration is currently closed."
- [ ] **Do:** set `registration_mode` = `approval`; register a user in a private window.
      **Expect:** "pending administrator approval"; that user **cannot** log in yet
      ("This account is pending approval.").

### Forgot / reset password

> Email send requires `RSTASH_EMAIL` (Resend). Without it the link is generated but not delivered.

- [ ] **Do:** `/forgot-password`, enter a username that has an email on file.
      **Expect:** "If a matching account … a reset link has been sent." (With Resend configured,
      the email arrives; the link is `/reset-password?user=…&token=…`.)
- [ ] **Do:** open the reset link, set a new password.
      **Expect:** "Your password has been reset."; sign in with the new password.

## 5. Admin

- [ ] **Do:** as a non-admin user, visit `/admin/settings`.
      **Expect:** bounced to home (`/`).
- [ ] **Do:** as admin, `/admin/settings` → set `site_name` = `My Cloud`, save.
      **Expect:** "Saved 1 setting."; `site_name` shows a **modified** badge; the brand wordmark in
      the top bar and the page titles now read "My Cloud".
- [ ] **Do:** set an invalid value (e.g. `max_upload_size` = `abc`).
      **Expect:** a per-field validation error; the setting is not changed.
- [ ] **Do:** `/admin/users` — approve/disable/enable/delete. Protected: admins (incl. yourself)
      show a **Protected** chip instead of disable/delete.
- [ ] **Do:** on a non-admin user, set **Storage** and **Egress** quotas (e.g. `10MB` / `1GB`) in
      the inline form and click **Save**.
      **Expect:** both fields persist (pre-filled on reload) and the **Storage** + **Egress**
      columns show human-readable values. Clearing a field saves it as `unlimited`.
- [ ] **Do:** `/admin/audit`.
      **Expect:** a table of recent entries (you'll see `storage.put` / `storage.delete` after §7).

## 6. File browser (cookie-authed UI, read-only + delete)

> The browser lists, navigates, downloads, and deletes. It does **not** upload —
> content arrives via the remoteStorage API (§7) or the `rstash seed` command (§1).
> Populate some data first, then come back here.

- [ ] **Do:** **Files** (→ `/files`).
      **Expect:** top-level modules/folders are listed; folders sort first.
- [ ] **Do:** navigate into a folder via its link, then use **Up**.
      **Expect:** navigation works; the path (`/notes/`) updates.
- [ ] **Do:** click a file's name.
      **Expect:** it downloads / opens with the right content.
- [ ] **Do:** click **delete** on a file.
      **Expect:** it disappears from the listing.

## 7. remoteStorage protocol (the API)

### 7a. WebFinger

- [ ] **Do:** `curl.exe "http://localhost:8080/.well-known/webfinger?resource=acct:admin@localhost"`
      **Expect:** JSON (`application/jrd+json`) with `subject` `acct:admin@localhost`, a `storage`
      href `…/storage/admin`, and the `/oauth/authorize` + `/oauth/token` properties.

### 7b. Get a token (implicit flow — easiest by hand)

- [ ] **Do:** in the browser (while signed in as `admin`), open:
      `http://localhost:8080/oauth/authorize?response_type=token&redirect_uri=http://localhost:8080/&scope=*:rw`
      **Expect:** a consent page listing "Read and write all your data". Click **Authorize**.
- [ ] **Do:** after approving you're redirected to `…/#access_token=XXXX&token_type=bearer`.
      Copy the `access_token` value from the URL bar. Save it:
      `$T = "XXXX"` (PowerShell).
      **Expect:** a long hex token.

### 7c. Storage CRUD with the token

- [ ] **Do (PUT):**
      `curl.exe -X PUT -H "Authorization: Bearer $T" -H "Content-Type: text/plain" --data "hello" http://localhost:8080/storage/admin/documents/a.txt -i`
      **Expect:** `201 Created` with an `ETag` header.
- [ ] **Do (GET):**
      `curl.exe -H "Authorization: Bearer $T" http://localhost:8080/storage/admin/documents/a.txt -i`
      **Expect:** `200`, body `hello`, matching `ETag`, `Content-Type: text/plain`.
- [ ] **Do (folder listing):**
      `curl.exe -H "Authorization: Bearer $T" http://localhost:8080/storage/admin/documents/`
      **Expect:** `application/ld+json` with `@context …folder-description` and an `items` map
      containing `a.txt`.
- [ ] **Do (root listing):**
      `curl.exe -H "Authorization: Bearer $T" http://localhost:8080/storage/admin/`
      **Expect:** `items` contains `documents/` (a virtual subfolder).
- [ ] **Do (HEAD):**
      `curl.exe -I -H "Authorization: Bearer $T" http://localhost:8080/storage/admin/documents/a.txt`
      **Expect:** `200` with `ETag` + `Content-Length: 5`, no body.
- [ ] **Do (conditional 304):** take the ETag value `E` from above and
      `curl.exe -H "Authorization: Bearer $T" -H "If-None-Match: $E" http://localhost:8080/storage/admin/documents/a.txt -i`
      **Expect:** `304 Not Modified`.
- [ ] **Do (DELETE):**
      `curl.exe -X DELETE -H "Authorization: Bearer $T" http://localhost:8080/storage/admin/documents/a.txt -i`
      **Expect:** `200`. A subsequent GET → `404`.

### 7d. Auth & scope rules

- [ ] **Do:** GET a document **without** the Authorization header.
      **Expect:** `401` with `WWW-Authenticate: Bearer`.
- [ ] **Do:** mint a token scoped `contacts:rw` (repeat 7b with `scope=contacts:rw`) and PUT to
      `/storage/admin/photos/x.jpg`.
      **Expect:** `403` (insufficient scope).
- [ ] **Do (public read):** with a `*:rw` token, PUT to `/storage/admin/public/notes/p.txt`,
      then GET that same path **without** any token.
      **Expect:** PUT `201`; anonymous GET `200` (public documents are world-readable).
- [ ] **Do:** GET `/storage/nobody/x.txt` with a valid token.
      **Expect:** `404` (unknown user).

### 7e. Authorization-code + PKCE (optional, for real clients)

A real remoteStorage client uses `response_type=code` with PKCE. This is exercised by the
integration test `OAuthTests`. To try by hand you'd compute
`code_challenge = BASE64URL(SHA256(code_verifier))`, hit `/oauth/authorize?response_type=code…`,
approve, then `POST /oauth/token` with `grant_type=authorization_code&code=…&code_verifier=…&redirect_uri=…`.

- [ ] **Do (revoke):** `curl.exe -X POST --data "token=$T" http://localhost:8080/oauth/revoke -i`
      **Expect:** `200`. The token then fails on storage (`401`).

### 7f. Public-write operator policy

- [ ] **Do:** `/admin/settings` (Storage) → set **Public writes** = `disabled`.
- [ ] **Do:** with a `*:rw` token, PUT to `/storage/admin/public/site/x.txt`.
      **Expect:** `403` "public writes are disabled". A DELETE under `/public/` is likewise `403`.
- [ ] **Do:** GET an existing `/public/…` document **without** a token; and PUT to a non-public
      path (e.g. `/storage/admin/notes/y.txt`).
      **Expect:** public reads still `200`; non-public writes still `201`. (Deleting public files
      from the **Files** UI also still works — it bypasses the protocol.)
- [ ] **Do:** set **Public writes** back to `enabled`.

## 8. Quotas & egress

### 8a. Storage quota

- [ ] **Do:** `/admin/users` → set `admin`'s Storage quota to e.g. `20` (bytes) — or use the global
      `total_storage_limit` in `/admin/settings`.
- [ ] **Do:** PUT a body larger than the remaining quota (token with `*:rw`).
      **Expect:** `507 Insufficient Storage`.
- [ ] **Do:** reset the quota to `0` (unlimited) afterward.

### 8b. Egress (bandwidth) quota

- [ ] **Do:** `/admin/users` → set `admin`'s **Egress** quota to a small value (e.g. `20` bytes).
- [ ] **Do:** PUT a document, then GET it repeatedly with a `*:rw` token until the monthly total
      would exceed the cap.
      **Expect:** the GET that would exceed returns `429 Too Many Requests` with a `Retry-After`
      header (seconds until the next UTC month). `HEAD` is **not** charged and still `200`.
- [ ] **Do:** on the dashboard, check the **Usage** card.
      **Expect:** the **Bandwidth · this month** meter reflects the egress consumed; with the cap
      set it shows `used of <cap>`, otherwise a faint "no limit" track. Storage shows likewise.
- [ ] **Do:** reset the egress quota to `0` afterward.

### 8c. New-account defaults

- [ ] **Do:** `/admin/settings` (Storage) → set `default_user_storage_limit` and
      `default_user_egress_limit` (e.g. `2GB` / `5GB`). Register a new user (open mode).
      **Expect:** the new user's **Storage**/**Egress** columns show those defaults.
- [ ] **Do:** change the defaults, then check an **existing** user.
      **Expect:** existing users are unchanged — defaults apply only at account creation.

## 9. Hardening & ops

- [ ] **Do:** `curl.exe -I http://localhost:8080/healthz`
      **Expect:** headers include `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
      `Referrer-Policy: …`, and a `Content-Security-Policy`.
- [ ] **Do:** `curl.exe http://localhost:8080/openapi/v1.json`
      **Expect:** an OpenAPI JSON document.

## 10. UI & mobile

- [ ] **Do:** narrow the browser to a phone width (or use device emulation).
      **Expect:** the left sidebar collapses; a **hamburger** appears in the top bar. Tapping it
      opens a **full-screen** nav drawer (with a ✕ to close); tapping any link closes it.
- [ ] **Do:** on a narrow screen, open `/admin/users` and `/admin/audit`.
      **Expect:** wide tables scroll horizontally **within their card**, not stretching the page.
      The settings tab strip is hidden on mobile (the drawer already links each subpage).
- [ ] **Do:** view `/login` and the dashboard at phone width.
      **Expect:** the sign-in card and the Usage/Connected-apps cards stack and stay readable.

## 11. Single-file & container

- [ ] **Do:**
      `dotnet publish src/Rstash.Server -c Release -r win-x64 --self-contained true -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true -o publish/server`
      then run `./publish/server/Rstash.Server.exe` (set `RSTASH_DB`/`RSTASH_BLOB` to a temp path).
      **Expect:** a single ~120 MB exe; it boots and serves `/healthz` with no SDK installed.
- [ ] **Do (optional, needs Docker):** `docker build -t rstash .` then
      `docker run -p 8080:8080 -v rstash-data:/data rstash`
      **Expect:** image builds; container boots and serves `/healthz`.

---

## Notes / known gaps (deferred, not bugs)

- Egress/bandwidth limits and the public-write operator toggle are now implemented (see §7f, §8b).
- Remaining Go→C# parity gaps (encryption at rest, rate-limit enforcement, refresh-token grant,
  admin JSON API, metrics, email verification, abuse reports, …) are tracked in
  [PARITY-GAPS.md](PARITY-GAPS.md).
- Postgres/MySQL/SQL Server DB providers and S3/Azure blob backends are stubbed (factories throw)
  pending their NuGet packages — SQLite + filesystem + database blobs work today.
- The original Go server lives in `legacy/` for reference; it is not built.
