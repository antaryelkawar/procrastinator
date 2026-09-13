output "backend_container_id" {
  description = "Docker container ID for procrastinator-backend"
  value       = docker_container.backend.id
}

output "ui_container_id" {
  description = "Docker container ID for procrastinator-ui"
  value       = docker_container.ui.id
}

output "backend_host_port" {
  description = "Host port the backend listens on directly (host networking; binds :<port>)"
  value       = var.backend_host_port
}

output "ui_host_port" {
  description = "Host port published for the UI container (container port 80)"
  value       = var.ui_host_port
}

output "backend_image_ref" {
  description = "Full image reference for procrastinator-backend"
  value       = "${var.registry_url}/procrastinator-backend:${var.backend_image_tag}"
}

output "ui_image_ref" {
  description = "Full image reference for procrastinator-ui"
  value       = "${var.registry_url}/procrastinator-ui:${var.ui_image_tag}"
}

output "github_secret_names" {
  description = "Names of the three GitHub Actions secrets (never values)"
  value = [
    github_actions_secret.registry_url.secret_name,
    github_actions_secret.registry_username.secret_name,
    github_actions_secret.registry_password.secret_name,
  ]
}

output "postgres_container_id" {
  description = "Docker container ID for procrastinator-postgres"
  value       = docker_container.postgres.id
}

output "postgres_port" {
  description = "Host port the Postgres container listens on directly (host networking)"
  value       = var.postgres_host_port
}
