---
title: Deployment
description: Run gosilo in production with TLS, reverse proxies, and systemd.
order: 6
---

## Automatic TLS

The simplest production setup — gosilo handles HTTPS directly via Let's Encrypt:

```bash
export GOSILO_ADDR=":443"
export GOSILO_BASE_URL="https://storage.example.com"
export GOSILO_TLS_MODE="auto"

gosilo
```

Requirements:
- Your domain's DNS points to the server
- Port 443 is open and reachable from the internet
- The certificate cache directory (`./certs` by default) is writable

Certificates are automatically obtained and renewed.

## Manual TLS

If you have your own certificates:

```bash
export GOSILO_TLS_MODE="manual"
export GOSILO_TLS_CERT="/etc/ssl/certs/storage.example.com.pem"
export GOSILO_TLS_KEY="/etc/ssl/private/storage.example.com.key"
```

## Behind a Reverse Proxy

Run gosilo behind nginx, Caddy, or another proxy. Disable TLS in gosilo and let the proxy handle it:

```bash
export GOSILO_ADDR="127.0.0.1:8080"
export GOSILO_BASE_URL="https://storage.example.com"
export GOSILO_TLS_MODE="off"
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

Create `/etc/systemd/system/gosilo.service`:

```ini
[Unit]
Description=gosilo remoteStorage server
After=network.target

[Service]
Type=simple
User=gosilo
Group=gosilo
WorkingDirectory=/opt/gosilo
ExecStart=/usr/local/bin/gosilo
Restart=on-failure
RestartSec=5
EnvironmentFile=/opt/gosilo/.env

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/gosilo

[Install]
WantedBy=multi-user.target
```

Set up the service user and directory, then enable it:

```bash
sudo useradd -r -s /usr/sbin/nologin gosilo
sudo mkdir -p /opt/gosilo
sudo chown gosilo:gosilo /opt/gosilo
sudo cp gosilo /usr/local/bin/
sudo cp .env /opt/gosilo/.env

sudo systemctl daemon-reload
sudo systemctl enable --now gosilo
```

## Docker

```dockerfile
FROM alpine:latest
COPY gosilo-linux-amd64 /usr/local/bin/gosilo
RUN chmod +x /usr/local/bin/gosilo
EXPOSE 8080
VOLUME ["/data"]
WORKDIR /data
CMD ["gosilo"]
```

```bash
docker build -t gosilo .
docker run -d \
  -p 8080:8080 \
  -v gosilo-data:/data \
  -e GOSILO_BASE_URL="https://storage.example.com" \
  gosilo
```

## Verify Your Setup

Before going to production, run `gosilo check` to verify your configuration:

```bash
gosilo check
```

This validates all settings and tests connectivity to the database and blob store.

## Database in Production

### SQLite

Works well for personal and small-group use. Make sure the database directory is on a filesystem that supports file locking (avoid NFS). SQLite is the default and requires no setup.

### PostgreSQL

For larger deployments or when you need high availability:

```bash
export GOSILO_DB="postgres:host=db.example.com dbname=gosilo user=gosilo password=secret sslmode=require"
export GOSILO_BLOB="fs:/var/lib/gosilo/blobs"
```

Using `fs:` for blob storage alongside PostgreSQL keeps large files on disk instead of in the database.

## Backups

### SQLite

Back up the database files (stop the server first for a consistent snapshot, or use SQLite's backup API):

```bash
cp /opt/gosilo/gosilo.db /backups/gosilo-$(date +%F).db
cp /opt/gosilo/gosilo-blobs.db /backups/gosilo-blobs-$(date +%F).db
```

### Filesystem Blob Storage

If you use `fs:` for blobs, include that directory in your regular backup schedule alongside the metadata database.
