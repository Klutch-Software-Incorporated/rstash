---
title: Web UI
description: Dashboard, file browser, admin panel, and OAuth authorization.
order: 3
---

gosilo includes a full web interface — no separate frontend to install. Everything is embedded in the single binary.

## User Dashboard

After logging in, you land on your dashboard at `/~yourname/`. It shows:

- **Storage stats** — file count, total size, and quota usage (if quotas are enabled)
- **Recent files** — your 10 most recently modified files
- **Largest files** — your 10 biggest files
- **Active sessions** — how many browser sessions you have open
- **OAuth tokens** — apps currently authorized to access your data

## File Browser

The file browser at `/~yourname/files/` lets you manage your stored data:

- **Browse folders** — navigate your remoteStorage folder tree
- **Upload files** — drag-and-drop or use the upload button (respects max upload size and quotas)
- **Create folders** — organize your data into modules and subfolders
- **Preview files** — view images, PDFs, and text files inline
- **Search** — find files across your entire storage by name or path
- **Bulk operations** — select multiple files and delete them at once

Files are organized just like the remoteStorage API exposes them, so what you see in the browser matches what apps access via the API.

## Account Settings

Your settings page at `/~yourname/settings` lets you:

- **Change your password**
- **Manage sessions** — see all active sessions with creation and expiry dates, terminate individual sessions or all at once
- **Manage OAuth tokens** — see which apps have access, revoke tokens you no longer need
- **View storage usage** — see quota breakdown if quotas are enabled

## OAuth Authorization

When a remoteStorage app wants to access your data, it redirects you to gosilo's OAuth consent screen. You'll see:

- Which app is requesting access (identified by its origin URL)
- What scopes it's requesting (e.g., "Read & write your documents data")
- A warning if it's requesting full access to all your data

Click **Allow** to grant the app a token, or **Deny** to reject it. You can always revoke access later from your settings page.

## Admin Panel

Admin users have access to the admin panel at `/admin/`. It includes:

### Dashboard

An overview of your server: total users, pending approvals, storage usage, active users in the last 24 hours and 7 days, top users by storage, and recent audit log entries.

### User Management

View and manage all users. From here you can:

- Create new users
- Grant or revoke admin privileges
- Disable or re-enable accounts (immediately terminates their sessions)
- Approve or reject pending registrations
- Set per-user storage quotas
- Delete users

### Settings

A grouped editor for all runtime settings — access control, rate limiting, storage quotas, OAuth, legal pages, and more. Changes take effect immediately without restarting the server. Each setting shows whether it's using the default value or a database override, and you can reset any override with one click.

Environment-only settings (database DSN, listen address, TLS) are shown as read-only so you can see their current values.

### Audit Log

A searchable log of all state-changing actions: user creation, login attempts, file uploads/deletions, setting changes, admin actions, and OAuth grants. Useful for troubleshooting and security review.

### Other Admin Tools

- **Abuse Reports** — review and resolve content reports submitted via the public abuse form
- **OAuth Test Tool** — test the OAuth authorization flow interactively
- **Server Logs** — view recent log output (when file logging is enabled)

## Next Steps

- [Configuration Reference](/docs/configuration/) — all environment variables
- [CLI Reference](/docs/cli/) — command-line tools
