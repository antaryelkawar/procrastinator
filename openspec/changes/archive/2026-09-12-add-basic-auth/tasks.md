# Tasks: add-basic-auth

## 1. Backend configuration

- [x] 1.1 [backend] Add `PROCRASTINATOR_BASIC_AUTH_USERS` parsing and fail-fast validation to `config.Load` in `procrastinator-backend/config/config.go` (JSON array of `{"user","pass"}`, 1–16 pairs, user 1–64 chars, pass 8–128 chars; `BasicAuthUsers []Credential` field — `Credential{User, Pass string}` is **owned by the `config` package**, per design D1; error messages name the variable and violated constraint). Extend tests in `procrastinator-backend/config/config_test.go`.
- [x] 1.2 [backend] Document `PROCRASTINATOR_BASIC_AUTH_USERS` exact grammar and bounds in `procrastinator-backend/.env.example`.

## 2. Backend auth middleware

- [x] 2.1 [backend] Create `procrastinator-backend/api/auth/middleware.go`: `Middleware(pairs []config.Credential) func(http.Handler) http.Handler` (type from `config`, per 1.1) — parse `Authorization: Basic` header, constant-time comparison (`subtle.ConstantTimeCompare` on fixed-max-length-padded user/pass fields against each configured pair, combined with constant-time AND), reject with `401` + `WWW-Authenticate: Basic` + `httpx.WriteErrorEnvelope`. Add unit test `middleware_test.go` (valid, missing header, non-Basic scheme, wrong creds, timing-observability parity of status/headers/body using equal-length inputs).
- [x] 2.2 [backend] Wire the middleware into `procrastinator-backend/api/server.go` `Routes()` so auth is **outermost**: append `auth.Middleware(...)` **last** in the `Middlewares` slice of `gen.ChiServerOptions` (the generated fold `handler = middleware(handler)` makes the last element the first to execute — see design D3), after `httpx.UserMiddleware` and `httpx.MaxBodyMiddleware` in the slice. Add/extend integration test in the api suite asserting: 401-without-creds, 401-on-registered-user-path-with-bad-creds with **zero registry lookups** (fake `repo.UserRegistry` — order pin), 200 on valid creds.
- [x] 2.3 [backend] Inject credentials into the server: add the parsed `[]config.Credential` to `api.New(...)` in `procrastinator-backend/api/server.go` (stored on `api.Server`, consumed by `Routes()`), and pass `cfg.BasicAuthUsers` through in `procrastinator-backend/api/cmd/procrastinator/main.go`. Verify `main` still fails fast when `config.Load` returns the validation error (existing behavior, extend test if one exists).

## 3. OpenAPI contract + regeneration

- [x] 3.1 [backend] Add `components.securitySchemes.basicAuth` (`type: http`, `scheme: basic`) and global `security: [{basicAuth: []}]` to `procrastinator-backend/api/openapi.yaml`; ensure documented 401 responses use the `{"error": string}` envelope and reword existing 401 descriptions currently attributed only to path-identity ("Missing or invalid user identity") to also cover failed Basic Auth.
- [x] 3.2 [backend] Regenerate Go server stubs with the exact repo command `make codegen-go` (directive lives in `api/gen.go`, outputs `api/gen/openapi.gen.go`); run `make codegen-drift-check` and `go test ./api/... ./config/...`.
- [x] 3.3 [ui] Regenerate orval TS client artifacts under `ui/src/lib/api/generated/` from the updated contract with the exact repo commands `make codegen-ts` and `make codegen-ts-client` (orval config at `ui/orval.config.ts`); run `make codegen-drift-check` and the client drift test.

## 4. UI credential reuse

- [x] 4.1 [ui] Create `ui/src/lib/api/auth.ts`: `basicAuthHeader()` building `Authorization: Basic <base64>` from `import.meta.env.VITE_API_BASIC_AUTH_USER` / `VITE_API_BASIC_AUTH_PASSWORD` using UTF-8-safe encoding (`btoa(String.fromCharCode(...new TextEncoder().encode(...)))` — plain `btoa` breaks non-Latin1 credentials that the backend accepts), throwing at module load with a clear message naming the missing variable. Add unit test `ui/src/lib/api/auth.test.ts` (header present/correct when vars set; throws naming missing variable when absent; non-ASCII password encodes to the same base64 the backend would compute).
- [x] 4.2 [ui] In `ui/src/lib/api/generated/mutator.ts`, merge `basicAuthHeader()` into the outgoing `init.headers` of `customFetch` (augment, don't replace). Extend the existing mutator test to assert the header is present on every request.
- [x] 4.3 [ui] In `ui/src/lib/api/upload.ts`, merge `basicAuthHeader()` into the `headers` record applied by `performUpload`/`uploadDocument`/`createImportBatch` XHR paths. Extend the existing upload test to assert the header is set.
- [x] 4.4 [ui] Document `VITE_API_BASIC_AUTH_USER` / `VITE_API_BASIC_AUTH_PASSWORD` in `ui/.env.example`, and confirm `ApiError` surfacing of 401 responses through the existing mutator/upload error handlers (test-only change if behavior is already correct).

## 5. Existing test migration + verification

- [x] 5.1 [backend] Introduce a shared fixture credential set (test-only) and attach the `Authorization` header in the existing api integration test helpers (`api/handlers_test.go` `testEnv`/`do`/`uploadFile`) and `e2e/tenancy_e2e_test.go` (or construct the test `Server` with the fixture creds), so all previously-unauthenticated API call sites keep passing behind the new middleware. Must complete before 5.2.
- [x] 5.2 [backend] Run full backend test suite (`go test ./...` in `procrastinator-backend`) and confirm all tests pass with the auth middleware in place.
- [x] 5.3 [ui] Run frontend test/build (`npm test` / `npm run build` in `ui/`) and confirm no regressions; verify a 401 from bad server credentials surfaces as `ApiError` (mock or integration).
