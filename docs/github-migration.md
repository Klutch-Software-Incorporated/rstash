# Migrating rstash to GitHub

Plan for moving the **rstash application repo** from Azure DevOps to GitHub
(`git@github.com:Klutch-Software-Incorporated/rstash.git`) so outside
contributors can participate, without exposing any of the private rstash.cloud
deploy infrastructure.

Status: **planning** (no steps executed yet).

## Guiding principle

The dependency arrow only ever points **ADO → GitHub**.

- The **public GitHub repo** is fully self-contained: source, CI, and release
  binaries. It holds **zero** Azure credentials and never reaches into
  rstash.cloud or its deploys.
- The **private Azure DevOps side** *pulls from* GitHub: it watches the GitHub
  repo and runs the image build + ACR push + App Service deploy. Nothing in
  GitHub authenticates to Azure.

This keeps the blast radius of a public repo to zero secrets, and means a
malicious fork PR can never trigger or touch a deploy.

## Target architecture

| Concern | Today (ADO) | After migration |
|---|---|---|
| Source of truth | ADO Git (`dev.azure.com/.../rstash`) | **GitHub** (`Klutch-Software-Incorporated/rstash`), hard move |
| CI (build/vet/test) | `azure-pipelines.yml` BuildTest stage | **GitHub Actions** `.github/workflows/ci.yml` (runs on PRs incl. forks) |
| Release binaries | Azure Artifacts Universal feed `rstash/rstash` | **GitHub Releases** (5 cross-compiled binaries per tag) |
| PR validation | ADO branch policy on `main` | **GitHub branch protection** (required CI check + review) |
| Docker image → ACR | `azure-pipelines.yml` PublishFeed stage | **ADO pipeline in `rstash-infra`** that checks out GitHub as a resource and pushes to ACR |
| Approval gate | ADO Environment `rstash-artifact-feed` | ADO Environment (unchanged, lives with the deploy pipeline) |
| `rstash-infra` repo | ADO Git | **Stays on ADO** (private, Azure-coupled, holds billing secrets — no reason to move) |

```
   contributors ──PR──▶  GitHub: rstash (public)
                          ├─ Actions: CI (build/vet/test) on every PR
                          └─ Actions: Release (binaries) on tag
                                     │
                                     │  ADO watches GitHub (read-only
                                     │  GitHub service connection)
                                     ▼
   Azure DevOps (private) ── deploy pipeline (in rstash-infra) ──▶ ACR ──▶ App Service
```

## Versioning

Two version namespaces, both CalVer, both replacing ADO's old
`$(Date:yyyy)$(Date:MM)$(Date:dd).$(Rev:r)` build number:

- **Public binaries (tag-driven).** Maintainer pushes an annotated tag
  `vYYYY.MMDD.N` (e.g. `v2026.0610.1`); GitHub Actions Release cross-compiles
  the 5 binaries and attaches them to the GitHub Release. The version flows into
  the binary via the existing `-X rstash/internal/config.Version=` ldflag
  (`internal/config/version.go`); `task build VERSION=...` (already templated)
  is reused by the workflow.
- **Deployed image (main-driven).** The ADO image pipeline triggers on pushes to
  GitHub `main` and stamps a CalVer `$(Date:yyyy).$(Date:MM)$(Date:dd).$(Rev:r)`
  — a 1:1 port of the old pipeline's behavior. It tags the image with that
  version + `latest` and pushes to ACR.

Why not tag-drive the image too: ADO repository-resource **tag** triggers on
GitHub are unreliable, whereas branch triggers are solid. Tracking `main` for the
image also matches exactly how the old pipeline behaved (it triggered on `main`,
gated by approval). App Service can still pin a specific dated image tag if you
don't want `latest`.

---

## Phase 1 — GitHub repo bring-up (hard move)

1. **Create the GitHub repo** `Klutch-Software-Incorporated/rstash` (public,
   no auto-generated README/LICENSE — we already have ours).
2. **Re-point the local remote** (hard move; ADO repo set read-only/archived):
   ```sh
   git remote rename origin ado            # keep ADO reachable during cutover
   git remote add origin git@github.com:Klutch-Software-Incorporated/rstash.git
   git push -u origin main
   git push origin --tags                  # if any tags exist
   ```
   Optionally drop the `ado` remote once cutover is verified.
3. **Branch protection on `main`** (Settings → Branches): require a PR,
   require the `CI` status check to pass, require at least one review,
   dismiss stale approvals. This replaces the ADO build-validation branch policy.
4. **OSS scaffolding** (none of this exists yet except LICENSE/CONTRIBUTING/README):
   - `.github/workflows/ci.yml`, `.github/workflows/release.yml` (Phases 2–3)
   - `.github/ISSUE_TEMPLATE/` (bug + feature) and `PULL_REQUEST_TEMPLATE.md`
   - `SECURITY.md` (point reports at curtis.la.graff@klutch.software, not public issues)
   - `CODE_OF_CONDUCT.md` (optional)
5. **Remove `azure-pipelines.yml` from the app repo** — its BuildTest role moves
   to Actions and its deploy role moves to the infra repo. Delete it as part of
   the same PR that adds the Actions workflows.

## Phase 2 — GitHub Actions CI

`.github/workflows/ci.yml` — mirrors the ADO BuildTest stage. Runs on every push
and PR (including forks; needs no secrets):

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:
jobs:
  build-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24.0', cache: true }
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: site/package-lock.json }
      - uses: arduino/setup-task@v2
        with: { repo-token: ${{ secrets.GITHUB_TOKEN }} }
      - run: go mod download
      - run: npm ci
        working-directory: site
      - run: task vet
      - run: task test
      - run: go build ./...
```

Notes:
- Integration tests (S3/Azurite/etc.) stay opt-in via env vars, same as today —
  CI leaves them skipped.
- This is the check name (`CI` / `build-test`) wired into branch protection.

## Phase 3 — GitHub Releases (binaries)

`.github/workflows/release.yml` — replaces the Azure Artifacts Universal feed.
Triggered by a `v*` tag:

```yaml
name: Release
on:
  push:
    tags: ['v*']
permissions:
  contents: write          # needed to create the Release
jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24.0', cache: true }
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm', cache-dependency-path: site/package-lock.json }
      - uses: arduino/setup-task@v2
        with: { repo-token: ${{ secrets.GITHUB_TOKEN }} }
      - run: npm ci
        working-directory: site
      - name: Build release binaries
        run: task build VERSION="${GITHUB_REF_NAME#v}"
      - uses: softprops/action-gh-release@v2
        with:
          files: dist/rstash-*
          generate_release_notes: true
```

Follow-ups:
- Update `README.md` download instructions (currently the Fossil `uv` URL) to
  point at `https://github.com/Klutch-Software-Incorporated/rstash/releases`.
- The `Taskfile.yml` `unversioned`/`release` tasks push binaries to **Fossil** —
  now dead. Either delete them or repoint to `gh release upload`. (`task build`
  itself is unchanged and reused by the workflow above.)

## Phase 4 — Re-home the deploy (ADO pulls from GitHub)

The image build + ACR push moves **into the `rstash-infra` ADO project**, so all
Azure/deploy concerns live on the private side and the GitHub repo stays clean.

1. **GitHub service connection in ADO** (Project Settings → Service connections →
   GitHub): a **read-only** connection (GitHub App install scoped to the one repo,
   or a fine-grained PAT with `contents:read`). This is the one and only ADO↔GitHub
   link, and it only *reads* GitHub.
2. **Pipeline YAML — written:** `rstash-infra/pipelines/rstash-image.yml`. It
   declares the GitHub repo as a `resources.repositories` resource (endpoint
   `github.com_clagraff`), triggers on its `main` branch, `checkout: app`, and
   runs the two-step Docker `build` (with `--build-arg VERSION=$(Build.BuildNumber)`)
   + `push` to ACR via the existing `rstash-acr-eus2` service connection, keeping
   the `rstash-artifact-feed` Environment approval gate. Lifted from the old
   `azure-pipelines.yml` PublishFeed stage, minus the Universal Package publish
   (now GitHub Releases).
3. **Version** is the pipeline's own CalVer build number (`name:` →
   `$(Build.BuildNumber)`), fed to `--build-arg VERSION` and used as the image tag.

The deploy pipeline YAML lives in **`rstash-infra`** (decided). The old ADO app
repo will hold nothing once the deploy half moves out, so there's no reason to
keep a pipeline file there — `rstash-infra` becomes the single owner of every
Azure concern (Bicep provisioning + image build/deploy). It must **not** live in
the GitHub repo (that would point the dependency arrow the wrong way).

## Phase 5 — Decommission & cleanup

- **Azure DevOps app repo**: set read-only / archive once GitHub is verified as
  canonical. Disable its old build pipeline (the one driven by the now-deleted
  `azure-pipelines.yml`).
- **Azure Artifacts Universal feed** `rstash/rstash`: stop publishing; leave
  existing versions for history or delete. GitHub Releases is now the home.
- **Docs / source-of-truth references** (all currently say Fossil — two
  migrations stale):
  - `README.md:108` "Source control is managed via Fossil" → GitHub.
  - `README.md:23,30` Fossil `uv` download URLs → GitHub Releases.
  - `CONTRIBUTING.md:7` "Fossil (source control)" prerequisite → Git/GitHub +
    a short "fork → branch → PR" workflow section for outside contributors.
  - `CLAUDE.md` "Source control is managed via Git (Azure DevOps remote)" → GitHub.
  - Project memory `MEMORY.md` Source line → GitHub origin.
- **Skills**: the `azure-pr` skill (fetch ADO PR comments) and the Fossil-based
  `commit`/`ticket` skills no longer match the workflow. Replace PR review with a
  `gh`-CLI GitHub flow; retire or repoint the Fossil skills.

## One-time setup checklists

**GitHub side**
- [ ] Create public repo, push `main` + tags
- [ ] Branch protection on `main` (require CI check + 1 review)
- [ ] Add `.github/workflows/ci.yml` + `release.yml`
- [ ] Add issue/PR templates, `SECURITY.md`
- [ ] Verify a fork PR runs CI green
- [ ] Cut a test tag, confirm Release with 5 binaries

**Azure DevOps side**
- [ ] Create read-only GitHub service connection
- [ ] Add `rstash-image.yml` pipeline in `rstash-infra`, wire the GitHub repo resource + trigger
- [ ] Confirm ACR push + approval gate still work, sourced from GitHub
- [ ] Archive old ADO app repo + disable old pipeline
- [ ] Stop Universal feed publishes

## Risks / open questions

- **Fork PRs & CI cost**: CI on forks is free on public repos and needs no
  secrets — safe. The deploy never runs from a PR (it's ADO-side, tag-triggered).
- **Tag discipline**: releases + deploys are now tag-driven; a mistagged push
  ships a version. Consider restricting who can push tags (branch protection
  covers branches, not tags — use a tag protection rule or a maintainers-only
  release process).
- **ADO resource triggers on GitHub tags**: confirm ADO fires on GitHub *tag*
  pushes (not just branch commits) for the chosen trigger config; adjust the
  resource `trigger` filter accordingly.
- **Org slug** is confirmed `Klutch-Software-Incorporated/rstash`. Update the
  stale `klutch-software/rstash` link in `rstash-infra/README.md` to match.
```
