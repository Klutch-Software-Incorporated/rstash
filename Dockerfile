# syntax=docker/dockerfile:1

# ── Build ──
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY . .
# The ADO image pipeline passes --build-arg VERSION=<git describe> (e.g. v0.4.1).
# Declared here so it's consumed cleanly; baking it into the assembly
# (InformationalVersion) is a follow-up — the C# app has no --version reader yet.
ARG VERSION=
RUN dotnet publish src/Rstash.Server/Rstash.Server.csproj -c Release -o /app

# ── Runtime ──
FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app ./

# SQLite data lives here by default; mount a volume for persistence, or point
# RSTASH_DB / RSTASH_BLOB at Postgres/S3/Azure for production.
#
# No ASPNETCORE_URLS: rstash binds from RSTASH_ADDR (default :8080) and picks the scheme
# from RSTASH_TLS_MODE, so it calls UseUrls itself and would override anything set here.
ENV RSTASH_DB=sqlite:/data/rstash.sqlite \
    RSTASH_BLOB=sqlite:/data/rstash-blobs.sqlite
VOLUME /data
EXPOSE 8080

ENTRYPOINT ["./Rstash.Server"]
