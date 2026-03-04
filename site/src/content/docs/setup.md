---
title: Setup
description: Configure registration, users, quotas, and common options.
order: 2
---

## Initial Setup

When you run `gosilo init` interactively, it asks two setup questions after creating the admin account:

```
Allow public registration? (y/N):
Enable rate limiting? (Y/n):
```

- **Registration** — defaults to **closed** (only admins can create accounts). Answer `y` to allow anyone to register.
- **Rate limiting** — defaults to **enabled** (10 req/sec with a burst of 20). Answer `n` to disable it.

When running non-interactively (`gosilo init --username admin --password secret`), the defaults are used automatically (closed registration, rate limiting enabled).

## Creating Users

There are two ways to create users:

**From the CLI:**

```bash
gosilo user add alice           # interactive password prompt
gosilo user add alice --admin   # create as admin
```

**From the admin panel:**

Log in as an admin, go to the admin panel, and use the "Create User" form at the bottom of the Users page. You can set admin privileges right from there.

## Registration Modes

Control how new users can sign up. Change this in the admin panel under Settings, or via CLI:

```bash
gosilo config set registration_mode open
```

- **`closed`** (default) — Only admins can create accounts. The registration page shows a "closed" message.
- **`open`** — Anyone can register and immediately log in.
- **`approval`** — Anyone can register, but they can't log in until an admin approves their account from the admin panel.

## Storage Quotas

gosilo can limit how much data users store. Configure this under Settings in the admin panel:

- **`off`** — No limits, anyone can store as much as they want.
- **`total`** — All users share one global storage pool (default: 50 GB).
- **`user`** — Each user gets their own quota (e.g., 500 MB per user). Admins can override individual quotas from the user list.

The storage meter appears on each user's dashboard so they can track their usage.

## Rate Limiting

Per-IP rate limiting is enabled by default (10 requests/sec with a burst of 20). Adjust or disable it:

```bash
gosilo config set rate_limit_rate 0    # disable
gosilo config set rate_limit_rate 20   # 20 req/sec
gosilo config set rate_limit_burst 40
```

## Upload Size

The maximum file size per upload defaults to 50 MB. Change it in Settings or via CLI:

```bash
gosilo config set max_upload_size 100MB
```

## OAuth Token Lifetime

OAuth tokens issued to remoteStorage apps expire after 30 days by default. Users can always revoke tokens manually from their settings page.

```bash
gosilo config set token_lifetime 90d    # 90 days
gosilo config set token_lifetime 0      # never expire
```

## Web UI Mode

gosilo's web interface has three modes:

- **`full`** (default) — Everything: login, file browser, admin panel, OAuth.
- **`oauth`** — Only login and OAuth authorization (no file browser or admin panel). Useful if you only want API access.
- **`off`** — No web UI at all, pure API server.

Set via environment variable (requires restart):

```bash
export GOSILO_WEB_MODE=oauth
```

## Changing Settings at Runtime

Most settings can be changed without restarting the server. Use the admin panel Settings page, or the CLI:

```bash
gosilo config list              # show all settings with current values
gosilo config set key value     # override a setting (takes effect immediately)
gosilo config reset key         # remove override, revert to env/default
```

Database overrides always take precedence over environment variables. Run `gosilo config list` to see which settings are overridden.

## Next Steps

- [Web UI](/docs/web-ui/) — tour of the dashboard, file browser, and admin panel
- [Configuration Reference](/docs/configuration/) — full list of environment variables
