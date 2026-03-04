---
title: Getting Started
description: Download gosilo and get it running in under a minute.
order: 1
---

## Download a Binary

Grab the latest release for your platform:

```bash
# Linux (amd64)
curl -LO https://code.lag.dev/gosilo/releases/latest/gosilo-linux-amd64
chmod +x gosilo-linux-amd64
sudo mv gosilo-linux-amd64 /usr/local/bin/gosilo
```

Pre-built binaries are available for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)
- Windows (amd64)

## Build from Source

You'll need Go 1.22 or later:

```bash
git clone https://code.lag.dev/gosilo
cd gosilo
go build -o gosilo .
```

## First Run

Start the server:

```bash
gosilo serve
```

Visit `http://localhost:8080` in your browser. Since no users exist yet, gosilo automatically shows the setup page where you can create your first admin account.

Alternatively, you can set up the first admin from the command line before starting the server:

```bash
gosilo init                              # interactive setup
gosilo init --username admin --password secret   # non-interactive
```

Either way, gosilo creates a SQLite database in the current directory on first start.

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
