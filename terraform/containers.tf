# Backend + UI containers for procrastinator

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
