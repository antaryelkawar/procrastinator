variable "docker_host" {
  description = "SSH or Unix socket address of the Unraid Docker daemon (e.g. ssh://root@unraid-host or unix:///var/run/docker.sock)"
  type        = string
  default     = "ssh://root@unraid-host"
}

variable "github_owner" {
  description = "GitHub owner (user or organization) for the target repository"
  type        = string
}

variable "github_token" {
  description = "GitHub personal access token with repo admin scope (set via TF_VAR_github_token)"
  type        = string
  sensitive   = true
}

variable "github_repository" {
  description = "GitHub repository in owner/name form (e.g. owner/procrastinator)"
  type        = string
}

variable "registry_url" {
  description = "Container registry URL (overridable; defaults to the fixed JobOptimizer registry)"
  type        = string
  default     = "registry.yelkawar.com"
}

variable "registry_username" {
  description = "Username for pulling images from registry.yelkawar.com (set via TF_VAR_registry_username)"
  type        = string
  sensitive   = true
}

variable "registry_password" {
  description = "Password/token for pulling images from registry.yelkawar.com (set via TF_VAR_registry_password)"
  type        = string
  sensitive   = true
}

variable "backend_image_tag" {
  description = "Docker image tag for procrastinator-backend (default latest = CI-pushed image)"
  type        = string
  default     = "latest"
}

variable "ui_image_tag" {
  description = "Docker image tag for procrastinator-ui (default latest = CI-pushed image)"
  type        = string
  default     = "latest"
}

variable "backend_host_port" {
  description = "Host port the backend listens on directly (host networking; no port mapping — the container binds :<port>)"
  type        = number
  default     = 8321
}

variable "ui_host_port" {
  description = "Host port published for the UI container (container port 80) — the UI is the exception that stays on the default bridge network, so this is still a published/mapped host port (8322 -> container 80)"
  type        = number
  default     = 8322
}

variable "backend_database_url" {
  description = "Postgres connection URL for the backend. With host networking the Postgres container listens on the host, so use localhost:5432 (not a container name); set via TF_VAR_backend_database_url"
  type        = string
  default     = "postgresql://pgadmin:pgadmin@localhost:5432/procrastinator"
  sensitive   = true
}

variable "backend_llm_api_key" {
  description = "LLM API key for the backend (set via TF_VAR_backend_llm_api_key)"
  type        = string
  sensitive   = true
}

variable "backend_llm_model" {
  description = "LLM model name for the backend (non-secret)"
  type        = string
}

variable "backend_basic_auth_users" {
  description = "Basic-auth user list for the backend, CSV of user:bcrypt-hash (set via TF_VAR_backend_basic_auth_users)"
  type        = string
  sensitive   = true
}

variable "backend_storage_host_path" {
  description = "Host filesystem path for backend storage volume (pre-created Unraid share or directory; mounted at /app/storage)"
  type        = string
}

variable "postgres_user" {
  description = "Postgres superuser name for the procrastinator-postgres container (set via TF_VAR_postgres_user)"
  type        = string
  default     = "pgadmin"
}

variable "postgres_password" {
  description = "Postgres password for the procrastinator-postgres container (set via TF_VAR_postgres_password)"
  type        = string
  sensitive   = true
}

variable "postgres_db" {
  description = "Postgres database name for the procrastinator-postgres container"
  type        = string
  default     = "procrastinator"
}

variable "postgres_host_port" {
  description = "Host port the Postgres container listens on directly (host networking; fixed 5432 for the postgres image)"
  type        = number
  default     = 5432
}
