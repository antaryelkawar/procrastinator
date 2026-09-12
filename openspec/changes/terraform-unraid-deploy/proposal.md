# Proposal: Terraform Unraid deployment (terraform-unraid-deploy)

## Why

The procrastinator backend (`procrastinator-backend`) and UI (`ui`) are already built and
pushed as multi-arch Docker images to `registry.yelkawar.com` by the existing GitHub
Actions workflows (`.github/workflows/backend.yaml`, `.github/workflows/ui.yaml`), but
**only when the repo secrets `REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`
are set** — otherwise the workflows fall back to build-only (no push). There is no
declarative, repeatable way to:

1. Wire the GitHub Actions secrets the CI needs, and
2. Run the two images as containers on the Unraid host.

A Docker registry v2 is already deployed on Unraid (explicitly out of scope — not
managed by this module).

## What Changes

Introduce a Terraform configuration under `terraform/` that manages the Unraid Docker
 workload via the **`docker/docker` provider (HashiCorp)** — the provider whose
`docker_container` / `docker_network` / `docker_registry_image` resources actually
create containers, networks, and registry configuration. (Corrected constraint: the
`unraid/unraid` community provider is read-only for containers and is NOT used; see
review-spec-1 CRITICAL findings.)

- **Provider setup**: `docker/docker` with `host = "ssh://<user>@<unraid-host>"`
  (default assumption; `unix:///var/run/docker.sock` variant applies when running
  Terraform on the Unraid itself), plus the `integrations/github` provider for
  repository secrets. All credentials via Terraform variables / `TF_VAR_*` env —
  never hardcoded.
- **Containers**: two `docker_container` resources —
  `procrastinator-backend` (image `registry.yelkawar.com/procrastinator-backend`,
  tag via `backend_image_tag` variable, default `latest`; requires env
  `PROCRASTINATOR_DATABASE_URL`, `PROCRASTINATOR_LLM_API_KEY`, `PROCRASTINATOR_LLM_MODEL`,
  `PROCRASTINATOR_BASIC_AUTH_USERS` per `config/config.go`; listens on `:8080`,
  published on host port `backend_host_port` default **8080**; needs a writable
  storage dir volume; restart policy `unless-stopped`) and `procrastinator-ui`
  (image `registry.yelkawar.com/procrastinator-ui`, nginx:alpine serving the SPA on
  container port 80 per `ui/Dockerfile`, published on host port `ui_host_port`
  default **8081**; no required env; restart policy `unless-stopped`). Both attach
  to a module-managed `docker_network`.
- **Private image pulls**: containers pull from `registry.yelkawar.com` without a
  manual `docker login` on the Unraid host. Pull credentials for the Unraid Docker
  daemon are a **prerequisite** (the registry v2 already deployed on Unraid also
  configures daemon auth in `/etc/docker/daemon.json` / daemon-level registry auth on
  the host) — not something this module manages.
- **Secrets wiring**: creates three GitHub Actions repository secrets (`REGISTRY_URL`
  fixed to `registry.yelkawar.com`; `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` from
  sensitive variables) via `github_actions_secret`.

## Constraints / decisions (fixed)

1. **Provider**: `docker/docker` (HashiCorp). Unraid is reached over SSH:
   `host = "ssh://user@unraid-host"`. Container/host ports: backend 8080,
   UI 8081 (numeric defaults, overridable via `backend_host_port` /
   `ui_host_port`).
2. **Image registry**: `registry.yelkawar.com` (keep it). Default deploy tag: `latest`
   (overridable via `backend_image_tag` / `ui_image_tag`). The `latest` tag in the
   registry is the CI-pushed image.
3. **Secrets**: exactly `REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`,
   values read from TF_VARs — never hardcoded or committed.
4. **Out of scope / do not manage**: the registry v2 deployment on Unraid, daemon
   registry auth configuration, databases, reverse proxies, monitoring, or any other
   service.

## Impact

- **New code**: `terraform/` module (providers, variables, resources, outputs, README).
- **No changes** to application code, Dockerfiles, or GitHub workflows.
- **Users running this**: `terraform init && terraform plan && terraform apply`
  from the `terraform/` directory with variables/TF_VARs supplied.
