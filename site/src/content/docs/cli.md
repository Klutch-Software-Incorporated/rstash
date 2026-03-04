---
title: CLI Reference
description: All gosilo commands and their usage.
order: 5
---

gosilo is a single binary with a built-in CLI for server management, user administration, and diagnostics.

## Server

### `gosilo serve`

Start the remoteStorage server.

```bash
gosilo serve
gosilo serve --web oauth    # override web UI mode
gosilo serve --db sqlite:custom.db
```

### `gosilo init`

Initialize the database and create the first admin user.

```bash
gosilo init                                       # interactive setup
gosilo init --username admin --password "s3cure"   # non-interactive
```

In interactive mode, after creating the admin account gosilo asks whether to allow public registration and whether to enable rate limiting. In non-interactive mode (when `--username` is provided), sensible defaults are used: closed registration and rate limiting enabled.

### `gosilo env`

Print a documented `.env` template with all available configuration variables.

```bash
gosilo env          # print to stdout
gosilo env > .env   # save to file
```

## User Management

### `gosilo user add`

Create a new user. Prompts for a password interactively.

```bash
gosilo user add alice
gosilo user add alice --admin              # create as admin
gosilo user add alice --password "secret"  # non-interactive
```

### `gosilo user list`

List all users.

```bash
gosilo user list
gosilo user list --json    # machine-readable output
```

Shows username, admin status, disabled status, approval status, and creation date.

### `gosilo user passwd`

Change a user's password.

```bash
gosilo user passwd alice
gosilo user passwd alice --password "newsecret"
```

### `gosilo user promote`

Grant admin privileges.

```bash
gosilo user promote alice
```

### `gosilo user disable`

Disable or re-enable an account. Disabling immediately terminates all active sessions.

```bash
gosilo user disable alice        # disable
gosilo user disable alice --enable   # re-enable
```

### `gosilo user delete`

Permanently delete a user account. Removes the user record, sessions, and OAuth tokens. Stored files are not deleted automatically.

```bash
gosilo user delete alice
gosilo user delete alice --force   # skip confirmation
```

### `gosilo user approve`

Approve or reject a pending registration (when using `approval` registration mode).

```bash
gosilo user approve alice
gosilo user approve alice --reject   # delete the pending user
```

## Configuration

### `gosilo config list`

Show all runtime settings with their current values and sources.

```bash
gosilo config list
gosilo config list --json
```

### `gosilo config get`

Get a single setting's value.

```bash
gosilo config get registration_mode
gosilo config get quota_user --json
```

### `gosilo config set`

Set a runtime override. Takes effect immediately — no restart needed.

```bash
gosilo config set registration_mode open
gosilo config set quota_user 500MB
gosilo config set rate_limit_rate 0       # disable rate limiting
```

### `gosilo config reset`

Remove a database override and revert to the environment variable or default value.

```bash
gosilo config reset registration_mode
```

## Diagnostics

### `gosilo doctor`

Run health checks on your installation.

```bash
gosilo doctor
gosilo doctor --json
```

Checks database integrity, schema, WAL mode, user count, storage stats, expired sessions/tokens, settings consistency, TLS configuration, and blob store connectivity.

### `gosilo audit tail`

Show recent audit log entries.

```bash
gosilo audit tail          # last 25 entries
gosilo audit tail -n 50    # last 50
gosilo audit tail --json
```

### `gosilo audit export`

Export the full audit log as JSON Lines (one JSON object per line).

```bash
gosilo audit export > audit.jsonl
```

### `gosilo licenses`

List all third-party dependencies and their licenses.

```bash
gosilo licenses
gosilo licenses --json
gosilo licenses --module github.com/some/pkg   # show full license text
```
