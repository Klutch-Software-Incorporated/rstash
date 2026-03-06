---
title: CLI Reference
description: All gosilo commands and their usage.
order: 5
---

gosilo's CLI is intentionally minimal. The server is managed through the web UI — the CLI just starts it and provides a couple of diagnostic tools.

## `gosilo`

Start the remoteStorage server. This is the default command — just run `gosilo` with no arguments.

```bash
gosilo
```

On first run, visit the server in your browser to complete setup (create admin account). All configuration is via environment variables.

## `gosilo env`

Print a documented `.env` template with all available configuration variables.

```bash
gosilo env          # print to stdout
gosilo env > .env   # save to file
```

Useful for seeing every available setting with its default value and description.

## `gosilo check`

Validate your configuration and test connectivity to the database and blob store.

```bash
gosilo check
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
