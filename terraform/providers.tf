provider "docker" {
  host = var.docker_host
}

provider "github" {
  owner = var.github_owner
  token = var.github_token
}


