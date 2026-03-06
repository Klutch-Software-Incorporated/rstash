#!/bin/bash
set -euo pipefail

apt-get update
apt-get install -y podman

mkdir -p /srv/site /etc/caddy /etc/containers/systemd

# Config files
cp /tmp/Caddyfile /etc/caddy/Caddyfile

# Pull caddy image directly on the server
podman pull docker.io/library/caddy:2-alpine

# Deploy Quadlet files
cp /tmp/caddy.container /etc/containers/systemd/
cp /tmp/caddy-data.volume /etc/containers/systemd/
cp /tmp/caddy-config.volume /etc/containers/systemd/

systemctl daemon-reload
systemctl start caddy
