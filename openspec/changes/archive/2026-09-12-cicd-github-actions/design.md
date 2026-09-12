# Design: cicd-github-actions

## Context

Monorepo with two standalone units:
- `procrastinator-backend/` — one Go module (module name `procrastinator-backend`, `go.mod` at that dir). Entrypoint: `api/cmd/procrastinator/main.go`. DB migrations live in `procrastinator-backend/migrations/` (goose, read from disk at runtime via the `-migrations` flag, default `migrations` relative to CWD).
- `ui/` — React/Vite SPA (`npm run build` = `tsc -b && vite build`, PWA assets) with existing scripts `lint` (eslint), `test` (vitest run --passWithNoTests).

No CI/CD exists today. Registry host/credentials are repo secrets, never committed. Patterns proven in the JobOptimizer sibling repo: docker/setup-buildx-action + tonistiigi/binfmt, docker/build-push-action with GHA cache, `permissions: packages: write`.

## Goals / Non-Goals

**Goals:**
- Two GitHub Actions workflows (`backend.yaml`, `ui.yaml`) with path-filtered triggers + workflow_dispatch.
- Two committed multi-stage Dockerfiles, both buildable on linux/amd64 and linux/arm64.
- GHA-cached multi-platform builds; conditional push gated on secret presence.
- Registry-as-secrets contract, documented as an open configuration item.

**Non-Goals:**
- No deployment/CD to any host, no release tagging/versioning, no artifact attestations.
- No new lint frameworks, coverage gates, or matrix jobs.
- No integration-test runner in CI (DB-backed Go tests self-skip — see Risks).

## Decisions

### D1 — Go commands run at the module root: `go vet ./...` / `go test ./...` (working-directory: procrastinator-backend)

The module root IS `procrastinator-backend/`; `api/cmd/procrastinator` is a package path inside it. `go vet ./...` from the module root resolves the entrypoint package as `api/cmd/procrastinator` — there is no separate module under `api/`, so `go vet ./...` is the correct, complete invocation. `go vet ./api/...` would be a subset (misses `config`, `commons`, `core`, `infra`) with no benefit. Vet and test are separate steps in the same job so failures short-circuit the Docker build (spec: "fail and skip dependent steps"). Verified in codebase: DB-backed tests self-skip when `PROCRASTINATOR_TEST_DATABASE_URL` is unset (guarded via `TESTPG_SKIP` in test mains and `t.Skip` in e2e), so CI needs no Postgres service.

### D2 — Backend runtime assets: migrations/ + CA certs; no openapi.yaml

The image needs exactly:
1. **Binary** `/procrastinator` — built with `CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/procrastinator ./api/cmd/procrastinator` (pure Go; nothing else in the module is a command; `seed/main.go` is a dev-only tool, excluded).
2. **`migrations/` directory** — goose reads SQL files from disk at startup (`store.Migrate(ctx, migrationsDir)`); without it the container fatals on boot. Copy `migrations/ → /app/migrations/`. Default flag value `migrations` + WORKDIR `/app` resolves it with zero config.
3. **CA certificates** — pgx connects over Postgres TLS when DATABASE_URL uses `sslmode=require|verify-full`; the runtime stage must ship system CA roots.

**openapi.yaml is NOT bundled**: it is used only by oapi-codegen generation and drift tests at build time; the runtime serves generated Go handlers. Bundling it would be dead weight.
`.env` is NOT bundled (godotenv treats absence as non-fatal; config comes from real env vars in the deployment environment).

**Base images:** `golang:1.x-alpine` builder (small, matches repo style; pinned by digest in follow-up hardening) → `gcr.io/distroless/static-debian12:nonroot` runtime (includes ca-certificates, no shell, minimal surface). Alternative considered: `alpine + apk add ca-certificates` — rejected (larger, shell attack surface) and `distroless/runtime` (needs shell for goose? no — goose runs in-process). QEMU note: `CGO_ENABLED=0` cross-compilation means the build stage does NOT need emulation; only the final image copy stage runs per-platform (cheap). Register arm64 binfmt anyway per spec so any future CGO dependency doesn't break builds.

### D3 — UI runtime: nginx:alpine + SPA fallback config

Builder: `node:22-alpine` (package.json tooling requires Node >= 22.12), `npm ci`, `npm run build`. Runtime: `nginx:alpine`, copy `dist/ → /usr/share/nginx/html/` plus a minimal `ui/nginx.conf` — `try_files $uri /index.html` SPA fallback (React Router deep links must not 404) + gzip, cache long-lived hashed assets from vite's default fingerprinting. **ui/nginx.conf is a new committed file in this change**; the spec's nginx-serve requirement implicitly requires it for a functional image.

### D4 — Workflow shape (both files follow one template, grounded on JobOptimizer patterns)

Per-file layout (e.g. backend):

```yaml
name: backend CI
on:
  workflow_dispatch:
  push:
    paths: ['procrastinator-backend/**']

permissions:
  contents: read
  packages: write        # for GHCR fallback; harmless otherwise

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5          # backend only  /  actions/setup-node@v4 node-version: 22 (ui only)
      - go vet ./...                        # (module cwd) /  npm ci, npm run lint, npm run test, npm run build (ui only)
      - go test ./...
      - uses: docker/setup-qemu-action@v3
      - uses: docker/setup-buildx-action@v3
      - uses: docker/login-action@v3        # ONLY when secrets present (see D5)
        if: needs-push
      - uses: docker/build-push-action@v6   # push per D5
```

- Separate `setup-qemu-action@v3` (cleaner than ad-hoc `tonistiigi/binfmt` docker run) + `setup-buildx-action@v3`; `platforms: linux/amd64,linux/arm64`.
- Steps use explicit `working-directory` (backend: `procrastinator-backend`, ui: `ui`); go commands at module root per D1.
- No `buildkitd.toml`, no artifact upload, no coverage steps — boyscout scope.

### D5 — Registry-as-secrets contract & push gating

Secrets: `secrets.REGISTRY_URL`, `secrets.REGISTRY_USERNAME`, `secrets.REGISTRY_PASSWORD`. GHA can't detect secret presence directly, so a job-level pre-step resolves it:

```yaml
env:
  HAS_REGISTRY: ${{ secrets.REGISTRY_URL != '' && secrets.REGISTRY_USERNAME != '' && secrets.REGISTRY_PASSWORD != '' }}
push: ${{ env.HAS_REGISTRY == 'true' }}          # on build-push-action
tags: ${{ env.HAS_REGISTRY == 'true' && format('{0}/{1}:{2}', secrets.REGISTRY_URL, image, sha) || '' }}
```

Concretely, two build-push steps or one with `push: ${{ env.HAS_REGISTRY }}` + `load: ${{ !env.HAS_REGISTRY }}`:
- secrets present: multi-platform build with `push: true`, `cache-from: type=gha`, `cache-to: type=gha,mode=max`, tags `REGISTRY_URL/<name>:latest` and `REGISTRY_URL/<name>:<sha>`.
- secrets absent: same build with `push: false, load: true` (single-platform `linux/amd64` for load semantics; cache-from still applies so CI stays fast).

Secrets consumed via `docker/login-action` env (`REGISTRY: ${{ secrets.REGISTRY_URL }}`, `username: ${{ secrets.REGISTRY_USERNAME }}`, `password: ${{ secrets.REGISTRY_PASSWORD }}`) — never echoed into logs, never hardcoded. Image names: `procrastinator-backend` and `procrastinator-ui` (approx; final kebab name fixed in tasks).

Note: the spec names `docker/build-push-action@v7` as the reference pattern; `v6`/`v7` both support the full flag set used here — tasks pin to the major current at implementation time.

### D6 — .dockerignore files

Both contexts must exclude local runtime junk that would bloat or break builds:
- `procrastinator-backend/.dockerignore`: `*.exe`, `*.log`, `.env`, `storage/`, `server.*`, `.run*`, `infra/postgres/dev*` — keeps context small and prevents credentials (`.env`) entering image layers.
- `ui/.dockerignore`: `node_modules/`, `dist/`, logs, `.tmp/`.

## Risks / Trade-offs

- [DB-backed Go tests wrongly skipped in CI] → They self-skip by design (`TESTPG_SKIP`); accepted for this change (spec scopes CI to vet+test without services). Adding a Postgres service container later is a one-job change.
- [`go test ./...` includes package `seed` (dev tool)] → It has no tests; vet still covers it. Harmless.
- [Multi-platform + GHA cache misses on architecture] → `cache-to: mode=max` stores per-arch layers (proven pattern in JobOptimizer).
- [QEMU binfmt unavailability on runner] → standard GitHub runners include binfmt support via setup-qemu-action; also mitigated by CGO_ENABLED=0 cross-compile (no emulation needed in build stage).
- [Larger images than distroless for UI] → nginx:alpine (~50MB) accepted; static-site alternatives add config complexity for no repo need.

## Migration Plan

1. Merge Dockerfiles + workflows + .dockerignore + ui/nginx.conf.
2. Faithful CI runs (green, local-build-only) prove ergonomics.
3. At deployment: set repo secrets `REGISTRY_URL/USERNAME/PASSWORD` → next push (or manual dispatch) pushes images.

## Open Questions

- Final registry host (open config item; JobOptimizer's `registry.yelkawar.com` is a reference, not a decision).
- Go version pin for `golang:1.x-alpine` build stage — read from `go.mod` at implementation time (`golang:${major.minor}-alpine`).
