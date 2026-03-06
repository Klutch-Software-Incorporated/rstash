---
title: CLI Reference
description: All rstash commands and their usage.
order: 5
---

rstash's CLI is intentionally minimal. The server is managed through the web UI — the CLI just starts it and provides a couple of diagnostic tools.

## `rstash`

Start the remoteStorage server. This is the default command — just run `rstash` with no arguments.

```bash
rstash
```

On first run, visit the server in your browser to complete setup (create admin account). All configuration is via environment variables.

## `rstash env`

Print a documented `.env` template with all available configuration variables.

```bash
rstash env          # print to stdout
rstash env > .env   # save to file
```

Useful for seeing every available setting with its default value and description.

## `rstash check`

Validate your configuration and test connectivity to the database and blob store.

```bash
rstash check
```

Output:

```
Checking configuration...

  OK    Config validation
  OK    Database connection
  OK    Blob store connection

3 passed, 0 failed
```

If TLS is configured with manual certificates, it also checks that the cert and key files exist. Useful for verifying your setup before going to production.
