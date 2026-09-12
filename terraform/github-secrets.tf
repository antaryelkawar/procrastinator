resource "github_actions_secret" "registry_url" {
  repository      = var.github_repository
  secret_name     = "REGISTRY_URL"
  plaintext_value = var.registry_url
}

resource "github_actions_secret" "registry_username" {
  repository      = var.github_repository
  secret_name     = "REGISTRY_USERNAME"
  plaintext_value = var.registry_username
}

resource "github_actions_secret" "registry_password" {
  repository      = var.github_repository
  secret_name     = "REGISTRY_PASSWORD"
  plaintext_value = var.registry_password
}
