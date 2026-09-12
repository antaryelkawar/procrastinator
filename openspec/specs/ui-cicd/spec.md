# Spec: ui-cicd

Automated continuous integration and containerization for the Vite-based React UI. Ensures code quality through automated linting and testing, and produces multi-platform Nginx-based Docker images.

## Purpose

Provide a reliable, automated pipeline that validates UI changes via `npm run lint` and `npm run test`, and packages the static assets into a production-ready Nginx Docker image. The pipeline supports multi-platform builds and automated publishing to a configured registry, ensuring that only verified code reaches the container registry.

## Requirements

### Requirement: UI CI workflow triggers on UI changes
A GitHub Actions workflow SHALL exist at `.github/workflows/ui.yaml` that triggers on `push` events filtered by path `ui/**` and on `workflow_dispatch`. CI steps SHALL be, in the working directory `ui` with Node >= 22.12: `npm ci`, `npm run lint`, `npm run test`, `npm run build`, followed by a Docker image build. All commands SHALL map to the package.json scripts (lint=`eslint .`, test=`vitest run --passWithNoTests`, build=`tsc -b && vite build`) — no additional linting frameworks introduced.

#### Scenario: UI-only change runs UI CI
- **WHEN** a commit containing changes only under `ui/` is pushed
- **THEN** the UI workflow runs install, lint, test, build, and Docker build

#### Scenario: Backend-only change does not run UI CI
- **WHEN** a commit with changes only under `procrastinator-backend/` is pushed
- **THEN** the UI workflow does not run

#### Scenario: Manual dispatch
- **WHEN** a user triggers the UI workflow via `workflow_dispatch`
- **THEN** the full pipeline (install, lint, test, build, Docker build) executes

### Requirement: UI CI gates on checks passing
The UI workflow SHALL fail and skip the Docker build if any of lint, test (`npm run test`, which may pass trivially with `--passWithNoTests`), or `npm run build` exits nonzero.

#### Scenario: Type or lint error blocks image build
- **WHEN** `npm run build` or `npm run lint` exits nonzero
- **THEN** the workflow run fails and no Docker image build/push job runs

### Requirement: UI Dockerfile is multi-stage nginx static serving
A Dockerfile SHALL exist at `ui/Dockerfile` using a multi-stage build: a Node builder stage (Node >= 22.12, `npm ci`, `tsc -b && vite build`) producing `dist/`, followed by a static-serving stage (e.g. nginx:alpine) that copies `dist/` in as static content. The Dockerfile MUST build successfully for both `linux/amd64` and `linux/arm64` and must not run the build inside the final image.

#### Scenario: amd64 image builds
- **WHEN** `docker buildx build --platform linux/amd64 ui` runs on a clean checkout
- **THEN** the build succeeds and produces an nginx image serving the built dist assets

#### Scenario: arm64 image builds
- **WHEN** `docker buildx build --platform linux/arm64` runs with QEMU/binfmt emulation
- **THEN** the build succeeds

### Requirement: UI image is published to configured registry
When registry secrets (`secrets.REGISTRY_URL`, `secrets.REGISTRY_USERNAME`, `secrets.REGISTRY_PASSWORD`) are configured, the UI workflow SHALL build with `docker/build-push-action` with `context: ui`, platforms `linux/amd64,linux/arm64`, buildx with QEMU setup, GHA cache (`cache-from: type=gha`, `cache-to: type=gha,mode=max`), and push to the registry host read from secrets — never hardcoded. If secrets are absent, the workflow SHALL run install/lint/test/build and a local no-push build, and skip the push.

#### Scenario: Secrets configured — push happens
- **WHEN** CI runs with REGISTRY_* secrets configured
- **THEN** the multi-platform UI image is built with cache and pushed to the configured registry

#### Scenario: Secrets missing — CI still green
- **WHEN** REGISTRY_* secrets are not configured
- **THEN** install/lint/test/build run, a local Docker build (no push) succeeds, and the workflow does not fail from missing credentials

### Requirement: Registry configuration remains an open item
The registry host and credentials are repository configuration inputs, not source code: they SHALL be referenced exclusively via `${{ secrets.REGISTRY_URL }}` / `${{ secrets.REGISTRY_USERNAME }}` / `${{ secrets.REGISTRY_PASSWORD }}`, and the choice of registry must be documented as an open configuration item before first deployment (JobOptimizer's `registry.yelkawar.com` is a reference, not a decision).

#### Scenario: No hardcoded registry values
- **WHEN** the workflow files are reviewed
- **THEN** no registry hostname, username, or password literal appears in any committed file; only secret references and image-tag templates are present
