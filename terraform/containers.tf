# Backend + UI + Postgres containers for procrastinator

resource "docker_container" "backend" {
  name    = "procrastinator-backend"
  image   = "${var.registry_url}/procrastinator-backend:${var.backend_image_tag}"
  restart = "unless-stopped"

  ports {
    internal = 8080
    external = var.backend_host_port
  }

  env = [
    "PROCRASTINATOR_DATABASE_URL=${var.backend_database_url}",
    "PROCRASTINATOR_LLM_API_KEY=${var.backend_llm_api_key}",
    "PROCRASTINATOR_LLM_MODEL=${var.backend_llm_model}",
    "PROCRASTINATOR_BASIC_AUTH_USERS=${var.backend_basic_auth_users}",
  ]

  volumes {
    host_path      = var.backend_storage_host_path
    container_path = "/app/storage"
  }

  networks_advanced {
    name = docker_network.procrastinator.id
  }
}

resource "docker_container" "ui" {
  name    = "procrastinator-ui"
  image   = "${var.registry_url}/procrastinator-ui:${var.ui_image_tag}"
  restart = "unless-stopped"

  ports {
    internal = 80
    external = var.ui_host_port
  }

  networks_advanced {
    name = docker_network.procrastinator.id
  }
}

resource "docker_container" "postgres" {
  name    = "procrastinator-postgres"
  image   = "postgres:18"
  restart = "unless-stopped"

  ports {
    internal = 5432
    external = var.postgres_host_port
  }

  env = [
    "POSTGRES_USER=${var.postgres_user}",
    "POSTGRES_PASSWORD=${var.postgres_password}",
    "POSTGRES_DB=${var.postgres_db}",
  ]

  mounts {
    type   = "volume"
    source = docker_volume.postgres_data.name
    target = "/var/lib/postgresql/data"
  }

  networks_advanced {
    name = docker_network.procrastinator.id
  }

  healthcheck {
    test     = ["CMD-SHELL", "pg_isready -U ${var.postgres_user} -d ${var.postgres_db}"]
    interval = "5s"
    timeout  = "5s"
    retries  = 12
  }
}
