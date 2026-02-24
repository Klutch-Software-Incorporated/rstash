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

All configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| GOSILO_ADDR | :8080 | Listen address |
| GOSILO_BASE_URL | http://localhost:8080 | Public URL of the server |
| GOSILO_DB_PATH | gosilo.db | SQLite database path |
| GOSILO_BLOB_BACKEND | sqlite | Blob storage backend (sqlite, fs, s3) |
| GOSILO_BLOB_PATH | | Path for filesystem blob backend |
| GOSILO_REGISTRATION | closed | Registration mode: open, invite, closed |
| GOSILO_LOG_LEVEL | info | Log level: debug, info, warn, error |

## Architecture

- `main.go` — entry point: loads config, opens DB, wires handlers, starts HTTP server
- `internal/config/` — env var configuration loading
- `internal/db/` — SQLite database initialization and migrations
- `internal/model/` — domain types (User, OAuthClient, OAuthToken, Node)
- `internal/blob/` — pluggable blob storage interface and backends (SQLite, filesystem, S3)
- `internal/api/` — remoteStorage protocol handlers (storage API, WebFinger, OAuth token), CORS, scope checking, request logging
- `internal/web/` — web UI handlers (login, setup, registration, admin, file browser, OAuth authorize), session middleware, CSRF
- `internal/ui/` — embedded templates and static assets (go:embed)

## Key Conventions

- Standard library net/http for routing (Go 1.22 enhanced patterns)
- modernc.org/sqlite (pure Go, no CGO)
- log/slog for structured logging
- Interfaces for pluggable backends (blob.Store)
- Server-rendered Go HTML templates for web UI
- All assets embedded via go:embed for single-binary deployment

## remoteStorage Protocol

- Spec: draft-dejong-remotestorage-26
- WebFinger: `GET /.well-known/webfinger?resource=acct:user@host`
- OAuth: `GET /oauth/authorize`, `POST /oauth/token`
- Storage: `GET/PUT/DELETE/HEAD /storage/{user}/{path...}`
- Public paths (`/public/`) readable without auth
- Folders end with `/`, documents don't
- ETags required on all storage responses
- Folder listings use JSON-LD with `@context: "http://remotestorage.io/spec/folder-description"`
