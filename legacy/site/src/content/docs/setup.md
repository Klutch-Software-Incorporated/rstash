---
title: Setup
description: Configure registration, users, quotas, and common options.
order: 2
---

## Environment Variables

Some configuration **must be set via environment variables before starting** the server — these cannot be changed at runtime through the admin panel:

| Variable | Default | Description |
|----------|---------|-------------|
| `RSTASH_ADDR` | `:8080` | Listen address (`host:port`) |
| `RSTASH_BASE_URL` | `http://localhost:8080` | Public URL (used in WebFinger, OAuth redirects) |
| `RSTASH_DB` | `sqlite:rstash.sqlite` | Metadata database — `sqlite:`, `postgres:`, `mysql:`, or `mssql:` |
| `RSTASH_BLOB` | `sqlite:rstash-blobs.sqlite` | Blob storage — same DB prefixes, plus `fs:/path` or `s3:bucket` |
| `RSTASH_TLS_MODE` | *(auto-detect)* | `off`, `manual`, or `auto` (Let's Encrypt) |
| `RSTASH_TLS_CERT` | | TLS certificate file (for `manual` mode) |
| `RSTASH_TLS_KEY` | | TLS private key file (for `manual` mode) |
| `RSTASH_TLS_CACHE` | `./certs` | Autocert certificate cache directory |
| `RSTASH_EMAIL` | *(none)* | Email provider DSN (e.g. `resend:KEY?from=noreply@example.com`) |
| `RSTASH_LOG_FILE` | *(none)* | Path to log file (empty = stderr only) |

If you're fine with the defaults (SQLite, port 8080, no TLS, no email), you don't need to set any of these — just run `rstash`. Otherwise, set them before first run. Run `rstash env` to generate a documented `.env` template, or `rstash check` to validate your configuration.

See the [configuration reference](/docs/configuration/) for full details on all DSN formats and options.

## Initial Setup

When you first run `rstash`, it starts the server and redirects you to the setup wizard at `/setup`. The wizard has two steps:

1. **Review settings** — shows your current server configuration: base URL, database type (e.g., "SQLite (default)" or "PostgreSQL"), file storage type, and TLS status. If the database or file storage is using SQLite defaults, a warning explains what that means and how to change it before continuing.

2. **Create admin account** — pick a username, password, and email address. This account has full admin privileges.

After setup, you're logged in and taken to the admin panel. Everything below can be configured at runtime from the admin Settings page.

## Creating Users

From the admin panel, go to Users and use the "Create User" form. You can set admin privileges from there.

If registration is open, users can also create their own accounts at `/register`.

## Registration Modes

Control how new users can sign up. Change this in the admin panel under Settings.

- **`closed`** (default) — Only admins can create accounts. The registration page shows a "closed" message.
- **`open`** — Anyone can register and immediately log in.
- **`approval`** — Anyone can register, but they can't log in until an admin approves their account from the admin panel.

## Storage & Download Limits

rstash has two independent knobs for storage and two for monthly downloads (egress), all configured under Settings in the admin panel. Each defaults to `0` (unlimited/disabled) for a friction-free self-hoster experience.

**Storage**

- **Server storage limit** — a global cap across all users. Useful when the host has a fixed disk or a pay-per-GB backend.
- **Default user storage limit** — stamped onto each new user account at creation time. Existing users are never retroactively changed by editing this; admins edit per-user values from the user list.

**Egress (monthly downloads)**

- **Server egress limit** — global monthly download cap. Useful on bandwidth-metered hosts.
- **Default user egress limit** — stamped onto each new user account; resets on the calendar-month boundary (UTC).

A `0` on any of these means "no limit." A user whose row has `StorageQuota = 0` is unlimited — changing the server default later will not affect them. This is deliberate: it means billing integrations and admin edits are the single source of truth for existing accounts.

The storage meter and monthly-download meter appear on each user's dashboard whenever they have a limit set.

## Rate Limiting

Per-IP rate limiting is enabled by default (10 requests/sec with a burst of 20). Adjust or disable it from the admin Settings page.

## Upload Size

The maximum file size per upload defaults to 50 MB. Change it in Settings.

## OAuth Token Lifetime

OAuth tokens issued to remoteStorage apps expire after 30 days by default. Users can always revoke tokens manually from their settings page. Adjust the lifetime in Settings.

## Changing Settings at Runtime

Everything in this section (registration, quotas, rate limiting, upload size, OAuth lifetimes) can be changed without restarting the server. Use the admin panel Settings page — changes take effect immediately. Each setting shows whether it's using the default value or a database override, and you can reset any override with one click.

The environment variables listed at the top of this page (database, listen address, TLS, email) require a restart and are shown in the admin panel as read-only.

## Next Steps

- [Web UI](/docs/web-ui/) — tour of the dashboard, file browser, and admin panel
- [Configuration Reference](/docs/configuration/) — full list of environment variables
