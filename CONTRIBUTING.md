# Contributing to rstash

## Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/) (task runner)
- Git

## Contribution workflow

rstash is developed on [GitHub](https://github.com/Klutch-Software-Incorporated/rstash). To contribute:

1. Fork the repository and clone your fork.
2. Create a branch off `main` for your change.
3. Make your change; run `task fmt`, `task vet`, and `task test` before pushing.
4. Open a pull request against `main`. CI (build, vet, test, secret scan) runs automatically on every PR.

A maintainer will review; once CI is green and the change is approved, it'll be merged.

## Versioning & Releases

rstash uses [Semantic Versioning](https://semver.org/). It's pre-1.0, so the
`0.` prefix is itself the signal that the config, database schema, and protocol
surfaces are still stabilizing.

Categorize a change by the worst thing it does to an **existing deployment on
upgrade**:

| Bump (pre-1.0)  | When | Examples |
|---|---|---|
| **breaking** → `0.(y+1).0` | Not backward-compatible | non-reversible `AutoMigrate` change (column drop/rename); removing/renaming an `RSTASH_*` var or changing a default's behavior; breaking the remoteStorage protocol or admin API; dropping a DSN/config format |
| **feature** → `0.y.(z+1)` | Backward-compatible addition | new blob backend, new endpoint, new *optional* setting, new CLI subcommand |
| **fix** → `0.y.(z+1)` | Backward-compatible fix | bugfix, security patch, dependency bump, docs, perf, internal refactor |

Label each PR `breaking` / `feature` / `fix` so the highest label across the
merged-since-last-release set dictates the next bump, and the release notes
group cleanly.

**Who cuts a release:** maintainers, not contributors. Merging a PR does not cut
a release — a release tag is a deliberate "this slice of `main` is blessed for
download" act. Contributors land labeled changes; a maintainer decides when to
tag and what the bump is, then pushes a `vX.Y.Z` tag. That tag (and only that)
triggers the GitHub Release workflow, which cross-compiles the binaries and
attaches them. Tagging is **decoupled from deploys**: rstash.cloud continuously
deploys every approved merge to `main` regardless of tags — a release tag only
governs the downloadable binaries.

**Version string:** every binary bakes its version in via `git describe` —
`v0.4.1` on a release tag, `v0.4.1-7-gabc1234` on an untagged commit — so any
build traces back to a commit (`rstash --version`, or the admin status page). A
plain `go build`/`go run` during development falls back to the commit SHA Go
embeds, so it's never an opaque `dev`.

**`go install` is not supported.** The module path is the bare name `rstash`
(not a URL), so `go install` can't resolve it, and the binary embeds a docs site
generated at build time that a plain `go install` would skip. Use the binaries
on the [Releases page](https://github.com/Klutch-Software-Incorporated/rstash/releases)
or `task build`.

## Getting Started

```sh
task build    # Build the binary
task run      # Run the server via go run
task test     # Run all tests
task fmt      # Format source code
task vet      # Run go vet
task clean    # Remove build artifacts and local database
```

## Running Tests

```sh
# All tests
task test

# Single package
go test ./internal/blob/

# Single test
go test -run TestName ./internal/package/
```

### Integration Tests

Some tests require external services and are skipped by default. Set the corresponding environment variable to enable them.

#### S3 Blob Storage

S3 integration tests require an S3-compatible service. The easiest way is to run MinIO locally:

```sh
# Start MinIO (use docker.io/ prefix for Podman)
podman run -d --name minio -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  docker.io/minio/minio server /data --console-address ":9001"

# Create the test bucket
podman exec minio mc alias set local http://localhost:9000 minioadmin minioadmin
podman exec minio mc mb local/rstash-test
```

Then run the tests:

```sh
export RSTASH_TEST_S3_DSN="rstash-test?endpoint=localhost:9000&tls=false&access_key=minioadmin&secret_key=minioadmin"
go test ./internal/blob/ -run TestS3_Integration -v
```

Cleanup:

```sh
podman stop minio && podman rm minio
```

#### Azure Blob Storage

Azure Blob integration tests run against [Azurite](https://github.com/Azure/Azurite), the official local emulator. The fastest way is Docker/Podman:

```sh
# Start Azurite (blob service only, on port 10000)
podman run -d --name azurite -p 10000:10000 \
  mcr.microsoft.com/azure-storage/azurite \
  azurite-blob --blobHost 0.0.0.0

# Create the test container. The key below is Azurite's well-known
# default credential — it's public and safe to paste.
az storage container create --name rstash-test \
  --connection-string "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
```

Then run the tests:

```sh
export RSTASH_TEST_AZURE_BLOB_DSN="rstash-test?account=devstoreaccount1&key=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==&endpoint=127.0.0.1:10000&tls=false"
go test ./internal/blob/ -run TestAzureBlob_Integration -v
```

Cleanup:

```sh
podman stop azurite && podman rm azurite
```

## Manual Smoke Testing with S3

To test the full server with S3-backed blob storage:

```sh
# Start MinIO as above, then create a bucket
podman exec minio mc mb local/rstash

# Start rstash with S3 blob storage
export RSTASH_DB="sqlite:rstash.sqlite"
export RSTASH_BLOB="s3:rstash?endpoint=localhost:9000&tls=false&access_key=minioadmin&secret_key=minioadmin"
go run . serve
```

You can browse objects in the MinIO console at `http://localhost:9001` (login: minioadmin/minioadmin).

**Note:** The S3 bucket must exist before starting the server. rstash checks for the bucket at startup and will exit with a clear error if it's missing.

## Project Structure

See the Architecture section in [README.md](README.md) and the conventions in [CLAUDE.md](CLAUDE.md).

## Dev Mode (Hot Reload)

Build with the `dev` tag to serve templates, static assets, and the Astro site from disk instead of the embedded binary. This gives you hot reload on browser refresh — no Go recompile needed.

```sh
go run -tags dev .
```

What this changes:

- **Go templates** (`internal/ui/templates/`) — re-parsed on every request, so edits show up on refresh
- **Static assets** (`internal/ui/static/`) — served from disk via `os.DirFS`, changes are immediate
- **Astro site** (`internal/ui/site/`) — served from disk, so `astro build --watch` output is picked up on refresh

Without the `dev` tag (default), everything is embedded via `go:embed` as usual for single-binary deployment.

## Key Conventions

- Standard library `net/http` for routing (Go 1.22+ enhanced patterns)
- GORM ORM with multi-dialect support
- All database access goes through `*db.Repository`
- Transactions use `repo.Transaction(func(txRepo *db.Repository) error { ... })`
- `log/slog` for structured logging
- Interfaces for pluggable backends (`blob.Store`, `auth.Service`)
- Server-rendered Go HTML templates for web UI
- All assets embedded via `go:embed` for single-binary deployment
