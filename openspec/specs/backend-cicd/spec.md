# Spec: backend-cicd

Automated continuous integration and containerization for the Go backend. Ensures code quality through automated testing and produces multi-platform Docker images for deployment.

## Purpose

Provide a reliable, automated pipeline that validates backend changes via `go vet` and `go test`, and packages the application into a secure, multi-platform Docker image. The pipeline supports both automated triggers on push and manual dispatch, with image publishing to a configured registry when credentials are provided.

## Requirements

### Requirement: Backend CI workflow triggers on backend changes
A GitHub Actions workflow SHALL exist at `.github/workflows/backend.yaml` that triggers on `push` events filtered by path `procrastinator-backend/**` and on `workflow_dispatch`. CI run steps SHALL be: `go vet ./...`, `go test ./...`, and a Docker image build, executed in the working directory `procrastinator-backend`. The workflow MUST NOT use steps irrelevant to the Go backend (no npm/node tooling).

#### Scenario: Backend-only change runs backend CI
- **WHEN** a commit containing changes only under `procrastinator-backend/` is pushed
- **THEN** the backend workflow runs, executing go vet, go test, and a Docker build

#### Scenario: Unrelated change does not run backend CI
- **WHEN** a commit containing changes only under `ui/` is pushed
- **THEN** the backend workflow does not run

#### Scenario: Manual dispatch
- **WHEN** a user triggers the backend workflow via `workflow_dispatch`
- **THEN** the full pipeline (vet, test, Docker build) executes

### Requirement: Backend CI gates on verified tests
The backend workflow SHALL fail (and skip dependent steps) if `go vet ./...` or `go test ./...` exits nonzero, including when the module compiles but tests fail.

#### Scenario: Failing test blocks build
- **WHEN** `go test ./...` reports test failures
- **THEN** the workflow run fails and the Docker build/push job does not run

### Requirement: Backend Dockerfile is multi-platform capable
A Dockerfile SHALL exist at `procrastinator-backend/Dockerfile` using a multi-stage build: a Go build stage (module `procrastinator-backend`, building `api/cmd/procrastinator/main.go` via `go build ./...` or the cmd path) followed by a minimal runtime stage (e.g. distroless or alpine/runc minimal base) that embeds the built binary and required runtime assets (CA certs for the pgx Postgres connection). The Dockerfile MUST build successfully for both `linux/amd64` and `linux/arm64`.

#### Scenario: amd64 image builds
- **WHEN** `docker buildx build --platform linux/amd64 procrastinator-backend` runs on a clean checkout
- **THEN** the build succeeds and produces a runnable image containing the backend binary

#### Scenario: arm64 image builds
- **WHEN** `docker buildx build --platform linux/arm64` runs using QEMU/binfmt emulation
- **THEN** the build succeeds without a cross-compile error

### Requirement: Backend image is published to configured registry
When registry secrets (`secrets.REGISTRY_URL`, `secrets.REGISTRY_USERNAME`, `secrets.REGISTRY_PASSWORD`) are configured, the backend workflow SHALL build with `docker/build-push-action` with `context: procrastinator-backend`, platforms `linux/amd64,linux/arm64`, buildx with QEMU setup, and GHA cache (`cache-from: type=gha`, `cache-to: type=gha,mode=max`), and push the image tagged with the registry host from secrets. The workflow file MUST NOT hardcode any registry hostname or credentials. If secrets are absent, the workflow SHALL still run test/vet and a local (load-only, no-push) build, and skip the push.

#### Scenario: Secrets configured — push happens
- **WHEN** CI runs with REGISTRY_* secrets configured
- **THEN** the multi-platform image is built with cache and pushed to the configured registry

#### Scenario: Secrets missing — CI still green
- **WHEN** REGISTRY_* secrets are not configured
- **THEN** vet/test run and a local Docker build (no push) succeeds; the workflow does not fail due to missing credentials
