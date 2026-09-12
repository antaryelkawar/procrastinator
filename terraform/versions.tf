terraform {
  required_version = ">= 1.5"

  required_providers {
    docker = {
      source  = "kreuzwerker/docker"
      version = "~> 3.5"
    }
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }
}
