# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Gosilo is a remoteStorage server (draft-dejong-remotestorage-26) written in Go.
It implements the remoteStorage protocol including WebFinger discovery, OAuth 2.0
authorization, and the storage API (GET/PUT/DELETE/HEAD for documents and folders).

Source control is managed via Fossil, not git.

## Build & Run Commands

- **Build:** `task build` (or `go build -o ./build/gosilo.exe .`)
- **Run:** `task run` (or `go run .`)
- **Run tests:** `task test` (or `go test ./...`)
- **Run a single test:** `go test -run TestName ./internal/package`
- **Format code:** `task fmt` (or `gofmt -w .`)
- **Vet code:** `task vet` (or `go vet ./...`)

## Configuration

All configuration is via environment variables (see `gosilo env` for a documented template):

| Variable | Default | Description |
|----------|---------|-------------|
| GOSILO_ADDR | :8080 | Listen address (host:port) |
| GOSILO_BASE_URL | http://localhost:8080 | Public URL of the server |
| GOSILO_DB | sqlite:gosilo.db | Metadata database DSN (sqlite:, postgres:, mysql:, mssql:) |
| GOSILO_BLOB | sqlite:gosilo-blobs.db | Blob store DSN (sqlite:path, fs:/path, or any database DSN) |
| GOSILO_REGISTRATION | closed | Registration mode: open, closed |
| GOSILO_LOG_LEVEL | info | Log level: debug, info, warn, error |
| GOSILO_RATE_LIMIT | 10 | Per-IP rate limit (req/sec, 0=disabled) |
| GOSILO_RATE_BURST | 20 | Rate limit burst size |
| GOSILO_QUOTA_MODE | total | Quota mode: off, total, user |
| GOSILO_QUOTA_TOTAL | 50GB | Global storage limit |
| GOSILO_QUOTA_USER | | Per-user storage quota |
| GOSILO_MAX_UPLOAD | 50MB | Max upload size per request |
| GOSILO_WEB_MODE | full | Web UI mode: full, oauth, off |
| GOSILO_TOKEN_LIFETIME | 30d | OAuth token lifetime (30d, 24h, 0=no expiry) |
| GOSILO_TLS_CERT | | TLS certificate file path |
| GOSILO_TLS_KEY | | TLS private key file path |
| GOSILO_TLS_MODE | *(empty)* | TLS mode: off, manual, auto (empty=auto-detect) |
| GOSILO_TLS_CACHE | ./certs | Autocert certificate cache directory |

## Architecture

- `main.go` — entry point, delegates to CLI
- `internal/cli/` — cobra-based CLI: serve, init, user, config, audit, doctor, env commands
- `internal/config/` — env var configuration loading, validation, DSN parsing, byte-size/token-lifetime parsing
- `internal/settings/` — runtime settings (DB overrides merged with env defaults, atomic snapshot)
- `internal/db/` — GORM-based database layer with Repository pattern, AutoMigrate, multi-dialect support (SQLite, PostgreSQL, MySQL, SQL Server)
- `internal/model/` — domain types (User, OAuthClient, OAuthToken, Node, Session, AuditEntry, AuthorizationCode)
- `internal/auth/` — authentication service interface and local (password-based) implementation, session/cookie management
- `internal/blob/` — pluggable blob storage interface and backends (SQLite, filesystem)
- `internal/storage/` — storage service (PutDocument, GetDocument, DeleteDocument, GetFolder), ETag generation, quota checking
- `internal/api/` — remoteStorage protocol handlers (storage API, WebFinger, OAuth token), CORS, scope checking, request logging, rate limiting, security headers
- `internal/web/` — web UI handlers (login, setup, registration, admin, file browser, OAuth authorize, settings), session middleware, CSRF, AdminGuard
- `internal/ui/` — embedded templates and static assets (go:embed), template renderer, flash messages

## Key Conventions

- Standard library net/http for routing (Go 1.22 enhanced patterns)
- cobra for CLI (command groups, help examples, --json/--db global flags, consistent exit codes)
- GORM ORM with multi-dialect support (gorm.io/gorm)
- glebarez/sqlite for pure-Go SQLite (no CGO), gorm.io/driver/postgres, gorm.io/driver/mysql, gorm.io/driver/sqlserver
- log/slog for structured logging
- Interfaces for pluggable backends (blob.Store, auth.Service)
- Server-rendered Go HTML templates for web UI (custom CSS, no build tooling)
- All assets embedded via go:embed for single-binary deployment
- Audit logging for all state-changing operations (admin, CLI, storage, auth, OAuth)
- Runtime settings: DB overrides take precedence over env defaults, atomic snapshot swap
- All database access goes through `*db.Repository` (wraps `*gorm.DB`); package-level DB functions no longer exist
- Transactions use `repo.Transaction(func(txRepo *db.Repository) error { ... })` pattern
- SQLite LIKE is case-insensitive by default; we set `PRAGMA case_sensitive_like = ON` at DB init so path prefix queries are case-sensitive. For other databases, LIKE queries on paths use case-sensitive collation (e.g. COLLATE BINARY or equivalent).
- Schema managed by GORM AutoMigrate — no raw DDL strings

## remoteStorage Protocol

- Spec: draft-dejong-remotestorage-26
- WebFinger: `GET /.well-known/webfinger?resource=acct:user@host`
- OAuth: `GET /oauth/authorize`, `POST /oauth/token`
- Storage: `GET/PUT/DELETE/HEAD /storage/{user}/{path...}`
- Public paths (`/public/`) readable without auth
- Folders end with `/`, documents don't
- ETags required on all storage responses
- Folder listings use JSON-LD with `@context: "http://remotestorage.io/spec/folder-description"`
