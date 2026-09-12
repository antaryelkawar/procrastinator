# Proposal: add-basic-auth

## Why

The procrastinator backend API currently has no authentication: any client that can reach the server can read and mutate any user's data. Basic Auth provides a simple, standard mechanism to restrict API access to configured credentials without introducing sessions, tokens, or an identity provider.

## What Changes

- Add HTTP Basic Authentication as a mandatory gate on every registered backend API route (all registered routes live under `/api/users/{userId}`).
- New middleware in the `Server.Routes()` middleware chain (`api/server.go`) that validates the `Authorization: Basic <credentials>` header before any handler runs — programmatically injected by the client; the browser's native Basic Auth prompt is not used.
- New configuration var `PROCRASTINATOR_BASIC_AUTH_USERS` (JSON array of `{"user","pass"}` pairs; grammar and bounds defined in the api-basic-auth delta) following the existing `config.Load` pattern, documented in `.env.example`.
- Requests with missing, malformed, or invalid credentials receive `401 Unauthorized` with a `WWW-Authenticate: Basic` response header and the standard `{"error": string}` envelope body (consistent with every other documented non-2xx response).
- The OpenAPI contract (`api/openapi.yaml`) gains a `basicAuth` security scheme; the TS client mutator (`ui/src/lib/api/generated/mutator.ts`) and the upload helper send the configured credentials from the frontend (`VITE_API_BASIC_AUTH_USER` / `VITE_API_BASIC_AUTH_PASSWORD`).
- **BREAKING**: unauthenticated API requests are rejected. Any existing tooling calling the API must send Basic Auth credentials.

## Security Posture (requirements, not silence)

- **Credential comparison** SHALL be constant-time (timing-attack mitigation) — required in the basic-auth spec.
- **Credential storage** is plaintext in the deployment environment's env vars, explicitly accepted (hashed storage is a non-goal).
- **Transport** assumes TLS termination in any non-local deployment; local dev may run plain HTTP on `:8080`, which is explicitly an accepted, documented trade-off.

## Out of Scope / Non-Goals

- **Rate limiting / brute-force protection (lockout, delays)** — not in scope for this change.
- **CORS handling changes / preflight exemption** — not in scope; no CORS middleware exists (the UI uses a Vite dev proxy) and none is introduced here.
- **Hashed or vault-backed credential storage** — not in scope; plaintext env vars are the accepted storage form.
- **HTTPS/TLS termination in backend code** — not in scope; TLS is assumed to be handled by the deployment environment, and plain-HTTP local dev is explicitly allowed.
- **Per-user accounts, sessions, tokens, password rotation tooling** — out of scope; Basic Auth shares one application-level credential set and fills no user-identity role (multitenancy is unchanged).

## Capabilities

### New Capabilities
- `api-basic-auth`: HTTP Basic Authentication for the backend API — credential configuration (exact variable, grammar, bounds), request authentication behavior, 401 responses, and which routes are protected.

### Modified Capabilities
- `api-contract`: the OpenAPI document must declare the `basicAuth` security scheme and apply it to the API; 401 responses use the existing error envelope.
- `multitenancy`: path-tenancy user resolution (`/api/users/{userId}`) is unchanged, but requests must now additionally pass Basic Auth before tenancy resolution applies.

## Impact

- **Code**: `api/server.go` (middleware chain), new auth middleware package, `api/config/config.go`, `.env.example`, `api/openapi.yaml` + regenerated `api/gen` and orval TS client, `ui/src/lib/api/generated/mutator.ts`, `ui/src/lib/api/upload.ts` (XHR upload auth header).
- **Dependencies**: no new third-party dependencies (Go stdlib handles Basic parsing).
- **Systems**: local dev and deployments must provision credentials via env vars; the frontend bakes build-time credentials into the bundle (accepted exposure model, see spec).
- **Tests**: existing integration tests that call the API without credentials will need to send credentials or expect 401.
