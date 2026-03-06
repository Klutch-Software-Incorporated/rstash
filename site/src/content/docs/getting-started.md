---
title: Getting Started
description: Download rstash and get it running in under a minute.
order: 1
---

## Download a Binary

Grab the latest release for your platform:

```bash
# Linux (amd64)
curl -LO https://fossil.klutch.software/rstash/uv/rstash-linux-amd64
chmod +x rstash-linux-amd64
sudo mv rstash-linux-amd64 /usr/local/bin/rstash
```

Pre-built binaries are available for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## Build from Source

You'll need Go 1.22 or later:

```bash
fossil clone https://fossil.klutch.software/rstash rstash.fossil
fossil open rstash.fossil
go build -o rstash .
```

## First Run

Start the server:

```bash
rstash
```

Visit `http://localhost:8080` in your browser. Since no users exist yet, rstash shows the setup wizard:

1. **Review settings** — rstash shows your current server configuration (database, storage backend, base URL, TLS). If you're using the SQLite defaults, a note explains what that means and how to change it.
2. **Create admin account** — pick a username and password for the first admin user.

That's it — you're in. rstash creates a SQLite database in the current directory on first start.

## Connect a remoteStorage App

Any [remoteStorage-compatible app](https://remotestorage.io/apps/) can connect to your server. When the app asks for a user address, enter:

```
yourname@your-server-hostname.com
```

The app discovers your server via WebFinger and walks you through the OAuth authorization flow to grant it access.

## Next Steps

- [Setup](/docs/setup/) — configure registration, quotas, and common settings
- [Web UI](/docs/web-ui/) — explore the dashboard, file browser, and admin panel
- [Deployment](/docs/deployment/) — run in production with TLS
