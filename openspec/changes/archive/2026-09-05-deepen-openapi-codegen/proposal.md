# Proposal: deepen-openapi-codegen

## Why

The `openapi-codegen-pipeline` change made `procrastinator-backend/api/openapi.yaml` the source of truth and generated the server types/route scaffolding (oapi-codegen, loose `chi-server` mode), a TypeScript type map (`openapi-typescript`), and the docs — with drift checks. But the generated layer is only half-used: the Go handlers still hand-parse every request (manual `r.FormFile` / `r.FormValue` / `r.ParseMultipartForm` / `json.NewDecoder(r.Body)`) and hand-write every response (ad-hoc `httpx.WriteJSON` / `httpx.WriteError` at each call site), and the UI still talks to the API through a hand-built, string-based `apiJson(method, path, body)` bridge over the generated types. This change completes the codegen: the server binds requests and encodes responses through the generated machinery, and the UI consumes a per-operation typed client — so the transport layer is fully generated and the hand-rolled request/response code is eliminated.

## What Changes

- **Server — request binding (deepen):** Switch the oapi-codegen server generation from loose `chi-server` to the strict server mode so that every documented operation's request inputs (path/query parameters + JSON or `multipart/form-data` body) are bound into the generated Go types and passed to the handler. Handlers SHALL consume the generated request types and SHALL NOT hand-parse documented inputs (the manual `r.FormFile` / `r.FormValue` / `r.ParseMultipartForm` / `json.NewDecoder(r.Body)` reads are removed).
- **Server — response encoding (deepen):** Route all success and error responses through the generated encoder path wired via `HandlerWithOptions` — a `ResponseEncoder` for success bodies and an `ErrorEncoder` / error handler for the `{"error": string}` envelope. The ad-hoc `httpx.WriteJSON` / `httpx.WriteError` calls at each handler call site are removed; handlers return the generated response object (or an error) for the encoder to emit. The wire format and the domain-error→status mapping are unchanged.
- **Client — per-operation typed SDK (deepen):** Generate a per-operation typed client from the same `openapi.yaml` (orval or an equivalent per-operation generator) and re-point the UI (`ui/src/lib/api`: `hooks.ts`, `upload.ts`, `client.ts`) to it. The manual `apiJson` / `apiVoid` path-string calls and the hand-built resource→path string bridge are replaced by typed per-operation functions; multipart uploads keep progress reporting.
- **Pipeline — reproducibility & budgets (extend):** The new/changed generators (strict server gen; per-operation client gen) are pinned, committed, and drift-checked like the existing ones, and the full codegen run + drift check stay within the existing time budgets (30s / 30s) with the document under 200 KB.
- **PRESERVED:** The observable HTTP contract — routes, methods, status codes, request/response wire shapes (snake_case, exact-decimal string money, `link_conflicting` always present, `lines` nullable), and the `{"error": string}` envelope — is unchanged. Only the mechanism (generated request binding + response encoding + client SDK) changes, not the contract.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `api-contract`: Deepens the existing OpenAPI single-source capability — the server now binds documented requests and encodes documented responses through the generated machinery (no hand-parsed inputs, no ad-hoc response writes), and the UI consumes a per-operation typed client (no manual `apiJson` path strings). The pipeline's reproducibility and budgets now cover the new/changed generators. (The base capability spec is the archived `openapi-codegen-pipeline` delta at `openspec/changes/archive/2026-09-04-openapi-codegen-pipeline/specs/api-contract/spec.md`; it is not yet folded into `openspec/specs/api-contract/`, so this change contributes its requirements as ADDED.)

## Impact

- **Code (backend):** `procrastinator-backend/api/gen/openapi.gen.go` is regenerated in strict server mode (handler signatures move from the loose `(w, r, userId, …)` form to the strict `(ctx, RequestObject) (ResponseObject, error)` form). The handlers in `handlers.go`, `finance_accounts.go`, `finance_movements.go`, `finance_import.go`, and `households.go` are reworked to consume generated request types and return generated response/error objects. The per-domain error-mapping helpers (`writeProcessError`, `writeFinanceError`, and the household equivalent) move into the centralized error-encoder path. `httpx.WriteJSON` / `httpx.WriteError` usage for documented operations is removed. The `Makefile` and the Go drift/budget tests update the pinned oapi-codegen flags (`-generate types,chi-server` → strict mode).
- **Code (UI):** `ui/src/lib/api` gains a committed per-operation typed client. `client.ts`, `hooks.ts`, and `upload.ts` re-point from the manual `apiJson` / `apiVoid` string bridge to the per-operation functions. `schema.ts` re-exports adapt to the client's types. `package.json` / `Makefile` gain the pinned client generator plus a drift check for its output.
- **Dependencies:** adds a pinned per-operation TypeScript client generator (orval or equivalent); the server generator flag set changes to strict mode (same pinned oapi-codegen version).
- **Build/CI:** the codegen, drift-check, and budget-check targets extend to the new/changed generators; the strict `gen/openapi.gen.go` and the new per-operation client artifact are committed and drift-checked.
- **Risk:** (1) Switching to strict server generation changes the generated interface — the handler refactor must preserve every status code and wire shape (guarded by the existing handler/finance/household/e2e tests + the drift check). (2) The new client generator must reproduce the exact wire conventions and `ApiError` handling, else the UI breaks (guarded by the existing UI tests + a new drift check + budget test).

## Out of Scope

- **No new API endpoints, routes, or behavioral changes.** The observable HTTP contract is preserved; only the mechanism for producing/consuming the transport layer changes.
- **No changes to domain logic, services, repositories, or database schema.** Code generation and the transport refactor target the HTTP layer only.
- **No authentication or authorization changes.** Path tenancy (`/api/users/{userId}`) is unchanged.
- **No UI component, page, or interaction changes.** Only the API transport layer in `ui/src/lib/api` is re-pointed; React components/pages are untouched.
- **No new language targets or non-HTTP transports.** Still Go server + TypeScript client + generated docs; no WebSockets/SSE.

## Non-Functional Requirements

- **Deterministic output:** Generated artifacts (strict server gen, per-operation client, TS types, docs) remain byte-for-byte reproducible for an identical pinned tool + flags + document.
- **Codegen execution time:** A full run of all generators (now including strict server gen and the per-operation client) stays under 30 seconds on a 2 vCPU / 4 GB CI runner.
- **OpenAPI document size:** The committed `openapi.yaml` remains under 200 KB.
- **Drift-check CI overhead:** The drift-check step (re-run all generators + diff) stays within 30 seconds.
