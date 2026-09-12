# Design: terraform-unraid-deploy

Change: `terraform-unraid-deploy` · Phase: DESIGN

## Context

The `procrastinator-backend` and `ui` images are built and pushed to `registry.yelkawar.com`
by GitHub Actions only when the repo secrets `REGISTRY_URL` / `REGISTRY_USERNAME` /
`REGISTRY_PASSWORD` exist. There is no declarative way to wire those secrets or run the two
containers on Unraid. This change adds a Terraform module under `terraform/` using the
HashiCorp `docker/docker` provider (SSH to the Unraid Docker daemon) and the
`integrations/github` provider for repository secrets.

Two non-blocking majors from review-spec-2 are resolved by explicit decisions here (D1, D2).

## Goals / Non-Goals

**Goals:**

- One idempotent `terraform apply` that: attaches its `docker_network`, creates both
  containers (`procrastinator-backend`, `procrastinator-ui`) from `registry.yelkawar.com`
  images, and creates the three GitHub Actions secrets.
- Verified deploy: `docker inspect` Running/Image, curl probes (backend 401, UI 200), and
  a deterministic CI-push/registry-existence check.
- Clean `terraform destroy` scoped to state-tracked resources.

**Non-Goals:**

- NOT managed by this module: the registry v2 on Unraid, Docker daemon registry auth
  (`/etc/docker/daemon.json`), databases (Postgres), reverse proxies, monitoring.
- No changes to application code, Dockerfiles, or GitHub workflow files.

## File layout

```
terraform/
├── versions.tf          # terraform + provider version constraints (docker, github)
├── providers.tf         # docker (host = var.docker_host) + github (owner + token)
├── variables.tf         # all input variables (see V-list below)
├── network.tf           # docker_network.procrastinator
├── containers.tf        # docker_container.backend + docker_container.ui + volume/data
├── outputs.tf           # non-sensitive outputs
├── README.md            # prerequisites + exact verification commands (§5.1)
└── github-secrets.tf    # 3 × github_actions_secret
```

(No `registry.tf` as a separate file: the registry host is a constant/local and daemon pull
auth is an explicit prerequisite, not a resource — see D3. No `terraform.tfvars` committed.)

## Decisions

### D1 — Database prerequisite (resolves review-spec-2 MAJOR 1)

Postgres is a **prerequisite, not a resource**. `main.go` runs `postgres.Open` + `Migrate`
before `ListenAndServe`, so the backend cannot reach Running/HTTP without a reachable DB,
and the review confirmed the boyscout guard correctly keeps the DB out of scope.

- New required sensitive variable `backend_database_url` (string, `sensitive = true`,
  no default) — supplied as a full Postgres connection URL (e.g.
  `postgres://user:pass@db-host:5432/procrastinator`) and injected verbatim as
  `PROCRASTINATOR_DATABASE_URL`.
- README documents the prerequisite in the same pattern as the registry prerequisite:
  "A reachable Postgres must exist before apply; this module does not create or migrate
  databases. Supply a connection URL to an existing instance."
- Alternatives considered: (a) `docker_container.postgres` sidecar in-scope — rejected:
  violates explicit "no databases" constraint in proposal/spec; (b) leave DB URL unexplained
  — rejected by review (unmodeled prerequisite). Prerequisite-with-named-variable keeps the
  happy path satisfiable (SPX-002) without scope creep.

### D2 — CI-push trigger pinned (resolves review-spec-2 MAJOR 2)

The §4.2 verification uses the deterministic manual dispatch; the workflow files already
define only `workflow_dispatch` + path-filtered push. Pinned exactly:

- Trigger mechanism: **`gh workflow run "backend CI" --ref main`** (and `gh workflow run
  "ui CI" --ref main`), with the fall-back trigger being a push to the default branch
  touching `procrastinator-backend/**` or `ui/**` (the workflows' path filters).
- The README documents, in order: dispatch → wait for completion
  (`gh run watch <id>` or re-poll `gh run list`) → verify registry tag list contains the
  pushed sha (`curl -fsS -u user:pass https://registry.yelkawar.com/v2/procrastinator-backend/tags/list`).
- The "phrase-triggered push" wording from the spec is treated as referring to the
  workflows' push/path filters; no phrase automation is implemented or relied upon.
- This is a documented manual verification, not an automated Terraform acceptance (the
  module never invokes `gh workflow run`).

### D3 — Registry auth: no registry resource

The containers' `registry.yelkawar.com` images are pulled by the Unraid daemon, which
already has pull auth configured (registry v2's daemon config / host docker login) — a
documented prerequisite (spec §3.1). The module declares **no** `docker_registry*` resource.
The registry URL is a local constant in `providers.tf`/`github-secrets.tf`
(`local.registry_url = "registry.yelkawar.com"`). If the daemon lacks auth, the apply fails
with a daemon pull-auth error referencing `registry.yelkawar.com` (spec §3.1 negative).
`/etc/docker/daemon.json` prerequisite is documented in README.

Note: this keep §1.2's "exactly" list at `docker_network` ×1, `docker_container` ×2,
`github_actions_secret` ×3, and zero speculative resources (boyscout guard).

### D4 — Provider configuration

`terraform/providers.tf`:

- `required_providers`: `docker` (~> 3.5) from `docker/docker`; `github` (~> 6.x) from
  `integrations/github`.
- `provider "docker"`: `host = var.docker_host` (default `ssh://root@unraid-host`; the
  SSH form is the default; `unix:///var/run/docker.sock` is valid when running TF on the
  Unraid itself). No `registry_auth` block (unused without registry resources).
- `provider "github"`: `owner = var.github_owner`, `token = var.github_token`
  (`sensitive`), per-repo resources use `repository = var.github_repository`.

### D5 — Resource mapping (spec requirement → resource)

| Spec requirement | Resource |
|---|---|
| §1.1 providers | `terraform.required_providers` (docker, github) + providers.tf blocks |
| §1.2 network | `docker_network.procrastinator` |
| §2.1 backend container | `docker_container.backend` |
| §2.3 UI container | `docker_container.ui` |
| §4.1 REGISTRY_URL | `github_actions_secret.registry_url` |
| §4.1 REGISTRY_USERNAME | `github_actions_secret.registry_username` |
| §4.1 REGISTRY_PASSWORD | `github_actions_secret.registry_password` |

Network in `network.tf`; containers in `containers.tf`; secrets in `github-secrets.tf`.

### D6 — Container configuration

**`docker_container.backend`** (name `procrastinator-backend`):

- `image = "${local.registry_url}/procrastinator-backend:${var.backend_image_tag}"` (default tag `latest`)
- `restart = "unless-stopped"`
- `networks_advanced` → `docker_network.procrastinator`
- Port: `ports { internal = 8080, external = var.backend_host_port }` (numeric var, default `8080`)
- Env (from vars, `sensitive = true`):
  - `PROCRASTINATOR_DATABASE_URL = var.backend_database_url`
  - `PROCRASTINATOR_LLM_API_KEY = var.backend_llm_api_key`
  - `PROCRASTINATOR_LLM_MODEL = var.backend_llm_model`
  - `PROCRASTINATOR_BASIC_AUTH_USERS = var.backend_basic_auth_users`
- Volume: host storage dir (var `backend_storage_host_path`, required — distroless nonroot
  uid 65532 needs a writable `MkdirAll` target) mounted read-write at the backend's
  storage dir. Host-path mode so a pre-created Unraid share is used; the share creation is
  a README prerequisite (Unraid share or `mkdir`),
- Health/on-start: none (distroless, no shell → no healthcheck executable). Reliability is
  enforced via `restart` policy and the documented Running/HTTP checks.

**`docker_container.ui`** (name `procrastinator-ui`):

- `image = "${local.registry_url}/procrastinator-ui:${var.ui_image_tag}"` (default `latest`)
- `restart = "unless-stopped"`; `networks_advanced` → same network
- Port: `ports { internal = 80, external = var.ui_host_port }` (default `8081`)
- No env; no volumes.

### D7 — GitHub secrets

`github-secrets.tf` — three `github_actions_secret` resources,
`repository = var.github_repository` (e.g. `owner/procrastinator`).
Note: `sensitive` is not a valid attribute on `github_actions_secret` (the value is
always encrypted by GitHub); the input variables (`registry_username`, `registry_password`)
are already `sensitive = true`, which masks them in plan/apply output.

- `registry_url` → value `var.registry_url`
- `registry_username` → `var.registry_username`
- `registry_password` → `var.registry_password`

Token/owner come from `var.github_token` / `var.github_owner` (never hardcoded).

### D8 — Variables (variables.tf)

| Variable | Type | Default | Sensitive | Notes |
|---|---|---|---|---|
| `docker_host` | string | `"ssh://root@unraid-host"` | no | §1.1 |
| `github_owner` | string | none (required) | no | §1.1 |
| `github_token` | string | none (required) | yes | §1.1, from `TF_VAR_github_token` |
| `github_repository` | string | none (required) | no | e.g. `owner/procrastinator` |
| `registry_url` | string | `"registry.yelkawar.com"` | no | overridable but fixed by default |
| `registry_username` | string | none (required) | yes | |
| `registry_password` | string | none (required) | yes | |
| `backend_image_tag` | string | `"latest"` | no | |
| `ui_image_tag` | string | `"latest"` | no | |
| `backend_host_port` | number | `8080` | no | |
| `ui_host_port` | number | `8081` | no | |
| `backend_database_url` | string | none (required) | yes | D1 prerequisite |
| `backend_llm_api_key` | string | none (required) | yes | |
| `backend_llm_model` | string | none (required) | no | non-secret model name |
| `backend_basic_auth_users` | string | none (required) | yes | `user:bcrypt-hash` CSV |
| `backend_storage_host_path` | string | none (required) | no | host dir for backend storage |

Required-without-default variables satisfy spec §1.1 "fail fast with validation error naming
the missing required variable" (Terraform's native required-var error names the variable).
All sensitive values are passed via `TF_VAR_*` env or a git-ignored `terraform.tfvars`
(a `.gitignore` inside `terraform/` excludes `*.tfvars` except `*.example`).

### D9 — Outputs (outputs.tf, all non-sensitive)

- `backend_container_id`, `ui_container_id` (container IDs)
- `backend_host_port`, `ui_host_port` (published ports as numbers)
- `backend_image_ref`, `ui_image_ref` (full `registry.yelkawar.com/...:<tag>`)
- `github_secret_names` (the 3 secret names — names only, never values)
- `network_id`

### D10 — Idempotency / lifecycle

- Standard resource graph; no `count`/`for_each` metagimmicks; second apply with unchanged
  inputs yields `No changes` (Terraform tracks all five resources).
- `docker_network` uses no explicit `ipv6`/driver overrides → default `bridge` behavior on
  the Unraid daemon; destroy removes network + containers + secrets tracked in state. The
  backend storage host-path directory is NOT created or destroyed by Terraform (prerequisite
  share) — destroy leaves the host share and any data intact (safer; documented).
- `terraform destroy` does not touch daemon auth, registry v2, or untracked same-named
  GitHub secrets created by other tools (spec §6.1).

## Risks / Trade-offs

- [SSH provider connection flakiness (long-running docker operations over ssh://) ] →
  README advises running TF on the Unraid host (`unix:///var/run/docker.sock`) for large
  image pulls if SSH proves unstable; `docker_host` is a variable so no code change needed.
- [`:latest` deploys are mutable — images drift between applies ] → `image` pinning via
  `backend_image_tag`/`ui_image_tag` vars; users can pin a CI sha tag for reproducibility.
  Accepted trade-off (spec fixes `latest` as default).
- [Backend requires Postgres at boot; a wrong `backend_database_url` makes the container
  crash-loop] → Known and intended (fail-fast per spec §2.2 negative scenario); README
  prerequisite section makes it explicit; `docker logs` shows the `config:`/postgres error.
- [GitHub `github_actions_secret` secrets are write-only — drift can't compare values] →
  acceptable; spec §4.1 requires "no drift" only on names/existence, and
  re-apply is a no-op for secret values Terraform already tracks.

## Open Questions

(none — all majors resolved; DB and CI trigger decisions above are final for the happy path)
