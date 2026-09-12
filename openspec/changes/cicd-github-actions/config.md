# Registry-as-secrets — open configuration item (cicd-github-actions)

Documents the registry-as-secrets contract for the `cicd-github-actions` change.

## Secrets the workflows read

Both workflows (`.github/workflows/backend.yaml`, `.github/workflows/ui.yaml`) read exactly these GitHub Actions secrets:

- `secrets.REGISTRY_URL`
- `secrets.REGISTRY_USERNAME`
- `secrets.REGISTRY_PASSWORD`

## Repo-level secrets to configure

These three must be configured at the repository level (GitHub repo → Settings → Secrets and variables → Actions) before push works:

- `REGISTRY_URL`
- `REGISTRY_USERNAME`
- `REGISTRY_PASSWORD`

## Open configuration item: registry host

The registry host is an open configuration item — it has NOT been decided for this repo. JobOptimizer's `registry.yelkawar.com` is a reference example only, not a decision for this repo.

## Image naming convention

- `<registry>/procrastinator-backend:<tag>`
- `<registry>/procrastinator-ui:<tag>`

where `<tag>` is `latest` and the commit SHA (`<github.sha>`), and `<registry>` = the value of `secrets.REGISTRY_URL`.

## Gating behavior

If the secrets are absent, CI still goes green — it runs vet/test/build (backend: `go vet`/`go test`/build; ui: lint/test/build) plus a local no-push Docker build, and the push step is skipped. Missing credentials do NOT fail the workflow.
