# Contributing to rstash

## Prerequisites

- [.NET 10 SDK](https://dotnet.microsoft.com/download)
- Git

rstash is a C#/.NET 10 application. (The original Go implementation is preserved
under `legacy/` as a behavioral reference; it is not built or shipped.)

## Contribution workflow

rstash is developed on [GitHub](https://github.com/Klutch-Software-Incorporated/rstash). To contribute:

1. Fork the repository and clone your fork.
2. Create a branch off `main` for your change.
3. Make your change; run `dotnet build` and `dotnet test` before pushing.
4. Open a pull request against `main`. CI (build, test, secret scan) runs automatically on every PR.

A maintainer will review; once CI is green and the change is approved, it'll be merged.

## Getting started

```sh
dotnet build Rstash.slnx                    # Build the solution
dotnet run --project src/Rstash.Server       # Run the server (default: http://localhost:8080)
dotnet test Rstash.slnx                      # Run all tests
```

The server's CLI subcommands short-circuit before the web host:

```sh
dotnet run --project src/Rstash.Server -- env     # Print an env-var template
dotnet run --project src/Rstash.Server -- check   # Validate config + DB/blob connectivity
dotnet run --project src/Rstash.Server -- seed    # Populate an account with sample data
```

## Running tests

```sh
# All tests
dotnet test Rstash.slnx

# A single test project
dotnet test tests/Rstash.Core.Tests
dotnet test tests/Rstash.IntegrationTests

# A single test by name
dotnet test --filter "FullyQualifiedName~TestName"
```

`Rstash.Core.Tests` are unit tests over Model/Services/Storage/Database;
`Rstash.IntegrationTests` exercise the host end-to-end via `WebApplicationFactory`.

## Database migrations

Schema is managed with EF Core code-first migrations compiled into the assembly
and applied at startup. To add one:

```sh
dotnet dotnet-ef migrations add <Name> -p src/Rstash.Database -s src/Rstash.Database -o Migrations
```

(`dotnet-ef` is pinned as a local tool in `.config/dotnet-tools.json`; run
`dotnet tool restore` first if needed.)

## Hot reload

```sh
dotnet watch --project src/Rstash.Server
```

Recompiles and reloads on source/Razor/CSS changes during development.

## Single-file publish

```sh
dotnet publish src/Rstash.Server -c Release -r <rid> --self-contained true \
  -p:PublishSingleFile=true -p:IncludeNativeLibrariesForSelfExtract=true
```

where `<rid>` is e.g. `linux-x64`, `win-x64`, or `osx-arm64`. A container image
is also provided via the root `Dockerfile`.

## Project structure & conventions

See the Solution Layout and Key Conventions in [CLAUDE.md](CLAUDE.md), and the
parity backlog in [docs/PARITY-GAPS.md](docs/PARITY-GAPS.md).
