# Terraform — Unraid deployment for procrastinator

Declaratively runs the backend + UI containers on an Unraid host (the `docker` provider talks to the daemon over SSH) and creates the three CI registry secrets in the target GitHub repo.

## Prerequisites

1. **Terraform >= 1.5** (per `versions.tf` `required_version`).
2. **SSH access to the Unraid host** — the docker provider reaches the daemon via `var.docker_host` (default `ssh://root@unraid-host`). `unix:///var/run/docker.sock` is valid when running Terraform on the Unraid itself.
3. **Postgres** — this module deploys `procrastinator-postgres` (Postgres 18) on the shared network. Set `TF_VAR_postgres_password` (required, sensitive). The backend connects via the container name: `postgresql://<postgres_user>:<postgres_password>@procrastinator-postgres:5432/<postgres_db>` (default: `postgresql://pgadmin:<password>@procrastinator-postgres:5432/procrastinator`). Supply this URL via `TF_VAR_backend_database_url` (sensitive, required).
4. **Docker registry v2 pull-auth already configured on Unraid** — `registry.yelkawar.com` is private; the Unraid Docker daemon must already have pull auth (e.g. in `/etc/docker/daemon.json` or host-level `docker login`). This module does NOT manage daemon auth and declares no registry resource. If the daemon lacks auth, apply fails with a pull-auth error referencing `registry.yelkawar.com`.
5. **A GitHub repo with Actions enabled** — the module creates `REGISTRY_URL`/`REGISTRY_USERNAME`/`REGISTRY_PASSWORD` secrets there. You need a token with repo admin scope and the `owner/name` repo.

A **pre-created, writable backend storage host path** (Unraid share or `mkdir`) is also required by `backend_storage_host_path`, mounted at `/app/storage` (the backend is distroless/nonroot uid 65532 and needs a writable storage dir).

## Usage

1. `terraform init`
2. `terraform plan` — supply sensitive values via `TF_VAR_*` env vars or a git-ignored `terraform.tfvars` (do NOT commit real `*.tfvars`; a `.gitignore` in `terraform/` excludes `*.tfvars` except `*.tfvars.example`).
3. `terraform apply`

`terraform output` exposes the published ports and image refs to support verification without source diving.

## Variables

| Variable | Type | Default | Sensitive | Notes |
|---|---|---|---|---|
| `docker_host` | string | `ssh://root@unraid-host` | no | daemon address (SSH or `unix://...`) |
| `github_owner` | string | — (required) | no | owner of target repo |
| `github_token` | string | — (required) | yes | repo admin scope; via `TF_VAR_github_token` |
| `github_repository` | string | — (required) | no | `owner/name` form |
| `registry_url` | string | `registry.yelkawar.com` | no | overridable; fixed by default |
| `registry_username` | string | — (required) | yes | via `TF_VAR_registry_username` |
| `registry_password` | string | — (required) | yes | via `TF_VAR_registry_password` |
| `backend_image_tag` | string | `latest` | no | image tag |
| `ui_image_tag` | string | `latest` | no | image tag |
| `backend_host_port` | number | `8080` | no | host port (container 8080) |
| `ui_host_port` | number | `8081` | no | host port (container 80) |
| `backend_database_url` | string | — (required) | yes | Postgres URL; via `TF_VAR_backend_database_url` |
| `backend_llm_api_key` | string | — (required) | yes | via `TF_VAR_backend_llm_api_key` |
| `backend_llm_model` | string | — (required) | no | non-secret model name |
| `backend_basic_auth_users` | string | — (required) | yes | `user:bcrypt-hash` CSV |
| `backend_storage_host_path` | string | — (required) | no | host dir mounted at `/app/storage` |
| `postgres_user` | string | `pgadmin` | no | Postgres superuser; via `TF_VAR_postgres_user` |
| `postgres_password` | string | — (required) | yes | via `TF_VAR_postgres_password` |
| `postgres_db` | string | `procrastinator` | no | Postgres database name |
| `postgres_host_port` | number | `5432` | no | host port (container 5432) |

## Verification

Run from the Unraid host, or with `DOCKER_HOST=ssh://...` for docker-over-SSH; `gh` authenticated locally.

1. **Backend up** — expected `true` within 10s of apply:

```bash
docker inspect procrastinator-backend --format "{{.State.Running}}"
```

2. **Backend HTTP** — expected `401` (Basic-Auth-guarded API; no unauthenticated `/health` route exists):

```bash
curl -s -o /dev/null -w "%{http_code}" http://<unraid-host>:8080/api/users/x/documents
```

3. **UI serving** — expected HTTP 200 with an HTML body:

```bash
curl -fsS http://<unraid-host>:8081/
```

4. **Image provenance** — expected `registry.yelkawar.com/procrastinator-backend:<tag>` (and the `procrastinator-ui` equivalent):

```bash
docker inspect procrastinator-backend --format "{{.Config.Image}}"
```

5. **CI success** — the most recent completed run has `conclusion == "success"` (and the `ui CI` equivalent):

```bash
gh run list --workflow="backend CI" --limit 5 --json status,conclusion
```

6. **Registry tag check** — the JSON tag list includes the pushed build's tag (e.g. the commit sha):

```bash
curl -fsS -u <registry-user>:<registry-pass> https://registry.yelkawar.com/v2/procrastinator-backend/tags/list
```

### CI push procedure

1. `gh workflow run "backend CI" --ref main` (and `gh workflow run "ui CI" --ref main`).
2. Wait for completion (`gh run watch <id>` or re-poll `gh run list`).
3. Confirm the registry tag list contains the pushed sha (the Registry tag check command above).

This is a documented manual verification — the Terraform module never invokes `gh workflow run` itself.

## Destroy

`terraform destroy` removes the three containers, the `procrastinator-pgdata` volume, the `procrastinator-net` network, and the three GitHub secrets **the module created** (tracked in state). It does NOT touch the registry v2, the daemon auth, or the backend storage host path/share (data is left intact). It does not assert absence of same-named secrets created by other tools.

## Repeat apply

`terraform apply` is idempotent: a second apply (or `terraform plan`) with unchanged inputs reports `No changes` / "no changes" (Terraform tracks all seven resources — one network, three containers, one volume, three secrets; no `count`/`for_each`).
