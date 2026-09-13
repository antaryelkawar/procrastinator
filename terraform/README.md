# Terraform — Unraid deployment for procrastinator

Declaratively runs the backend + UI containers on an Unraid host (the `docker` provider talks to the daemon over SSH) and creates the three CI registry secrets in the target GitHub repo.

## Prerequisites

1. **Terraform >= 1.5** (per `versions.tf` `required_version`).
2. **SSH access to the Unraid host** — the docker provider reaches the daemon via `var.docker_host` (default `ssh://root@unraid-host`). `unix:///var/run/docker.sock` is valid when running Terraform on the Unraid itself.
3. **Postgres** — this module deploys `procrastinator-postgres` (Postgres 18) on the host network. Set `TF_VAR_postgres_password` (required, sensitive). The backend connects to it via `localhost:5432` because both containers are on the host network: `postgresql://<postgres_user>:<postgres_password>@localhost:5432/<postgres_db>` (default: `postgresql://pgadmin:<password>@localhost:5432/procrastinator`). Supply this URL via `TF_VAR_backend_database_url` (sensitive, required).
4. **Docker registry v2 pull-auth already configured on Unraid** — `registry.yelkawar.com` is private; the Unraid Docker daemon must already have pull auth (e.g. in `/etc/docker/daemon.json` or host-level `docker login`). This module does NOT manage daemon auth and declares no registry resource. If the daemon lacks auth, apply fails with a pull-auth error referencing `registry.yelkawar.com`.
5. **A GitHub repo with Actions enabled** — the module creates `REGISTRY_URL`/`REGISTRY_USERNAME`/`REGISTRY_PASSWORD` secrets there. You need a token with repo admin scope and the `owner/name` repo.

A **pre-created, writable backend storage host path** (Unraid share or `mkdir`) is also required by `backend_storage_host_path`, mounted at `/app/storage` (the backend is distroless/nonroot uid 65532 and needs a writable storage dir).

## Networking

- **Backend + Postgres use host networking.** No port mapping is configured; the container port IS the host port. The backend binds `:8321` directly and Postgres binds `:5432` directly on the host.
- **The UI is the exception.** It stays on Docker's default bridge network with port mapping `8322 -> 80`. The official `nginx:alpine` image hardcodes `listen 80;` (see `ui/nginx.conf`); making it listen on `8322` under host networking would require an image rebuild, which is out of scope for this Terraform change.
- **Backend → Postgres.** The backend reaches Postgres via `localhost:5432` (both on the host network), not via the `procrastinator-postgres` container name.

## Usage

1. `terraform init`
2. `terraform plan` — supply sensitive values via `TF_VAR_*` env vars or a git-ignored `terraform.tfvars` (do NOT commit real `*.tfvars`; a `.gitignore` in `terraform/` excludes `*.tfvars` except `*.tfvars.example`).
3. `terraform apply`

`terraform output` exposes the host ports and image refs to support verification without source diving.

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
| `backend_host_port` | number | `8321` | no | host port the backend binds directly (host net, no mapping) |
| `ui_host_port` | number | `8322` | no | UI stays on default bridge, mapped `8322 -> 80` |
| `backend_database_url` | string | — (required) | yes | Postgres URL; use `localhost:5432` under host net; via `TF_VAR_backend_database_url` |
| `backend_llm_api_key` | string | — (required) | yes | via `TF_VAR_backend_llm_api_key` |
| `backend_llm_model` | string | — (required) | no | non-secret model name |
| `backend_basic_auth_users` | string | — (required) | yes | `user:bcrypt-hash` CSV |
| `backend_storage_host_path` | string | — (required) | no | host dir mounted at `/app/storage` |
| `postgres_user` | string | `pgadmin` | no | Postgres superuser; via `TF_VAR_postgres_user` |
| `postgres_password` | string | — (required) | yes | via `TF_VAR_postgres_password` |
| `postgres_db` | string | `procrastinator` | no | Postgres database name |
| `postgres_host_port` | number | `5432` | no | host port Postgres binds directly (host net, no mapping) |

## Verification

Run from the Unraid host, or with `DOCKER_HOST=ssh://...` for docker-over-SSH; `gh` authenticated locally.

1. **Backend up** — expected `true` within 10s of apply:

```bash
docker inspect procrastinator-backend --format "{{.State.Running}}"
```

2. **Backend HTTP** — a `401`/`404` is expected (the API is auth-guarded and has no `/` root route; a non-connection-refused response proves the HTTP server is up on the host port):

```bash
curl -fsS http://192.168.1.103:8321/
```

3. **UI serving** — expected HTTP 200 with an HTML body:

```bash
curl -fsS http://192.168.1.103:8322/
```

4. **Postgres on the host** — from the Unraid host, Postgres is reachable at `localhost:5432` (both it and the backend are on the host network):

```bash
pg_isready -h localhost -p 5432
```

5. **Image provenance** — expected `registry.yelkawar.com/procrastinator-backend:<tag>` (and the `procrastinator-ui` equivalent):

```bash
docker inspect procrastinator-backend --format "{{.Config.Image}}"
```

6. **CI success** — the most recent completed run has `conclusion == "success"` (and the `ui CI` equivalent):

```bash
gh run list --workflow="backend CI" --limit 5 --json status,conclusion
```

7. **Registry tag check** — the JSON tag list includes the pushed build's tag (e.g. the commit sha):

```bash
curl -fsS -u <registry-user>:<registry-pass> https://registry.yelkawar.com/v2/procrastinator-backend/tags/list
```

### CI push procedure

1. `gh workflow run "backend CI" --ref main` (and `gh workflow run "ui CI" --ref main`).
2. Wait for completion (`gh run watch <id>` or re-poll `gh run list`).
3. Confirm the registry tag list contains the pushed sha (the Registry tag check command above).

This is a documented manual verification — the Terraform module never invokes `gh workflow run` itself.

## Destroy

`terraform destroy` removes the three containers, the `procrastinator-pgdata` volume, and the three GitHub secrets **the module created** (tracked in state). The UI's default bridge network is not a Terraform-managed resource, so it is untouched. It does NOT touch the registry v2, the daemon auth, or the backend storage host path/share (data is left intact). It does not assert absence of same-named secrets created by other tools.

## Repeat apply

`terraform apply` is idempotent: a second apply (or `terraform plan`) with unchanged inputs reports `No changes` / "no changes" (Terraform tracks seven resources — three `docker_container` (backend, ui, postgres), one `docker_volume`, and three `github_actions_secret`; the UI's default bridge network is not a Terraform resource, and no `count`/`for_each`).
