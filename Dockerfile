# syntax=docker/dockerfile:1
#
# rstash container image.
#
# Multi-stage: the builder stage has the Go + Node toolchain needed to
# embed the Astro docs site and generate license metadata, then produces
# a statically linked binary. The runtime stage is distroless — no shell,
# no package manager, runs as non-root — containing only the binary.
#
# We inline the site-embed and licenses-collect commands here rather than
# depending on the Taskfile, so the image build is self-contained and
# doesn't pull in task/bash/curl just to drive a two-line command. Keep
# these commands in sync with Taskfile.yml's `build` dependencies
# (collect-licenses, site:embed) if they ever change.

# ---------- builder ----------

FROM docker.io/library/golang:1.24-alpine AS builder

WORKDIR /src

# Node is needed to build the embedded Astro site. Nothing else.
RUN apk add --no-cache nodejs npm

# Cache Go modules first — this layer only busts when go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Cache npm deps the same way.
COPY site/package.json site/package-lock.json ./site/
RUN cd site && npm ci

# Full source.
COPY . .

# Version baked into the binary via -ldflags. The ADO image pipeline passes
# --build-arg VERSION=<git describe> (.git is excluded from the build context,
# so the binary's VCS fallback can't fire here — the build-arg is the source of
# truth). Left empty by default: an un-injected manual `docker build` then
# falls through to config.Version's "unknown", never a misleading "dev".
ARG VERSION=

# Generate embedded licenses.json.
RUN go run ./cmd/collect-licenses

# Build the Astro site into internal/ui/site for go:embed.
RUN cd site && npx astro build --outDir ../internal/ui/site \
 && touch ../internal/ui/site/.keep

# Statically linked, stripped. glebarez/sqlite is pure Go, so CGO_ENABLED=0
# is fine everywhere rstash runs.
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-s -w -X rstash/internal/config.Version=${VERSION}" \
    -o /out/rstash .

# ---------- runtime ----------

FROM gcr.io/distroless/static-debian12:nonroot

# The distroless :nonroot tag runs as UID 65532 by default — non-root
# from the start, no USER directive needed.
WORKDIR /app
COPY --from=builder /out/rstash /app/rstash

# Default listen address. Operators override via RSTASH_ADDR / PORT.
EXPOSE 8080

# distroless has no shell, so ENTRYPOINT must be exec-form.
ENTRYPOINT ["/app/rstash"]
