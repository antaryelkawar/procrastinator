# Delta Spec: Terraform Unraid Deployment

Change: `terraform-unraid-deploy` · Phase: SPEC (cycle 2)

Provider basis (corrected): containers, network, and registry resources are managed by
the `docker/docker` provider (HashiCorp), which reaches the Unraid Docker daemon over
SSH (`host = "ssh://user@unraid-host"`). GitHub secrets use `github_actions_secret`.
This spec states observable behavior; the exact provider resource mapping for each
requirement is fixed in the design phase — but every deployable primitive below exists
in the `docker/docker` provider.

## 1. Terraform provider and module structure

### 1.1. Requirement: The module MUST configure the `docker/docker` and `github` providers for the Unraid host and repository.

- The Docker provider connects to the Unraid Docker daemon via SSH
  (`host = "ssh://<user>@<unraid-host>"`, configurable via variable
  `unraid_ssh_host`; the `unix:///var/run/docker.sock` form is valid when running
  Terraform on the Unraid itself).
- The GitHub provider is configured with a repository name and token, both from
  Terraform variables with `TF_VAR_*` env fallback — never hardcoded.
- The registry host is fixed to `registry.yelkawar.com`.

##### Scenario: Terraform can plan against the Unraid Docker daemon

- **GIVEN** SSH access from the Terraform runner to `unraid-host` for the docker user
- **WHEN** the user runs `terraform init && terraform plan` with
  `unraid_ssh_host` provided via tfvars or `TF_VAR_*`
- **THEN** the plan succeeds and lists the module resources (see §1.2)
- **AND** no committed file contains any secret value.

##### Scenario: Missing provider credentials fail fast

- **WHEN** `terraform plan` runs without the SSH host, GitHub token, or required
  registry credentials set
- **THEN** Terraform fails with a validation error naming the missing required
  variable (variables are declared required; sensitive ones marked `sensitive = true`).

### 1.2. Requirement: The module MUST declare exactly the in-scope resources.

Exactly: one `docker_network`, two `docker_container` (backend, UI), and three
`github_actions_secret` resources. Any registry-credential resource the design
settles on (e.g. `docker_registry_image`-scoped auth) is limited to configuring pulls
from `registry.yelkawar.com`. No databases, reverse proxies, monitoring, or other
services. (Boyscout guard.)

##### Scenario: No speculative resources

- **WHEN** `terraform plan` is inspected after `init`
- **THEN** the resources to be created are exactly: the docker network, the backend
  container, the UI container, registry-credential configuration for
  `registry.yelkawar.com` (if used), and three `github_actions_secret` resources
- **AND** no other services appear in the plan.

## 2. Container configuration

### 2.1. Requirement: The backend container MUST run the backend image from the JobOptimizer registry.

Image `registry.yelkawar.com/procrastinator-backend`. The deployed tag is
`backend_image_tag`, **default `latest`** (the `latest` image in the registry is the
CI-pushed image). Restart policy `unless-stopped`; attached to the module's network;
writable storage volume mounted.

##### Scenario: Backend container is running on Unraid

- **GIVEN** a reachable Unraid host with the registry v2 and daemon pull auth present
- **WHEN** `terraform apply` completes successfully
- **THEN** `docker inspect procrastinator-backend` shows `State.Running == true` and
  `Config.Image == "registry.yelkawar.com/procrastinator-backend:<tag>"`
- **AND** a repeat `terraform apply` with no input changes performs no changes.

### 2.2. Requirement: The backend container MUST receive its required environment and port.

Environment: `PROCRASTINATOR_DATABASE_URL`, `PROCRASTINATOR_LLM_API_KEY`,
`PROCRASTINATOR_LLM_MODEL`, `PROCRASTINATOR_BASIC_AUTH_USERS` (per
`config/config.go` required list; `PROCRASTINATOR_HTTP_ADDR` default `:8080`).
Values injected via sensitive Terraform variables/env — never hardcoded.
The backend publishes host port `backend_host_port` (numeric default **8080**) to
container port **8080**.

##### Scenario: Backend starts with required env present

- **GIVEN** valid DB / LLM / Basic-Auth values supplied via tfvars or TF_VARs
- **WHEN** `terraform apply` completes
- **THEN** within 10 seconds of apply, `docker inspect procrastinator-backend` shows
  `State.Running == true`
- **AND** `curl -s -o /dev/null -w "%{http_code}" http://<unraid-host>:8080/api/users/x/documents`
  prints an HTTP status (expected `401` — the API is Basic-Auth-guarded and has no
  unauthenticated `/health` route), proving the server is listening.

##### Scenario: Missing env fails fast and observably

- **GIVEN** any required variable is missing or invalid
- **WHEN** the backend container starts
- **THEN** the container exits with non-zero exit code (it must stay exited, not run)
- **AND** `docker logs procrastinator-backend` contains
  `config: missing required environment variable <NAME>` (or the matching
  `config:` validation error for invalid values).

##### Scenario: No secret values in committed files

- **WHEN** the Terraform sources are reviewed
- **THEN** no secret value (DB URL, LLM key, Basic-Auth users, registry password)
  appears in any committed `.tf` / `.tfvars` file
- **AND** `terraform plan` masks such values as `(sensitive value)` in console output.

### 2.3. Requirement: The UI container MUST run the SPA image on its published port.

Image `registry.yelkawar.com/procrastinator-ui`, tag via `ui_image_tag`
(default `latest`); nginx:alpine serves on container port **80** (per
`ui/Dockerfile`); published on host port `ui_host_port` (numeric default **8081**);
same network; restart policy `unless-stopped`; no environment variables required.

##### Scenario: UI container serves the SPA

- **GIVEN** a successful `terraform apply`
- **WHEN** a client requests `http://<unraid-host>:8081/`
- **THEN** the response is HTTP 200, `text/html`, the procrastinator SPA entry
  document (`index.html` served with no-cache semantics per `ui/nginx.conf`).

## 3. Registry authentication

### 3.1. Requirement: Containers MUST pull private images from `registry.yelkawar.com` without manual login.

Because `registry.yelkawar.com` is private, images are pulled with credentials so no
interactive `docker login` is required on the Unraid host. Pull credentials come from
sensitive Terraform variables. **Prerequisite**: the Unraid Docker daemon already has
registry auth configured (via the deployed registry v2's daemon configuration, e.g.
in `/etc/docker/daemon.json` or host-level Docker login) — this module does not
manage the daemon config.

##### Scenario: Private image pull succeeds without manual login

- **GIVEN** the registry v2 with pull credentials on the Unraid host
- **WHEN** `terraform apply` runs
- **THEN** `docker inspect procrastinator-backend` and
  `docker inspect procrastinator-ui` each show `State.Running == true` and
  `Config.Image` equal to the full `registry.yelkawar.com/...` reference
- **AND** `docker logs` for both containers contains no `unauthorized` /
  `authentication required` / `no such host` error.

##### Scenario: Bad credentials surface clearly

- **WHEN** the pull credentials are wrong and `terraform apply` runs
- **THEN** the apply (or container start) fails with an authentication error
  referencing `registry.yelkawar.com`, rather than silently leaving a stale image.

## 4. GitHub Actions secrets

### 4.1. Requirement: The three CI secrets MUST be managed via `github_actions_secret`.

The module creates `REGISTRY_URL`, `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` on the
target GitHub repository. `REGISTRY_URL` is fixed to `registry.yelkawar.com`;
username/password come from sensitive variables. The GitHub token and repo name come
from variables/`TF_VAR_*`.

##### Scenario: Secrets exist for the repository

- **GIVEN** a GitHub token with repo admin rights and the repository name supplied
- **WHEN** `terraform apply` completes
- **THEN** the repository has secrets `REGISTRY_URL`, `REGISTRY_USERNAME`,
  `REGISTRY_PASSWORD` with the supplied values
- **AND** re-running `terraform plan` shows no drift.

##### Scenario: No committed secret values

- **WHEN** the repo is reviewed
- **THEN** no credential value appears in any committed file; only variable
  declarations marked `sensitive = true` exist.

### 4.2. Requirement: Setting the secrets MUST unlock the CI push path (verifiable via the registry).

After the secrets exist, both workflows' push steps evaluate `HAS_REGISTRY == 'true'`
and push images to `registry.yelkawar.com`.

##### Scenario: Workflow push produces a registry image

- **GIVEN** the three secrets exist on the repository
- **WHEN** a `backend CI` (or `ui CI`) workflow run completes on a phrase-triggered
  push (manual workflow dispatch is the documented trigger; ZAPPR/phrase automation is
  out of scope)
- **THEN** `curl -fsS -u <registry-user>:<registry-pass> https://registry.yelkawar.com/v2/procrastinator-backend/tags/list`
  returns a JSON tag list that includes the pushed build's tag (e.g. the commit sha),
  or equivalently `docker pull registry.yelkawar.com/procrastinator-backend:<sha>` succeeds.

## 5. Verification

### 5.1. Requirement: The deployment MUST be verifiable with exact machine-executable commands.

The module README documents these checks (expected results in brackets):

- **Backend up**:
  `docker inspect procrastinator-backend --format "{{.State.Running}}"` → `true`
  within 10s of apply.
- **Backend HTTP**:
  `curl -s -o /dev/null -w "%{http_code}" http://<unraid-host>:8080/api/users/x/documents`
  → prints `401` (Basic-Auth-guarded API; no unauthenticated `/health` route exists).
- **UI up/serving**:
  `curl -fsS http://<unraid-host>:8081/` → HTTP 200 with HTML body.
- **Image provenance**:
  `docker inspect procrastinator-backend --format "{{.Config.Image}}"` →
  `registry.yelkawar.com/procrastinator-backend:<tag>` (same for `procrastinator-ui`
  with `ui:<tag>`, tags per §2.1/§2.3 defaults).
- **CI success**:
  `gh run list --workflow="backend CI" --limit 5 --json status,conclusion` (and the
  `ui CI` equivalent) → the most recent completed run has `conclusion == "success"`.

##### Scenario: End-to-end verification walk-through

- **GIVEN** `terraform apply` completed and the secrets were pushed to GitHub
- **WHEN** the user runs the exact commands above (from the Unraid host or with
  `DOCKER_HOST=ssh://...` for docker over SSH; `gh` authenticated locally)
- **THEN** all checks return the stated expected results.

### 5.2. Requirement: Verification steps MUST be documented in the module.

The Terraform module includes a README plus `terraform output` values (published
ports, container/image refs) so verification requires no source diving.

##### Scenario: Outputs support verification

- **WHEN** `terraform output` is run after apply
- **THEN** it reports the backend and UI published host ports and the image
  references deployed
- **AND** the README documents the exact verification commands listed in §5.1,
  including the manual workflow dispatch used for the CI check.

## 6. Idempotency and lifecycle

### 6.1. Requirement: `terraform apply` MUST be repeatable and `terraform destroy` scoped to module-created resources.

Applying twice with unchanged inputs yields no changes. `terraform destroy` removes
the containers, the network, the registry-credential configuration, and the three
secrets **the module created (tracked in Terraform state)**; it does not assert the
absence of same-named secrets created by other tools, and it does NOT touch the
registry v2 deployment or the daemon auth.

##### Scenario: Repeat apply is a no-op

- **GIVEN** a completed first `terraform apply`
- **WHEN** `terraform plan` runs again with unchanged inputs
- **THEN** the plan reports "no changes".

##### Scenario: Destroy tears down module-managed resources

- **GIVEN** a completed apply
- **WHEN** `terraform destroy` runs and completes
- **THEN** the two containers, the network, the registry-credential configuration,
  and the three GitHub secrets tracked in state are removed
- **AND** the registry v2 deployment and daemon auth remain untouched.

##### Scenario: Restart policy keeps containers alive

- **GIVEN** containers running under the module's restart policy (`unless-stopped`)
- **WHEN** the Unraid host's Docker daemon restarts
- **THEN** both containers restart automatically.
