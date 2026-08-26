# syntax=docker/dockerfile:1

# ── Build ──
# Pinned to the *build* platform so the SDK always runs natively and cross-publishes to
# the target. Letting an arm64 SDK run under QEMU instead turns a two-minute build into
# a twenty-minute one, and is the usual reason multi-arch .NET images get abandoned.
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:10.0 AS build

# Supplied by buildx, one value per --platform entry: amd64 | arm64.
ARG TARGETARCH
# Set to the tag ("v0.5.0") for a release build, and left empty otherwise — an empty
# value falls through to the Directory.Build.props default, so an unofficial image
# reports "v0.5.0+dev" without this file needing to know what the version is.
ARG VERSION=

WORKDIR /src
COPY . .

# .NET spells x86-64 "x64" where Docker spells it "amd64"; arm64 agrees with itself.
RUN RID="linux-$(echo "$TARGETARCH" | sed 's/^amd64$/x64/')" \
    && dotnet publish src/Rstash.Server/Rstash.Server.csproj \
        -c Release \
        -r "$RID" \
        --no-self-contained \
        ${VERSION:+-p:InformationalVersion="$VERSION"} \
        -o /app

# Created here, with a shell, because the chiseled runtime image has none. Copying it
# across with --chown is what gives a fresh named volume the right ownership.
RUN mkdir -p /data

# ── Runtime ──
# Chiseled: no shell, no package manager, and non-root (uid 1654) out of the box. The
# plain variant ships no ICU, which is fine because Directory.Build.props already sets
# InvariantGlobalization for every project — if that ever changes, this needs "-extra".
FROM mcr.microsoft.com/dotnet/aspnet:10.0-noble-chiseled

WORKDIR /app
COPY --from=build /app ./

# SQLite data lives here by default; mount a volume for persistence, or point
# RSTASH_DB / RSTASH_BLOB at Postgres/S3/Azure for production.
#
# NOTE: rstash runs as uid 1654, not root. A *named volume* inherits this directory's
# ownership and just works. A *bind mount* keeps the host directory's owner, so
# `chown -R 1654:1654 ./data` on the host first, or the server cannot write its database.
COPY --from=build --chown=$APP_UID:$APP_UID /data /data
VOLUME /data

# No ASPNETCORE_URLS: rstash binds from RSTASH_ADDR (default :8080) and picks the scheme
# from RSTASH_TLS_MODE, so it calls UseUrls itself and would override anything set here.
ENV RSTASH_DB=sqlite:/data/rstash.sqlite \
    RSTASH_BLOB=sqlite:/data/rstash-blobs.sqlite

EXPOSE 8080

# Exec form is mandatory: there is no shell in this image to parse the string form.
# The start period covers first-boot schema migration against a cold database.
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD ["/app/Rstash.Server", "healthcheck"]

ENTRYPOINT ["/app/Rstash.Server"]
