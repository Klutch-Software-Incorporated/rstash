---
title: Setup
description: Configure registration, users, quotas, and common options.
order: 2
---

## Initial Setup

When you first run `rstash`, it starts the server and redirects you to the setup wizard at `/setup`. The wizard has two steps:

1. **Review settings** — shows your current server configuration: base URL, database type (e.g., "SQLite (default)" or "PostgreSQL"), file storage type, and TLS status. If the database or file storage is using SQLite defaults, a warning explains what that means and how to change it before continuing.

2. **Create admin account** — pick a username and password. This account has full admin privileges.

After setup, you're logged in and taken to the admin panel.

> **Tip:** If you want to use PostgreSQL, MySQL, or a different storage backend, set the `RSTASH_DB` and `RSTASH_BLOB` environment variables *before* running rstash for the first time. Run `rstash env` to see all available options, or `rstash check` to verify your configuration.

## Creating Users

From the admin panel, go to Users and use the "Create User" form. You can set admin privileges from there.

If registration is open, users can also create their own accounts at `/register`.

## Registration Modes

Control how new users can sign up. Change this in the admin panel under Settings.

- **`closed`** (default) — Only admins can create accounts. The registration page shows a "closed" message.
- **`open`** — Anyone can register and immediately log in.
- **`approval`** — Anyone can register, but they can't log in until an admin approves their account from the admin panel.

## Storage Quotas

rstash can limit how much data users store. Configure this under Settings in the admin panel:

- **`off`** — No limits, anyone can store as much as they want.
- **`total`** — All users share one global storage pool (default: 50 GB).
- **`user`** — Each user gets their own quota (e.g., 500 MB per user). Admins can override individual quotas from the user list.

The storage meter appears on each user's dashboard so they can track their usage.

## Rate Limiting

Per-IP rate limiting is enabled by default (10 requests/sec with a burst of 20). Adjust or disable it from the admin Settings page.

## Upload Size

The maximum file size per upload defaults to 50 MB. Change it in Settings.

## OAuth Token Lifetime

OAuth tokens issued to remoteStorage apps expire after 30 days by default. Users can always revoke tokens manually from their settings page. Adjust the lifetime in Settings.

## Changing Settings at Runtime

Most settings can be changed without restarting the server. Use the admin panel Settings page — changes take effect immediately. Each setting shows whether it's using the default value or a database override, and you can reset any override with one click.

Settings that require a restart (database DSN, blob store DSN, listen address, TLS) are configured via environment variables and shown in the admin panel as read-only.

## Next Steps

- [Web UI](/docs/web-ui/) — tour of the dashboard, file browser, and admin panel
- [Configuration Reference](/docs/configuration/) — full list of environment variables
