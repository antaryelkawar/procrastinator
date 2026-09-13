# Backend + UI + Postgres containers for procrastinator

resource "docker_container" "backend" {
  name    = "procrastinator-backend"
  image   = "${var.registry_url}/procrastinator-backend:${var.backend_image_tag}"
  restart = "unless-stopped"

  # Under host networking there is no port mapping — the container binds the
  # host port directly. The Go backend reads PROCRASTINATOR_HTTP_ADDR (default
  # ":8080") and must bind :8321 (= var.backend_host_port) under host net.
  network_mode = "host"

  env = [
    "PROCRASTINATOR_HTTP_ADDR=:${var.backend_host_port}",
    "PROCRASTINATOR_DATABASE_URL=${var.backend_database_url}",
    "PROCRASTINATOR_LLM_API_KEY=${var.backend_llm_api_key}",
    "PROCRASTINATOR_LLM_MODEL=${var.backend_llm_model}",
    "PROCRASTINATOR_BASIC_AUTH_USERS=${var.backend_basic_auth_users}",
  ]

  volumes {
    host_path      = var.backend_storage_host_path
    container_path = "/app/storage"
  }
}

resource "docker_container" "ui" {
  name    = "procrastinator-ui"
  image   = "${var.registry_url}/procrastinator-ui:${var.ui_image_tag}"
  restart = "unless-stopped"

  # EXCEPTION (no host networking): the UI stays on Docker's default bridge
  # network with port mapping instead of host mode. The official nginx:alpine
  # image hardcodes `listen 80;` (see ui/nginx.conf); making it listen on a
  # configurable port would require an image rebuild (Dockerfile envsubst
  # template + CI push), which is out of scope for this Terraform change. The
  # SPA is static and does not call the backend directly (/api/ is routed by a
  # reverse proxy), so it needs no inter-container network; port mapping
  # 8322 -> 80 keeps it reachable at the same host port.
  ports {
    internal = 80
    external = var.ui_host_port
  }
}

resource "docker_container" "postgres" {
  name    = "procrastinator-postgres"
  image   = "postgres:18"
  restart = "unless-stopped"

  # Host networking: postgres listens directly on host port 5432; the backend
  # (also on host net) reaches it via localhost:5432 (no container name needed).
  network_mode = "host"

  env = [
    "POSTGRES_USER=${var.postgres_user}",
    "POSTGRES_PASSWORD=${var.postgres_password}",
    "POSTGRES_DB=${var.postgres_db}",
  ]

  mounts {
    type   = "volume"
    source = docker_volume.postgres_data.name
    # postgres:18+ images use the pg_ctlcluster layout (data lives in
    # /var/lib/postgresql/18/docker); mounting a volume directly at
    # /var/lib/postgresql/data makes the 18 entrypoint refuse to start
    # ("unused mount/volume" crash-loop). Mount the parent instead.
    target = "/var/lib/postgresql"
  }

  healthcheck {
    test     = ["CMD-SHELL", "pg_isready -U ${var.postgres_user} -d ${var.postgres_db}"]
    interval = "5s"
    timeout  = "5s"
    retries  = 12
  }
}
