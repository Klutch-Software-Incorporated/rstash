# syntax=docker/dockerfile:1

# ── Build ──
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src
COPY . .
RUN dotnet publish src/Rstash.Server/Rstash.Server.csproj -c Release -o /app

# ── Runtime ──
FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app ./

# SQLite data lives here by default; mount a volume for persistence, or point
# RSTASH_DB / RSTASH_BLOB at Postgres/S3/Azure for production.
ENV ASPNETCORE_URLS=http://+:8080 \
    RSTASH_DB=sqlite:/data/rstash.sqlite \
    RSTASH_BLOB=sqlite:/data/rstash-blobs.sqlite
VOLUME /data
EXPOSE 8080

ENTRYPOINT ["./Rstash.Server"]
