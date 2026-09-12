# Tasks: terraform-unraid-deploy

Design: `design.md` (decisions D1–D10). All new files live under `terraform/`.
Tags: `[terraform]` `.tf` implementation · `[github]` GitHub provider · `[docs]` README/verification.

## 1. Versions and providers

- [x] 1.1 `[terraform]` Create `terraform/versions.tf`: `terraform` block with
      `required_version`; `required_providers` pinning `docker` (`kreuzwerker/docker`, ~> 3.5)
      and `github` (`integrations/github`, ~> 6.0). Verify: `terraform init` resolves
      both providers and writes `.terraform.lock.hcl`.
- [x] 1.2 `[terraform]` Create `terraform/providers.tf`: `provider "docker"` with
      `host = var.docker_host`; `provider "github"` with `owner = var.github_owner`,
      `token = var.github_token` (D4); `locals { registry_url = "registry.yelkawar.com" }`
      (D3). Verify: `terraform validate` passes.

- [x] 1.3 `[terraform]` Create `terraform/variables.tf` with all 16 variables per D8
      (docker_host, github_owner, github_token, github_repository, registry_url,
      registry_username, registry_password, backend_image_tag, ui_image_tag,
      backend_host_port, ui_host_port, backend_database_url, backend_llm_api_key,
      backend_llm_model, backend_basic_auth_users, backend_storage_host_path) —
      numeric ports with defaults 8080/8081; tags default `latest`; required vars without
      defaults; `sensitive = true` + `TF_VAR_*` notes on secrets. Add `terraform/.gitignore`
      excluding `*.tfvars` (keep `*.tfvars.example`). Verify: `terraform validate` passes;
      grep shows no secret literals; running `terraform plan` with nothing set fails
      naming the missing required variable (spec §1.1 fail-fast).

## 2. Network

- [x] 2.1 `[terraform]` Create `terraform/network.tf`: single `docker_network.procrastinator`
      (no driver/ipv6 overrides — default bridge behavior). Verify: `terraform plan` lists
      exactly this network plus providers — no speculative resources (spec §1.2 boyscout).

## 3. Containers

- [x] 3.1 `[terraform]` Create `terraform/containers.tf` with `docker_container.backend`
      (D6): name `procrastinator-backend`; image `registry.yelkawar.com/procrastinator-backend:${var.backend_image_tag}`;
      `restart = "unless-stopped"`; `networks_advanced` → `docker_network.procrastinator`;
      port 8080→`var.backend_host_port`; env `PROCRASTINATOR_DATABASE_URL` /
      `PROCRASTINATOR_LLM_API_KEY` / `PROCRASTINATOR_LLM_MODEL` /
      `PROCRASTINATOR_BASIC_AUTH_USERS`; writable volume mount from
      `var.backend_storage_host_path`. **File ownership note**: this task owns
      `containers.tf`; task 3.2 appends its resource to the same file — coordinator must
      sequence 3.2 after 3.1 (no parallel ownership). Verify: `terraform validate` passes;
      container names/env appear in `terraform plan`.
- [x] 3.2 `[terraform]` Append `docker_container.ui` (D6) to `terraform/containers.tf`:
      name `procrastinator-ui`; image `registry.yelkawar.com/procrastinator-ui:${var.ui_image_tag}`;
      `restart = "unless-stopped"`; same network; port 80→`var.ui_host_port`; no env,
      no volumes. Verify: `terraform plan` shows both containers and the network only.

## 4. GitHub secrets

- [x] 4.1 `[github]` Create `terraform/github-secrets.tf`: three `github_actions_secret`
      resources with `repository = var.github_repository`
      (D7): `registry_url` (value `var.registry_url`), `registry_username`
      (`var.registry_username`), `registry_password` (`var.registry_password`).
      Note: `sensitive` is not a valid attribute on `github_actions_secret` (value always
      encrypted by GitHub); the input vars are already `sensitive = true`.
      Verify: `terraform validate` passes; no secret value in any committed file.

## 5. Outputs

- [x] 5.1 `[terraform]` Create `terraform/outputs.tf` (D9, all non-sensitive):
      `backend_container_id`, `ui_container_id`, `backend_host_port`, `ui_host_port`,
      `backend_image_ref`, `ui_image_ref`, `github_secret_names`, `network_id`.
      Verify: `terraform output` post-plan shows variable names only — no secret values
      (spec §5.2, §2.2 no-sensitive-output).

## 6. Verification and documentation

- [x] 6.1 `[docs]` Create `terraform/README.md`: prerequisites (SSH access for
      `docker_host`; `/etc/docker/daemon.json` registry pull-auth on Unraid — not managed;
      pre-created writable backend storage host path/Dataset; **reachable Postgres
      prerequisite** per D1 — module does not create/migrate DBs, supply a connection URL
      via `TF_VAR_backend_database_url`); usage (`terraform init && plan && apply` with
      TF_VARs); exact §5.1 verification commands (backend `docker inspect ... State.Running`,
      curl 401 backend / 200 UI, image provenance `Config.Image` inspect, `gh run list`
      CI check); D2 CI trigger procedure: `gh workflow run "backend CI" --ref main`
      (and `ui CI`) → wait (`gh run watch`/poll) →
      `curl -fsS -u <user>:<pass> https://registry.yelkawar.com/v2/procrastinator-backend/tags/list`
      includes pushed sha; lifecycle (`terraform destroy` scope). Verify: README contains
      every §5.1 command verbatim with expected outputs; no committed credentials.

## 7. End-to-end validation

- [x] 7.1 `[terraform]` `terraform init && terraform validate` in `terraform/` with all
      variables supplied via env fixtures (wrong-or-dummy values acceptable for plan
      against a fake host not required — validate only). Verify: zero errors,
      formatted (`terraform fmt -check`).
- [x] 7.2 `[terraform]` `terraform plan` resource-census check: plan output contains
      exactly `docker_network`(1), `docker_container`(2), `github_actions_secret`(3) and
      nothing else (spec §1.2 boyscout scenario). Verify: the census list matches exactly.
- [x] 7.3 `[docs]` Repeat-apply no-op walkthrough documented in README already; sanity:
      run `terraform plan` twice with identical inputs (against a static-config run or
      dry parse) and confirm deterministic resource set. Verify: no drift in declared
      resources; `.gitignore` excludes real tfvars.
