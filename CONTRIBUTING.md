# Contributing to rstash

## Prerequisites

- Go 1.24+
- [Task](https://taskfile.dev/) (task runner)
- [Fossil](https://fossil-scm.org/) (source control)

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

## Manual Smoke Testing with S3

To test the full server with S3-backed blob storage:

```sh
# Start MinIO as above, then create a bucket
podman exec minio mc mb local/rstash

# Start rstash with S3 blob storage
export RSTASH_DB="sqlite:rstash.db"
export RSTASH_BLOB="s3:rstash?endpoint=localhost:9000&tls=false&access_key=minioadmin&secret_key=minioadmin"
go run . serve
```

You can browse objects in the MinIO console at `http://localhost:9001` (login: minioadmin/minioadmin).

**Note:** The S3 bucket must exist before starting the server. rstash checks for the bucket at startup and will exit with a clear error if it's missing.

## Project Structure

See the Architecture section in [README.md](README.md) and the conventions in [CLAUDE.md](CLAUDE.md).

## Key Conventions

- Standard library `net/http` for routing (Go 1.22+ enhanced patterns)
- GORM ORM with multi-dialect support
- All database access goes through `*db.Repository`
- Transactions use `repo.Transaction(func(txRepo *db.Repository) error { ... })`
- `log/slog` for structured logging
- Interfaces for pluggable backends (`blob.Store`, `auth.Service`)
- Server-rendered Go HTML templates for web UI
- All assets embedded via `go:embed` for single-binary deployment
