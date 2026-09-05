## Context

`openapi-codegen-pipeline` (base, commit `2bd19ca`) made `procrastinator-backend/api/openapi.yaml` the source of truth and committed three generated artifacts with drift + budget checks: a Go server scaffold (`api/gen/openapi.gen.go`, oapi-codegen **v2.8.0** in loose `chi-server` mode, `-generate types,chi-server`), a TS type map (`ui/src/lib/api/generated/paths.d.ts`, `openapi-typescript` 7.13.0), and docs (`api/docs/index.html`, `redoc-cli` 0.13.21). The build wiring is the repo-root `Makefile`; the Go drift/budget tests live in `procrastinator-backend/api/gen/` (`drift_test.go`, `codegen_test.go`, `budget_test.go`, `lockstep_test.go`) and the TS drift check is `ui/src/lib/api/codegen.drift.test.ts`.

But the generated layer is only half-used. Today the transport is still hand-rolled:

- **Server request binding.** Every handler parses its own documented inputs: `r.ParseMultipartForm` / `r.FormFile` / `r.FormValue` (the two multipart ops `UploadDocument`, `CreateImportBatch`), `json.NewDecoder(r.Body)` (the JSON-body ops), and `r.URL.Query()`/`gen.ListMovementsParams` (`ListMovements`). See `handlers.go`, `finance_import.go`, `finance_movements.go`, `finance_accounts.go`, `households.go`.
- **Server response encoding.** Every handler writes its own response: `httpx.WriteJSON` for success and `httpx.WriteError` for the `{"error": string}` envelope, with four near-duplicate domain→status mappers (`writeProcessError`, `writeFinanceError`, `writeImportError`, `writeHouseholdError`) each calling `httpx.WriteError` per case.
- **Client.** The UI talks to the API through a hand-built, string-based bridge: `apiJson(method, path, body)` / `apiVoid` in `client.ts`, a `Resource` union + `PathFor<R>` conditional-type mapping in `client.ts`, and `movementsResource` string building in `hooks.ts`; `upload.ts` builds multipart URLs by hand. The generated `paths.d.ts` is used only for the request/response types.

Pinned dependencies: `go-chi/chi/v5 v5.3.2`, `oapi-codegen/oapi-codegen/v2 v2.8.0`, `oapi-codegen/runtime v1.7.0`. The observable HTTP contract (routes, methods, status codes, snake_case wire shapes, exact-decimal money, `{"error": string}` envelope, 204-no-body, path tenancy under `/api/users/{userId}`) is preserved by this change; only the *mechanism* changes.

## Goals / Non-Goals

**Goals:**
- Deepen server request binding: handlers consume the generated per-operation request types (path/query params, JSON body, multipart body) instead of hand-parsing documented inputs.
- Deepen server response encoding: handlers return generated response objects (or an error); a single centralized encoder path (wired via the generated strict-handler options) emits every success body and every `{"error": string}` envelope. The per-handler `httpx.WriteJSON`/`httpx.WriteError` calls and the four domain mappers' write calls are removed.
- Replace the UI string bridge with a per-operation typed client generated from the same `openapi.yaml` (orval), re-pointing `client.ts`/`hooks.ts`/`upload.ts`; keep the TanStack Query keys/invalidation and multipart progress.
- Extend reproducibility + budgets to the new/changed generators (strict server gen, per-operation client gen): pinned, committed, drift-checked, within 30s full / 30s drift / 200 KB doc.

**Non-Goals:**
- No new endpoints/routes, no behavioral change, no contract change (the wire is byte-identical).
- No domain/service/repository/DB changes. The ingest/ledger/statement/household services are untouched (handlers call them the same way).
- No auth change; path tenancy is unchanged.
- No UI component/page/interaction change (only `ui/src/lib/api`).
- No new language targets or non-HTTP transports.

## Decisions

### D1 — Server: generate the strict chi server with `-generate types,chi-server,strict-server`

The proposal says "switch from loose `chi-server` to the strict server." In oapi-codegen **v2.8.0** there is no standalone `strict-server` output and no `strict-chi` template: the strict server is emitted by the `strict-server` generate option (`Generate.Strict`, `configuration.go:180-181`) and it binds to the router selected alongside it — `operations.go:2378` maps `ChiServer || GorillaServer || StdHTTPServer` to `strict/strict-interface.tmpl` + `strict/strict-http.tmpl`. **Verified by generating `types,chi-server,strict-server` against the document.** So the flag set becomes `types,chi-server,strict-server` (keep `chi-server` for the chi router + `HandlerWithOptions`; add `strict-server`).

What this adds to `api/gen/openapi.gen.go` (the loose `ServerInterface` + chi router are still emitted, so `lockstep_test.go` / `codegen_test.go` scaffolding assertions keep passing):
- `StrictServerInterface` — one method per operation: `OpID(ctx context.Context, request OpIDRequestObject) (OpIDResponseObject, error)`.
- Per-op `OpIDRequestObject`: path params as fields (`UserId`, `AssetId`, `Id`, `HouseholdId`); JSON body as `*OpIDJSONRequestBody` (an alias of the schema type); query params as `Params OpIDParams` (`ListMovementsRequestObject.Params ListMovementsParams`); **multipart body as `*multipart.Reader`** (`UploadDocumentRequestObject.Body`, `CreateImportBatchRequestObject.Body`).
- Per-op `OpIDResponseObject` interface (`VisitOpIDResponse(w) error`) + concrete types `OpID<code>JSONResponse <Schema>` / `OpID<code>JSONResponse Error` / `OpID204Response struct{}`, each serializing to JSON with the documented status + `Content-Type: application/json` (204 writes the status only, no body).
- `StrictHTTPServerOptions{RequestErrorHandlerFunc, ResponseErrorHandlerFunc func(w, r, err)}` and `NewStrictHandler(ssi, middlewares)` / `NewStrictHandlerWithOptions(ssi, middlewares, options)` which return a `ServerInterface` (a `*strictHandler`) that adapts the strict interface to the loose chi router.

The strict handler (generated) does the **binding**: for JSON it `json.NewDecoder(r.Body).Decode(&body)` into `OpIDJSONRequestBody` (on non-EOF error → `RequestErrorHandlerFunc`; on empty body/EOF it leaves `request.Body = nil`); for multipart it calls `r.MultipartReader()` (on error → `RequestErrorHandlerFunc`) and stores the reader in `request.Body`. Then it invokes the strict handler; on `(resp, err)` with `err != nil` → `ResponseErrorHandlerFunc`; on success → `resp.VisitOpIDResponse(w)`.

**Alternatives considered:**
- *Switch router to std-http/gin/echo for the strict server.* Rejected — it would drop chi (the middleware, routing, and the `UserMiddleware` all depend on chi) for no contract benefit, and it's a larger, riskier change.
- *Keep loose `chi-server` and hand-write binding/encoding.* Rejected — that is exactly the half-used state this change removes.
- *A hand-rolled strict wrapper.* Rejected — the generated strict server is deterministic, drift-checked, and the whole point of the capability.

### D2 — Request binding: handlers consume generated types; multipart via `runtime.BindMultipart`

Handlers are rewritten to the strict signature and read inputs only from the generated `OpIDRequestObject`:
- **Path/query:** `request.UserId`, `request.Params.AccountId`, etc. (no `r.URL.Query()`).
- **JSON body:** `request.Body` (`*OpIDJSONRequestBody`). Because the strict server leaves `request.Body == nil` on an empty body, each JSON-body handler **must** return 400 when `request.Body == nil` (and when a required field is blank/invalid), preserving the current "invalid JSON body" / field-validation 400s.
- **Multipart:** `request.Body` is a `*multipart.Reader`; the handler binds it with the generated/runtime mechanism `runtime.BindMultipart(&body, *request.Body)` (oapi-codegen `runtime` v1.7.0, `bindform.go:40`) into `gen.UploadDocumentMultipartBody` / `gen.CreateImportBatchMultipartBody`. This is the "generated bind/parse" that replaces `r.ParseMultipartForm` / `r.FormFile` / `r.FormValue`.

**Size limit / 413 (wire-parity):** the strict server does not wrap the body, so the current `http.MaxBytesReader` + `isMaxBytesErr` → 413 behavior is preserved by installing `http.MaxBytesReader` in a **chi-level middleware** (applied in `Routes()` alongside `UserMiddleware`) scoped to the two upload routes with the existing per-route limits (`maxBytes` for `POST /…/documents`, `maxStatementBytes` for `POST /…/finance/import-batches`). When an oversized body is read during `runtime.BindMultipart`, it surfaces a `*http.MaxBytesError`; the upload handler detects it (`errors.As`) and returns 413 `{"error":"upload exceeds size limit"}` — the same mapping as today. (A `StrictMiddlewareFunc` cannot do this: the `strictHandler` creates the multipart reader *before* the strict middleware chain runs, so the limit must be installed at the chi layer where `w`/`r` are available.)

### D3 — Response encoding: handlers return generated objects; one centralized error encoder

- **Success:** handlers return the generated response object, e.g. `gen.ListAssets200JSONResponse(out)` / `gen.CreateMovement201JSONResponse(toMovement(mv))` / `gen.DeleteMovement204Response{}`. The generated `Visit…Response` is the "ResponseEncoder" for success — it sets the documented status + `Content-Type` and serializes the body (204 emits no body). Handlers never call `httpx.WriteJSON` / `w.Write` / `w.WriteHeader`.
- **Errors:** handlers return `(nil, err)` for every failure. `err` is a small typed `*apiError{status, msg}` (new, in the `api` package) whose `(status, msg)` come from **pure** mapping functions that replace the current write-based mappers: `mapIngestError`, `mapLedgerError`, `mapStatementError`, `mapHouseholdError`, plus direct `*apiError{400, "missing file field"}` etc. for validation. A **single centralized `ResponseErrorHandlerFunc`** (wired in `NewStrictHandlerWithOptions`) unwraps `*apiError` and writes the `{"error": msg}` envelope with that status + `Content-Type: application/json` (falling back to 500 for a non-`*apiError`). The **`RequestErrorHandlerFunc`** handles binding/parse failures: malformed JSON / multipart decode → 400, `*http.MaxBytesError` → 413 — always the `{"error": string}` envelope (the generated default would emit plain-text `http.Error`, so both must be replaced).
- This centralizes all error emission in two functions, removes every per-handler `httpx.WriteError`, and keeps the exact domain→status mapping (413/415/422/502 for ingest, 422/409 for statement, 409 for ledger conflict, 404 for `repo.ErrNotFound`, 403 for non-member, etc.). `httpx.WriteJSON`/`WriteError` are deleted once no handler or middleware references them (the chi `UserMiddleware`'s pre-handler 400/404/500 responses, which fire before the strict handler, are rewritten to emit the same `{"error": string}` envelope via the shared encoder helper).

**Why not return a per-op `OpID400JSONResponse` for every error:** the generated error types are per-operation, but the domain→status mapping is shared across operations and lives in the service errors; a single `*apiError` + central encoder keeps one mapping table and one envelope writer, which is the observable "centralized encoder path" the spec requires.

### D4 — Client: orval per-operation typed client (fetch mode) + `ApiError` mutator + XHR upload

Generate a per-operation client with **orval** (the de-facto per-operation OpenAPI→TS generator; the proposal allows "orval or equivalent"). Orval config lives at `ui/orval.config.ts`, input `target: ../procrastinator-backend/api/openapi.yaml`, output `target: src/lib/api/generated/client`, `client: 'fetch'` (not the `react-query` preset) so we get one typed function per `operationId` (`listAssets({ userId })`, `createMovement({ userId, data })`, …) without orval owning the query layer. A pinned orval version is added to `ui/package.json` devDependencies + the Makefile/budget wiring.
- **`ApiError` + network parity:** a custom orval **mutator** wraps `fetch` and reproduces `apiFetch`'s contract exactly — on `!response.ok` build `ApiError(status, errorCopy(status), errorDetailFromBody(...))`; on a thrown/timeout fetch build `ApiError(NETWORK_STATUS, errorCopy(NETWORK_STATUS))` (status `0`). This preserves the `ApiError` envelope handling **and** the network/timeout behavior (spec scenario "Network failure surfaces the same error").
- **Re-pointing:** `client.ts` drops `apiJson`/`apiVoid` + the `Resource`/`PathFor`/`buildUrl` string bridge; `hooks.ts` keeps its query keys + D5 invalidation map but its `queryFn`/`mutationFn` call the generated per-operation functions; `schema.ts` re-exports adapt to the generated types.
- **Multipart + progress:** orval's fetch functions have no XHR progress, so the two upload operations keep the existing XHR `performUpload` (progress preserved) but are typed against the generated multipart request/response types and route URL/body through the generated client config.

**Alternatives considered:**
- *orval `react-query` preset.* Rejected — it would own hook generation and fight the existing D5 invalidation map; a fetch-mode client lets `hooks.ts` stay the query authority.
- *Extend `openapi-typescript` + hand-write per-op functions.* Rejected — that reproduces the string bridge we are removing; orval emits real per-operation functions from the same document.
- *`axios` client mode.* Rejected — the current transport is `fetch`/XHR; staying on `fetch` keeps the mutator and error path identical to today.

### D5 — Pipeline: pin, commit, drift-check, and budget the new generators

- `Makefile`: `OAPI_CODEGEN_GENERATE := types,chi-server` → `types,chi-server,strict-server`; add a `codegen-ts-client` target (pinned orval) and fold it into `codegen`; extend `codegen-drift-check` and `budget-check` to cover the client artifact.
- Go tests in `api/gen/`: update the pinned `-generate` flag in `drift_test.go` / `codegen_test.go` / `budget_test.go` to `types,chi-server,strict-server` (the committed `openapi.gen.go` is regenerated once and re-committed). `lockstep_test.go` / `codegen_test.go` scaffolding assertions still hold because the loose `ServerInterface` is still emitted.
- UI: keep `codegen.drift.test.ts` (openapi-typescript → `paths.d.ts`); add an orval drift test (re-run orval, diff the committed client) and a budget test (full codegen incl. orval < 30s, drift < 30s, doc < 200 KB).

## Review findings disposition

The spec review (cycle 1, PASS, 4 majors) is addressed as follows; the delta spec was edited so the flagged scenarios are behavioral, and the design/tasks implement the observable verification.

- **CRIT-S-02 — "No hand-parsed request inputs in handlers" (was spec.md:32).** Root fix: the scenario is now *"Handlers consume generated request types with unchanged outputs"* — observable via the handler signature accepting the generated request type **and** the before/after status+body equality. Verified by the existing handler tests (multipart/JSON/query ops) plus the new strict-handler tests; no source-inspection assertion.
- **CRIT-S-02 — "No ad-hoc response writes in handlers" (was spec.md:57).** Root fix: the scenario is now *"Generated response objects produce the documented wire output"* — observable via HTTP response comparison (status/headers/body byte-identical). Verified by a wire-parity test that issues the same requests before/after and compares responses, and by the existing per-status tests.
- **CRIT-S-02 — "No manual path-string calls remain" (was spec.md:85).** Root fix: the scenario is now *"UI calls route through typed per-operation functions"* — observable via the TS types (compile-time, no `any`/untyped bodies) and the emitted HTTP requests (method/path/body) matching pre-change. Verified by the existing UI tests + a request-capture test.
- **CRIT-S-04 — client network/timeout edge case (spec.md:63-93).** Added scenario *"Network failure surfaces the same error"*; implemented by the orval mutator's fetch `catch` → `ApiError(0, …)` and verified by a client test that stubs a network failure/timeout.
- **CRIT-S-08 (minor) — spec names concrete Go/TS symbols.** The *requirements* stay behavioral (the named symbols are illustrative of the mechanism); the design above treats them as "the generated bind/parse path", "the centralized encoder", "the per-operation typed client". No spec churn required for a minor.

## Risks / Trade-offs

- **[Strict-server interface change breaks a status/wire shape]** → every handler is rewritten to return generated objects; guarded by the existing handler/finance/household/import tests + the new wire-parity test (byte-identical status/headers/body). The domain→status mapping is moved verbatim into the pure `mapXxxError` functions.
- **[The pinned `openapi_types.File` (runtime v1.7.0) does not expose the multipart part `Content-Type`], but `UploadDocument` passes `header.Header.Get("Content-Type")` to the ingest service (which builds the LLM data URI `data:<ct>;base64,…`).** → For `UploadDocument`, derive the MIME from the filename via `mime.TypeByExtension` (a documented mechanism nuance: MIME from the file's extension rather than the client-declared part header). It preserves the ingest contract (a MIME is still passed) and is arguably more robust; the existing ingest/handler tests are the guard (`mime.TypeByExtension(".pdf")` == `application/pdf`, matching the `service_test.go` expectation for `invoice.pdf`). `CreateImportBatch` is unaffected: `s.statement.Upload` already derives content type internally (only `filename`+`data` are passed). *Alternative rejected:* trusting the client header is not available through the generated `File` type.
- **[Oversized-upload 413 lost when the body wrapper is removed.]** → preserved by the chi-level `MaxBytesReader` middleware (D2) + `*http.MaxBytesError` → 413 in the upload handler, exactly as today.
- **[Empty-body / malformed-JSON 400 drift.]** the strict server treats an empty body as `request.Body == nil` (no auto-400). → each JSON-body handler returns 400 on nil/invalid body (D2), and malformed JSON is handled by the centralized `RequestErrorHandlerFunc` → 400 envelope.
- **[New dependency (orval) + a larger generated file] threaten the 30s/30s budgets and determinism.** → orval is pinned; the client artifact is committed and drift-checked; the budget tests now include orval and fail if full/drift exceed 30s. Determinism: same pinned tools + flags + document = same bytes (verified pattern from the existing drift tests; orval output is deterministic for pinned version + config + document).
- **[orval version/config churn changes generated code.]** → the orval config is committed and the version pinned; the drift check fails the build if the committed client doesn't match a fresh run.
- **[Trade-off:]** the centralized error encoder writes the `{"error": string}` envelope directly (it is not a handler, so "no ad-hoc writes in handlers" still holds); the cost is one hand-maintained encoder + pure mappers instead of generated per-op error types — accepted to keep a single shared domain→status table.

## Migration Plan

1. Regenerate `openapi.gen.go` with `types,chi-server,strict-server` and re-commit it (drift test updated to the new flag).
2. Introduce `*apiError` + the pure `mapXxxError` mappers + the chi `MaxBytesReader` middleware; rewrite the `httpx` middleware to use the shared envelope encoder.
3. Rewrite the 23 handlers to the strict signature (bind from `OpIDRequestObject`; return generated objects / `*apiError`). Keep the `toXxx` DTO converters.
4. Re-wire `server.go`: `var _ gen.StrictServerInterface = (*Server)(nil)`; `Routes()` = `HandlerWithOptions(NewStrictHandlerWithOptions(s, nil, StrictHTTPServerOptions{RequestErrorHandlerFunc, ResponseErrorHandlerFunc}), ChiServerOptions{BaseRouter, Middlewares:[UserMiddleware, MaxBodyMiddleware]})`.
5. Backend green (all existing handler/finance/household/import + e2e tests) before touching the UI.
6. Add orval (pinned) + config, generate the per-operation client, add the `ApiError` mutator; re-point `client.ts`/`hooks.ts`/`upload.ts`; adapt `schema.ts`.
7. Extend `Makefile` + Go drift/budget tests + UI drift/budget tests to cover the strict server gen and the client.
8. **Rollback:** the change is additive at the wire level (byte-identical contract) and confined to transport + generated code; reverting the commit restores the prior loose `openapi.gen.go`, handlers, and TS bridge. No data migration; nothing is persisted by this change.

## Open Questions

- **Exact orval version to pin** (any current stable, pinned in `package.json` + wiring). Decide at implementation; the design fixes the *shape* (fetch-mode per-op client + mutator), not the patch version.
- **Whether `openapi-typescript` (`paths.d.ts`) stays or is subsumed by orval's schemas.** Default: keep it (it is the documented source for `schema.ts` re-exports and is already drift-checked); orval additionally emits per-op functions. Revisit only if orval's generated schema types become the single source for `schema.ts`.
- **`UploadDocument` MIME source** — filename-derivation (`mime.TypeByExtension`) is the default (D2/risks). If a test reveals a case where the client-declared header is required, fall back to reading the part header via the raw `*multipart.Reader` *only* for that field (documented exception), which is the one place "hand-parsing" is unavoidable with the pinned `File` type.
