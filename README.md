<div align="center">

<img src="docs/assets/logo.svg" alt="rstash" width="96" height="96">

# rstash

**Your apps' data, in storage you own.**

[![Release](https://img.shields.io/github/v/release/Klutch-Software-Incorporated/rstash?sort=semver)](https://github.com/Klutch-Software-Incorporated/rstash/releases)
[![CI](https://github.com/Klutch-Software-Incorporated/rstash/actions/workflows/ci.yml/badge.svg)](https://github.com/Klutch-Software-Incorporated/rstash/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Website](https://rstash.cloud) · [Documentation](docs/) · [Roadmap](ROADMAP.md) · [Contributing](CONTRIBUTING.md)

</div>

---

rstash is a [remoteStorage](https://remotestorage.io/) server you run yourself. It is one
binary and a database file, and it gives every remoteStorage app somewhere to keep your
data that isn't somebody else's cloud.

Think of the part of Dropbox or iCloud that apps quietly sync to in the background — the
notes, the bookmarks, the task lists. remoteStorage is an open protocol for exactly that,
and rstash is the server end of it. Apps ask your permission, you grant them a folder,
and the data stays on your machine.

<!-- ASSET: hero.gif — 15–25s loop, ~1200px wide: first-run setup → file browser → an app
     asking for permission on the consent screen. Drop at docs/assets/hero.gif and
     uncomment. -->
<!-- <div align="center"><img src="docs/assets/hero.gif" alt="Setting up rstash and connecting an app" width="800"></div> -->

## Why rstash

- **You keep the data.** No account with us, no telemetry, no tier where your files get
  held hostage. rstash has no hosted version and [isn't getting one](ROADMAP.md#non-goals).
- **No lock-in, by protocol.** remoteStorage is a published spec
  ([draft-dejong-remotestorage-26](https://datatracker.ietf.org/doc/html/draft-dejong-remotestorage-26)).
  Any compliant app works with rstash, and your data moves to any other compliant server.
- **Small enough to actually run.** A single self-contained binary against a SQLite file.
  No Redis, no Elasticsearch, no job queue. Point it at Postgres and S3-style object
  storage instead when you outgrow that.
- **Built for a household, not a datacenter.** Per-user and server-wide quotas, an admin
  UI for users and settings, and an audit log — sized for you, your family, and a few
  friends.

## Apps that work with it

remoteStorage apps are written against the spec, not against rstash, so anything
compliant should connect.

<!-- TODO(interop): fill in from the recorded compatibility pass in ROADMAP §1 before
     publishing this section. Do not list an app here until someone has actually
     connected it to rstash and read and written data. -->

*A verified compatibility list is coming — see the interop item on the
[roadmap](ROADMAP.md). Until then, please
[tell us what you connected](https://github.com/Klutch-Software-Incorporated/rstash/issues/new/choose),
working or not.*

## Get started

**Download a binary**

```sh
curl -LO https://github.com/Klutch-Software-Incorporated/rstash/releases/latest/download/rstash-linux-x64
chmod +x rstash-linux-x64
./rstash-linux-x64
```

Builds are published for Linux (x64, arm64), macOS (x64, arm64), and Windows (x64).

**Or run it in Docker**

There's no published image yet, so build it from the repository:

```sh
git clone https://github.com/Klutch-Software-Incorporated/rstash.git && cd rstash
docker build -t rstash .
docker run -d --name rstash -p 8080:8080 -v rstash-data:/data \
  -e RSTASH_BASE_URL=http://localhost:8080 rstash
```

Then open **http://localhost:8080**. While no account exists, every page redirects to a
setup wizard that creates the first admin; after that you sign in and everything else
happens in the web UI.

Before anyone else uses it, put it behind a reverse proxy that terminates TLS, and set
`RSTASH_BASE_URL` to the public `https://` address with `RSTASH_TRUST_PROXY=true`.
rstash speaks plain HTTP itself. Full details in
**[docs/CONFIGURATION.md](docs/CONFIGURATION.md)**.

## What it looks like

<!-- ASSET: three stills, ~1000px wide, light theme, seeded demo data.
     1. files.png    — the file browser at /files showing the seeded modules
     2. consent.png  — the OAuth consent screen at /oauth/authorize
     3. admin.png    — admin settings or the user list
     Drop in docs/assets/ and uncomment. -->
<!--
| Your files | Granting an app access | Running the server |
|---|---|---|
| <img src="docs/assets/files.png" alt="File browser"> | <img src="docs/assets/consent.png" alt="App consent screen"> | <img src="docs/assets/admin.png" alt="Admin settings"> |
-->

## Status

The protocol surface is complete and in daily use: documents and folders over
`GET`/`PUT`/`DELETE`/`HEAD`, ETags and conditional requests, JSON-LD folder listings,
WebFinger discovery, and the OAuth app-authorization flow with PKCE and refresh tokens.
Storage runs on SQLite or Postgres, with blobs on disk, in the database, or in Azure Blob
Storage.

It is pre-1.0, which is the honest signal: configuration, schema, and defaults can still
move under you between releases. Direct HTTPS, S3-compatible blobs, and an admin JSON API
are the notable things not built yet — [ROADMAP.md](ROADMAP.md) has the ordering, and the
things deliberately not planned.

## Documentation

- **[Configuration](docs/CONFIGURATION.md)** — environment variables, database and blob
  DSNs, email, TLS, the CLI
- **[Identity & authorization](docs/IDENTITY.md)** — how signing in differs from the
  tokens handed to apps
- **[Roadmap](ROADMAP.md)** — what's next, and what is deliberately out of scope
- **[Parity gaps](docs/PARITY-GAPS.md)** — what the Go original did that this doesn't, yet

## Contributing

You don't have to write C# to help. Running rstash and reporting what broke, connecting
an app and telling us whether it worked, or fixing a paragraph that misled you are all
genuinely useful — and the compatibility list above is waiting on exactly that.

If you do want to write code: you need the [.NET 10 SDK](https://dotnet.microsoft.com/download),
and then `dotnet build Rstash.slnx` and `dotnet test Rstash.slnx` should both be clean on
a fresh clone. [CONTRIBUTING.md](CONTRIBUTING.md) covers the workflow, and
[CLAUDE.md](CLAUDE.md) has the architecture and the conventions worth knowing before you
change anything.

rstash is written in C# on .NET 10. It was written in Go first and rewritten in mid-2026;
the Go tree is preserved at the `go-final` tag.

## License

MIT. See [LICENSE](LICENSE).
