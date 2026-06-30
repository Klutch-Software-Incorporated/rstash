---
title: Deployment
description: Run rstash in production with TLS, reverse proxies, and systemd.
order: 6
---

## Automatic TLS

The simplest production setup — rstash handles HTTPS directly via Let's Encrypt:

```bash
export RSTASH_ADDR=":443"
export RSTASH_BASE_URL="https://storage.example.com"
export RSTASH_TLS_MODE="auto"

rstash
```

Requirements:
- Your domain's DNS points to the server
- Port 443 is open and reachable from the internet
- The certificate cache directory (`./certs` by default) is writable

Certificates are automatically obtained and renewed.

## Manual TLS

If you have your own certificates:

```bash
export RSTASH_ADDR=":443"
export RSTASH_BASE_URL="https://storage.example.com"
export RSTASH_TLS_MODE="manual"
export RSTASH_TLS_CERT="/etc/ssl/certs/storage.example.com.pem"
export RSTASH_TLS_KEY="/etc/ssl/private/storage.example.com.key"
```

## Behind a Reverse Proxy

Run rstash behind nginx, Caddy, or another proxy. Disable TLS in rstash and let the proxy handle it:

```bash
export RSTASH_ADDR="127.0.0.1:8080"
export RSTASH_BASE_URL="https://storage.example.com"
export RSTASH_TLS_MODE="off"
```

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name storage.example.com;

    ssl_certificate     /etc/letsencrypt/live/storage.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/storage.example.com/privkey.pem;

    client_max_body_size 50m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

### Caddy

```
storage.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy handles TLS automatically — no certificate configuration needed.

## systemd

Create `/etc/systemd/system/rstash.service`:

```ini
[Unit]
Description=rstash remoteStorage server
After=network.target

[Service]
Type=simple
User=rstash
Group=rstash
WorkingDirectory=/opt/rstash
ExecStart=/usr/local/bin/rstash
Restart=on-failure
RestartSec=5
EnvironmentFile=/opt/rstash/.env

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/rstash

[Install]
WantedBy=multi-user.target
```

Set up the service user and directory, then enable it:

```bash
sudo useradd -r -s /usr/sbin/nologin rstash
sudo mkdir -p /opt/rstash
sudo chown rstash:rstash /opt/rstash
sudo cp rstash /usr/local/bin/
sudo cp .env /opt/rstash/.env

sudo systemctl daemon-reload
sudo systemctl enable --now rstash
```

## Docker

The repository includes a multi-stage Dockerfile that builds from source:

```dockerfile
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X rstash/internal/config.Version=${VERSION}" -o /rstash .

FROM gcr.io/distroless/static-debian12
COPY --from=build /rstash /rstash
EXPOSE 8080
ENTRYPOINT ["/rstash"]
```

Build and run:

```bash
docker build -t rstash .
docker run -d \
  -p 8080:8080 \
  -v rstash-data:/data \
  -e RSTASH_BASE_URL="https://storage.example.com" \
  -e RSTASH_DB="sqlite:/data/rstash.sqlite" \
  -e RSTASH_BLOB="sqlite:/data/rstash-blobs.sqlite" \
  rstash
```

Or if you already have a pre-built binary and just want a minimal image:

```dockerfile
FROM gcr.io/distroless/static-debian12
COPY rstash-linux-amd64 /rstash
EXPOSE 8080
ENTRYPOINT ["/rstash"]
```

> **Tip:** Set `RSTASH_DB` and `RSTASH_BLOB` paths to point inside your mounted volume so data persists across container restarts.

## Verify Your Setup

Before going to production, run [`rstash check`](/docs/cli/) to verify your configuration:

```bash
rstash check
```

This validates all settings and tests connectivity to the database, blob store, and email provider (if configured).

## Database in Production

### SQLite

Works well for personal and small-group use. Make sure the database directory is on a filesystem that supports file locking (avoid NFS). SQLite is the default and requires no setup.

### PostgreSQL

For larger deployments or when you need high availability:

```bash
export RSTASH_DB="postgres:host=db.example.com dbname=rstash user=rstash password=secret sslmode=require"
export RSTASH_BLOB="fs:/var/lib/rstash/blobs"
```

Using `fs:` for blob storage alongside PostgreSQL keeps large files on disk instead of in the database. See the [configuration reference](/docs/configuration/) for all supported DSN formats.

## Backups

### SQLite

Back up the database files (stop the server first for a consistent snapshot, or use SQLite's backup API):

```bash
cp /opt/rstash/rstash.sqlite /backups/rstash-$(date +%F).db
cp /opt/rstash/rstash-blobs.sqlite /backups/rstash-blobs-$(date +%F).db
```

### Filesystem Blob Storage

If you use `fs:` for blobs, include that directory in your regular backup schedule alongside the metadata database.

### S3 Blob Storage

If you use `s3:` for blobs, rely on your S3 provider's built-in durability and versioning. You still need to back up the metadata database.
