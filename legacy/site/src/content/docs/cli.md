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

This is equivalent to `rstash serve`. On first run, visit the server in your browser to complete setup (create admin account). All configuration is via [environment variables](/docs/configuration/).

## `rstash env`

Print a documented `.env` template with all available configuration variables.

```bash
rstash env          # print to stdout
rstash env > .env   # save to file
```

Each variable is printed with a description, valid values (if restricted), and its default — all commented out so defaults take effect. Edit the file and uncomment what you need.

## `rstash check`

Validate your configuration and test connectivity to all configured backends.

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

Checks performed:

- **Config validation** — verifies all environment variables are well-formed (valid URLs, DSN formats, listen address, TLS settings)
- **Database connection** — opens the metadata database and runs a test query
- **Blob store connection** — opens the blob store backend
- **Email provider** — if `RSTASH_EMAIL` is set, verifies the provider is reachable
- **TLS certificates** — if `RSTASH_TLS_MODE=manual`, checks that the cert and key files exist

Run this before going to production to catch configuration issues early.
