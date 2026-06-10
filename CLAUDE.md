# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

rstash is a remoteStorage server (draft-dejong-remotestorage-26) written in Go.
It implements the remoteStorage protocol including WebFinger discovery, OAuth 2.0
authorization, and the storage API (GET/PUT/DELETE/HEAD for documents and folders).

The target audience is technical self-hosters running personal or small family/friends
servers. The design prioritizes a "just run it and go" experience — run `rstash` to
start the server and complete setup through the web UI.

Source control is managed via Git, hosted on GitHub (Klutch-Software-Incorporated/rstash).
The hosted rstash.cloud deploy pipeline lives in the separate `rstash-infra` repo on
Azure DevOps, which pulls from GitHub to build and push the container image — GitHub
itself holds no deploy credentials. See `docs/github-migration.md` for the full split.

## Build & Run Commands

- **Build:** `task build` (cross-compiles release binaries to `dist/`)
- **Run:** `task run` (or `go run .`)
- **Run tests:** `task test` (or `go test ./...`)
- **Run a single test:** `go test -run TestName ./internal/package`
- **Format code:** `task fmt` (or `gofmt -w .`)
- **Vet code:** `task vet` (or `go vet ./...`)

## CLI

The CLI is intentionally minimal. Running `rstash` starts the server (default command).

- `rstash` — start the HTTP server (same as `rstash serve`)
- `rstash env` — print a documented .env configuration template to stdout
- `rstash check` — validate configuration and test database/blob store connectivity

All server management (users, settings, etc.) is done through the web UI.

## Configuration

All configuration is via environment variables (see `rstash env` for a documented template):

| Variable | Default | Description |
|----------|---------|-------------|
| RSTASH_ADDR | :8080 | Listen address (host:port) |
| RSTASH_BASE_URL | http://localhost:8080 | Public URL of the server |
| RSTASH_DB | sqlite:rstash.sqlite | Metadata database DSN (sqlite:, postgres:, mysql:, mssql:) |
| RSTASH_BLOB | sqlite:rstash-blobs.sqlite | Blob store DSN (sqlite:path, fs:/path, s3:bucket, azureblob:container, or database DSN) |
| RSTASH_LOG_LEVEL | info | Log level: debug, info, warn, error |
| RSTASH_LOG_FILE | | Path to log file (empty = stderr only) |
| RSTASH_TLS_CERT | | TLS certificate file path |
| RSTASH_TLS_KEY | | TLS private key file path |
| RSTASH_TLS_MODE | *(empty)* | TLS mode: off, manual, auto (empty=auto-detect) |
| RSTASH_TLS_CACHE | ./certs | Autocert certificate cache directory |
| RSTASH_EMAIL | | Email provider DSN (e.g. resend:API_KEY?from=noreply@example.com) |

Additional settings (registration mode, rate limits, quotas, OAuth token lifetime, etc.)
are managed at runtime through the admin web UI and stored in the database.

## Architecture

- `main.go` — entry point, delegates to CLI
- `internal/cli/` — cobra-based CLI: serve (default), env, check commands
- `internal/config/` — env var configuration loading, validation, DSN parsing, byte-size/token-lifetime parsing, setting definitions registry
- `internal/settings/` — runtime settings (DB overrides merged with env defaults, atomic snapshot)
- `internal/db/` — GORM-based database layer with Repository pattern, AutoMigrate, multi-dialect support (SQLite, PostgreSQL, MySQL, SQL Server)
- `internal/model/` — domain types (User, OAuthClient, OAuthToken, Node, Session, AuditEntry, AuthorizationCode)
- `internal/auth/` — authentication service interface and local (password-based) implementation, session/cookie management
- `internal/blob/` — pluggable blob storage interface, backends (SQLite, filesystem, GORM, S3, Azure Blob Storage), and `OpenStore()` factory
- `internal/storage/` — storage service (PutDocument, GetDocument, DeleteDocument, GetFolder), ETag generation, quota checking
- `internal/api/` — remoteStorage protocol handlers (storage API, WebFinger, OAuth token), CORS, scope checking, request logging, rate limiting, security headers
- `internal/email/` — pluggable email delivery interface, Resend backend, `Open()` factory, email body templates
- `internal/web/` — web UI handlers (setup wizard, login, registration, admin panel, file browser, profile/settings, OAuth authorize, abuse reports, account/email management, password reset), session middleware, CSRF, AdminGuard, AccountGuard
- `internal/ui/` — embedded templates and static assets (go:embed), template renderer, flash messages

## Key Conventions

- Standard library net/http for routing (Go 1.22 enhanced patterns)
- cobra for CLI (minimal — just serve, env, check)
- GORM ORM with multi-dialect support (gorm.io/gorm)
- glebarez/sqlite for pure-Go SQLite (no CGO), gorm.io/driver/postgres, gorm.io/driver/mysql, gorm.io/driver/sqlserver
- log/slog for structured logging
- Interfaces for pluggable backends (blob.Store, auth.Service)
- Server-rendered Go HTML templates for web UI (custom CSS, no build tooling)
- All assets embedded via go:embed for single-binary deployment
- Audit logging for all state-changing operations (admin, storage, auth, OAuth)
- Runtime settings: DB overrides take precedence over env defaults, atomic snapshot swap
- All database access goes through `*db.Repository` (wraps `*gorm.DB`); package-level DB functions no longer exist
- Transactions use `repo.Transaction(func(txRepo *db.Repository) error { ... })` pattern
- SQLite LIKE is case-insensitive by default; we set `PRAGMA case_sensitive_like = ON` at DB init so path prefix queries are case-sensitive. For other databases, LIKE queries on paths use case-sensitive collation (e.g. COLLATE BINARY or equivalent).
- Schema managed by GORM AutoMigrate — no raw DDL strings

## Setup Flow

On first run (no users in database), all routes redirect to `/setup`:
1. **Review page** (`GET /setup`) — shows current server settings (base URL, database type, storage type, TLS) with warnings if SQLite defaults are in use
2. **Account creation** (`GET /setup?step=account`) — create the first admin account
3. After setup, the admin is logged in and redirected to the admin panel

## remoteStorage Protocol

- Spec: draft-dejong-remotestorage-26
- WebFinger: `GET /.well-known/webfinger?resource=acct:user@host`
- OAuth: `GET /oauth/authorize`, `POST /oauth/token`
- Storage: `GET/PUT/DELETE/HEAD /storage/{user}/{path...}`
- Public paths (`/public/`) readable without auth
- Folders end with `/`, documents don't
- ETags required on all storage responses
- Folder listings use JSON-LD with `@context: "http://remotestorage.io/spec/folder-description"`
