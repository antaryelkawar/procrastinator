# Proposal: CI/CD via GitHub Actions for procrastinator

## Why

The procrastinator monorepo (backend Go service + React/Vite UI) has no CI/CD. Builds, tests, and container images are manual. Adding GitHub Actions workflows plus committed Dockerfiles gives:

- Automated verification (tests, typecheck/lint) on every push touching each service.
- Reproducible, multi-platform container images published to a container registry.
- Fast builds via GHA layer caching (pattern proven in JobOptimizer sibling repo).

## What Changes

1. **New Dockerfiles** (committed, reusable):
   - `procrastinator-backend/Dockerfile` — multi-stage Go build (build stage → minimal runtime image, e.g. `distroless` base or alpine with CA certs for pgx TLS).
   - `ui/Dockerfile` — multi-stage: node builder (runs `npm ci`, `tsc -b && vite build`) → nginx serving the `dist/` static output.
2. **Backend workflow** `.github/workflows/backend.yaml`:
   - Triggers: push (path filter `procrastinator-backend/**`) + workflow_dispatch.
   - Jobs: `go vet ./...`, `go test ./...`, multi-platform (`linux/amd64,linux/arm64`) Docker build with buildx + QEMU, GHA cache (`type=gha` / `mode=max`), push to registry.
3. **UI workflow** `.github/workflows/ui.yaml`:
   - Triggers: push (path filter `ui/**`) + workflow_dispatch.
   - Jobs: `npm ci`, lint (`eslint .`), test (`npm run test`), build (`npm run build`), multi-platform Docker build + push as above.
4. **Registry configuration:** registry host and credentials are NOT hardcoded. The workflows must read them from repo secrets (`secrets.REGISTRY_URL`, `secrets.REGISTRY_USERNAME`, `secrets.REGISTRY_PASSWORD`); push steps are gated on secret availability. The actual registry host is an open configuration item (JobOptimizer uses `registry.yelkawar.com`, but that is not specified for this repo). See `openspec/changes/cicd-github-actions/config.md` for the registry-as-secrets contract.

## Impact

- **New files:** two Dockerfiles, two workflow files. No existing code changes.
- **Secrets required (repo-level):** `REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` — to be configured at deployment time.
- **Scope guard:** boyscout only — no new lint frameworks, no speculative services, no mono-lint workflow beyond what package/go tooling already provides.
