# Tasks: cicd-github-actions

Order: Dockerfiles before workflows (workflows depend on them); .dockerignore with its Dockerfile.

- [x] 1. [backend] Create `procrastinator-backend/Dockerfile` and `procrastinator-backend/.dockerignore`
  Owns: `procrastinator-backend/Dockerfile`, `procrastinator-backend/.dockerignore`
  Multi-stage: `golang:<go.mod version>-alpine` build stage (`CGO_ENABLED=0 go build -o /out/procrastinator ./api/cmd/procrastinator`), runtime `gcr.io/distroless/static-debian12:nonroot` with `COPY migrations/ /app/migrations/`, `WORKDIR /app`, binary at `/procrastinator`; distroless ships CA certs. .dockerignore per design D6 (excludes *.exe, logs, .env, storage/, server.*). Verify: `docker buildx build --platform linux/amd64,linux/arm64 procrastinator-backend` succeeds.

- [x] 2. [ui] Create `ui/Dockerfile`, `ui/nginx.conf`, `ui/.dockerignore`
  Owns: `ui/Dockerfile`, `ui/nginx.conf`, `ui/.dockerignore`
  Multi-stage: `node:22-alpine` (`npm ci`, `npm run build`) → `nginx:alpine` copying `dist/` to `/usr/share/nginx/html/`. nginx.conf: SPA `try_files $uri /index.html`, gzip, hashed-asset cache headers. .dockerignore excludes node_modules/, dist/, logs, .tmp/. Verify: amd64+arm64 build succeeds; container serves index.html at / and a deep link resolves through SPA fallback.

- [x] 3. [backend] Create `.github/workflows/backend.yaml`
  Owns: `.github/workflows/backend.yaml`
  Per design D4/D5: triggers `push` paths `procrastinator-backend/**` + `workflow_dispatch`; `permissions: contents: read, packages: write`; steps: checkout@v4, setup-go@v5, `go vet ./...` and `go test ./...` with `working-directory: procrastinator-backend`, setup-qemu-action, setup-buildx-action, HAS_REGISTRY gate env, docker/login-action (if secrets), docker/build-push-action@v6+ with `context: procrastinator-backend`, `platforms: linux/amd64,linux/arm64`, `cache-from: type=gha`, `cache-to: type=gha,mode=max`, push gated on secrets (load-only build when absent). No hardcoded registry literals. Verify: `actionlint` or workflow syntax check; scenario walkthroughs from spec.

- [x] 4. [ui] Create `.github/workflows/ui.yaml`
  Owns: `.github/workflows/ui.yaml`
  Same shape: paths `ui/**` + dispatch; setup-node@v4 (node 22), steps `npm ci`, `npm run lint`, `npm run test`, `npm run build` (cwd `ui`), then docker build/push with `context: ui` per D4/D5. No hardcoded registry literals. Verify: syntax check; spec scenarios.

- [x] 5. [shared] Document the registry-as-secrets contract as an open config item
  Owns: `openspec/changes/cicd-github-actions/config.md` (new note file accompanying the change)
  Records: required secrets (`REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`), where to set them (repo Settings → Secrets), that the registry host is open (reference: `registry.yelkawar.com`), and the image naming/tag scheme (`<registry>/<name>:latest`, `:<sha>`). Verify: no secret literal values in file; tasks + proposal reference it.
