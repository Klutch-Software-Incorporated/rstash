# gosilo - remoteStorage Server

`gosilo` is a self-hosted [remoteStorage](https://remotestorage.io/) server written in Go, implementing [draft-dejong-remotestorage-26](https://datatracker.ietf.org/doc/html/draft-dejong-remotestorage-26). It provides WebFinger discovery, OAuth 2.0 authorization, and the full storage API as a single binary with no external dependencies beyond SQLite.

## Key Features

- **Full remoteStorage protocol**: GET/PUT/DELETE/HEAD for documents and folders, ETags, conditional requests, folder listings with JSON-LD
- **WebFinger discovery**: Automatic `/.well-known/webfinger` endpoint for client bootstrapping
- **OAuth 2.0 implicit grant**: Built-in authorization flow with consent screen and scope management
- **Pluggable blob storage**: SQLite (default) or filesystem backends
- **Web UI**: Setup wizard, login, admin dashboard, file browser, account settings
- **Single binary**: All templates and static assets embedded via `go:embed`
- **Per-IP rate limiting**: Token bucket with configurable rate and burst, 429 responses with Retry-After

## Installation

Requires Go 1.24+.

```sh
go install gosilo@latest
```

Or build from source:

```sh
task build
```

The binary is output to `./build/gosilo.exe`.

## Quick Start

```sh
# Start with defaults (listens on :8080, SQLite storage)
gosilo serve

# First run opens the setup wizard at http://localhost:8080/setup
# to create an initial admin account.
```

## Usage

```
gosilo [command]

Commands:
  serve    Start the server (default)
  env      Print a documented .env template
  version  Print version
  help     Show help with all configuration options
```

## Configuration

All configuration is via environment variables. Run `gosilo env` to generate a documented `.env` template.

| Variable | Default | Description |
|----------|---------|-------------|
| `GOSILO_ADDR` | `:8080` | Listen address (host:port) |
| `GOSILO_BASE_URL` | `http://localhost:8080` | Public URL for WebFinger and OAuth redirects |
| `GOSILO_DB_PATH` | `gosilo.db` | Path to the SQLite database file |
| `GOSILO_BLOB_BACKEND` | `sqlite` | Blob storage backend: `sqlite`, `fs` |
| `GOSILO_BLOB_PATH` | | Directory for filesystem blob storage (required when backend=fs) |
| `GOSILO_REGISTRATION` | `closed` | Registration mode: `open`, `invite`, `closed` |
| `GOSILO_LOG_LEVEL` | `info` | Log verbosity: `debug`, `info`, `warn`, `error` |
| `GOSILO_RATE_LIMIT` | `10` | Per-IP rate limit in requests/sec (0 to disable) |
| `GOSILO_RATE_BURST` | `20` | Max burst size for rate limiting |

## Development

[Task](https://taskfile.dev/) is used as the task runner:

```sh
task build    # Build the binary
task run      # Run the server via go run
task test     # Run all tests
task fmt      # Format source code
task vet      # Run go vet
task clean    # Remove build artifacts and local database
```

Source control is managed via [Fossil](https://fossil-scm.org/), not git.

## Architecture

```
main.go                 Entry point, config loading, server wiring
internal/
  config/               Environment variable loading and validation
  db/                   SQLite database, migrations, queries
  model/                Domain types (User, OAuthClient, OAuthToken, Node)
  blob/                 Pluggable blob storage (SQLite, filesystem)
  storage/              Storage service (document/folder CRUD, ETags)
  auth/                 Authentication service (sessions, passwords)
  api/                  Protocol handlers (storage API, WebFinger, CORS, rate limiting)
  web/                  Web UI handlers (login, setup, admin, OAuth, file browser)
  ui/                   Embedded templates and static assets
```

## License

MIT
