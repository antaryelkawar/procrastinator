# Spec: api-contract

Single OpenAPI 3.1 document as the source of truth for the HTTP API surface: server code, client SDK, documentation, and drift-checked build are all generated from it. Consolidated from the `openapi-codegen-pipeline` and `deepen-openapi-codegen` changes.

## Purpose

Establish a single OpenAPI 3.1 document (`procrastinator-backend/api/openapi.yaml`) as the sole authoritative definition of the HTTP API surface. Server-side code (types, strict request/response binding, route scaffolding), client-side TypeScript SDK (per-operation typed client), and API documentation are all generated from it. The pipeline is reproducible, pinned, drift-checked, and within performance/size budgets.
## Requirements
### Requirement: Single source-of-truth OpenAPI contract

The HTTP API surface SHALL be defined by a single committed OpenAPI 3.1 document that is the sole authoritative definition of every route, operation, request/response schema, and error envelope. The document SHALL be valid OpenAPI 3.1. Every route registered by the backend HTTP router SHALL correspond to exactly one operation in the document, and every operation in the document SHALL correspond to exactly one registered route; there SHALL be no route without an operation and no operation without a route. The document SHALL define the standard JSON error envelope (`{"error": string}`) for every non-2xx response it documents.

#### Scenario: Document validates as OpenAPI 3.1

- **WHEN** the committed OpenAPI document is validated against the OpenAPI 3.1 schema
- **THEN** validation passes with no errors

#### Scenario: Routes and operations match exactly

- **WHEN** the set of (HTTP method, path-template) pairs registered by the backend router is compared against the set of operations in the document
- **THEN** the two sets are identical (every registered route has an operation, and every operation is a registered route)

#### Scenario: Existing endpoint families are all covered

- **WHEN** the document is inspected for the current endpoint families (document upload, assets, asset documents, finance accounts, finance movements, finance import-batches, households)
- **THEN** every current route appears as a documented operation

#### Scenario: Error envelope is documented

- **WHEN** a non-2xx operation (for example a 404 or 400 response) is inspected
- **THEN** its response schema is the `{"error": string}` envelope

### Requirement: Server-side code generation

The backend's HTTP transport types (request and response DTOs) and endpoint scaffolding SHALL be generated from the OpenAPI document rather than hand-authored. The generated Go types SHALL be committed to the repository and SHALL be the types the HTTP handlers encode and decode. Hand-written wire-DTO structs that duplicate a documented request or response schema SHALL be removed. The generated route/operation scaffolding SHALL be derived from the document's operations such that the handler surface reflects the document without per-endpoint manual signature maintenance.

#### Scenario: Generated types match the wire contract

- **WHEN** the server code generator is run against the document
- **THEN** a Go type is produced for every documented schema (asset, document, account, movement, import-batch, import-line, import-source, commit-summary, and each request body) whose field names, JSON keys, and optionality match the documented wire shapes (snake_case keys, `omitempty` fields optional, exact-decimal strings for money)

#### Scenario: Handlers use the generated types

- **WHEN** the backend compiles
- **THEN** handlers encode/decode the generated types and no hand-written wire-DTO struct duplicates a documented response or request schema

#### Scenario: New operation yields handler scaffolding

- **WHEN** a new operation is added to the document and the server generator is re-run
- **THEN** the generated route/operation scaffolding includes the new operation's signature and no hand-written handler-signature edit is required to reflect it

### Requirement: Client-side TypeScript SDK

The UI client SHALL consume a TypeScript SDK — the generated response/request types and a typed fetch client — produced from the same OpenAPI document. The hand-authored DTO interfaces (`ui/src/lib/api/types.ts`) SHALL be replaced by the generated types so that client and server share one contract. Client calls to documented endpoints SHALL be typed against the generated types rather than untyped or `any` payloads.

#### Scenario: Generated TS types match the wire contract

- **WHEN** the TypeScript generator is run against the document
- **THEN** a TypeScript type is produced for every documented response and request schema whose property names and optionality match the wire contract (snake_case, nullable/`omitempty` fields optional, exact-decimal string money, `link_conflicting` always present, `lines` nullable in list responses)

#### Scenario: Client is typed against the SDK

- **WHEN** the UI builds
- **THEN** client calls to documented endpoints are typed against the generated types (no `any`/untyped response body for a documented endpoint)

#### Scenario: Contract field flows to the client

- **WHEN** a field is added to a response schema in the document and the TypeScript generator is re-run
- **THEN** the generated TS type includes the field and the client compiles against it

### Requirement: Reproducible generation and drift detection

Code generation SHALL be reproducible and wired into the build so generated artifacts are deterministic and stay in sync with the document. Generator invocations SHALL be pinned (tool, version, and flags) in the build wiring. Generated artifacts SHALL be committed to version control. A drift check SHALL re-run the generators against the current document and fail the build/CI when the committed generated artifacts do not match the freshly generated output.

#### Scenario: Generation is deterministic

- **WHEN** the generators are run twice against the same document
- **THEN** the produced artifacts are byte-for-byte identical

#### Scenario: Stale generated code fails the build

- **WHEN** the document is edited but the generators are not re-run
- **THEN** the drift check fails with a message identifying the stale generated artifact(s)

#### Scenario: Re-running codegen clears the drift

- **WHEN** the generators are re-run after a document edit and the output is committed
- **THEN** the drift check passes

#### Scenario: Fresh clone builds from committed artifacts

- **WHEN** a clean clone is built
- **THEN** the build succeeds using the committed generated artifacts and the drift check reports them in sync with the document

### Requirement: Documentation generated from the contract

API documentation SHALL be generated from the OpenAPI document rather than hand-maintained, and SHALL be reproducible from the same document. The generated documentation SHALL cover every documented operation (method, path, parameters, request/response schemas, and error responses) and SHALL reflect the current document.

#### Scenario: Docs cover all operations

- **WHEN** the documentation generator is run against the document
- **THEN** the output documents every operation with its method, path, parameters, request/response schemas, and error responses

#### Scenario: Docs track document changes

- **WHEN** the document changes (a new operation or a schema field) and the documentation is regenerated
- **THEN** the regenerated documentation reflects the added operation/field

#### Scenario: Doc generation is idempotent

- **WHEN** the documentation generator is run twice against the same document
- **THEN** the output is identical

### Requirement: Codegen performance and size budgets

The codegen pipeline SHALL stay within defined performance and size budgets so that generation remains fast enough for local development and CI. A full run of all generators (server Go types, client TypeScript SDK, documentation) SHALL complete within **30 seconds** on a 2 vCPU / 4 GB CI runner. The committed OpenAPI 3.1 document SHALL remain under **200 KB** in file size. The drift-check step (re-run generators + diff against committed artifacts) SHALL add no more than **30 seconds** to total CI pipeline duration.

#### Scenario: Full codegen run is within time budget

- **WHEN** all generators (Go server types, TypeScript client SDK, documentation) are run against the committed OpenAPI document on a 2 vCPU / 4 GB CI runner
- **THEN** the total elapsed time is under 30 seconds

#### Scenario: OpenAPI document is within size budget

- **WHEN** the committed OpenAPI 3.1 document is measured
- **THEN** its file size is under 200 KB

#### Scenario: Drift check is within CI time budget

- **WHEN** the drift-check step (re-run all generators, diff against committed artifacts) is executed in CI
- **THEN** the step completes within 30 seconds

### Requirement: Server request binding from the contract

Every documented operation's request inputs — path parameters, query parameters, and the request body (JSON or `multipart/form-data`) — SHALL be provided to the handler as the Go types generated from that operation's parameters and request-body schema in `openapi.yaml`. Handler implementations SHALL consume those generated request types and SHALL NOT hand-parse documented request inputs: they SHALL NOT read named multipart form fields, query parameters, or the raw JSON body directly (the generated bind/parse mechanism replaces direct `r.FormFile` / `r.FormValue` / `r.ParseMultipartForm` / `json.NewDecoder(r.Body)` / `r.URL.Query()` reads for documented operations). The observable request behavior is preserved: valid inputs produce the same outcomes as before, and missing or malformed inputs produce the same documented non-2xx status codes with the `{"error": string}` envelope.

#### Scenario: Multipart upload binds to the generated body type

- **WHEN** a valid `multipart/form-data` upload carrying the required `file` part and an `owner_household_id` is POSTed to the document upload operation
- **THEN** the handler receives the generated multipart body with the file present and `owner_household_id` set, and returns `201` with the created asset

#### Scenario: Missing required file yields 400

- **WHEN** a `multipart/form-data` upload without the required `file` part is POSTed to a multipart operation
- **THEN** the response is `400` with the `{"error": string}` envelope (not `500`)

#### Scenario: JSON body is bound to the generated type

- **WHEN** a valid JSON request body matching the documented request schema is POSTed to a JSON-body operation
- **THEN** the handler consumes the generated request type and returns the documented success status

#### Scenario: Malformed JSON body yields 400

- **WHEN** an invalid (non-JSON or schema-mismatched) body is POSTed to a JSON-body operation
- **THEN** the response is `400` with the `{"error": string}` envelope

#### Scenario: Query parameters are bound to the generated params type

- **WHEN** the movements list operation is called with `account_id`, `from`, and `to` query parameters
- **THEN** the handler receives the generated params struct populated with those values and returns the same filtered results as before

#### Scenario: Handlers consume generated request types with unchanged outputs

- **WHEN** a documented operation is invoked with valid inputs (a JSON body, a multipart upload, or query parameters)
- **THEN** the handler's signature accepts the operation's generated request type for those inputs, and the operation returns the same status code and response body it returned before the change (the handler consumes the generated bind/parse output, not hand-parsed reads)

### Requirement: Server response encoding via the generated encoder

All success and error responses for documented operations SHALL be emitted through a single centralized encoder path wired via `HandlerWithOptions` — a generated `ResponseEncoder` for success bodies and an `ErrorEncoder` / error handler for the `{"error": string}` envelope — rather than ad-hoc response writes at each handler call site. Handlers SHALL return the generated response object (or an error) for the encoder to emit and SHALL NOT write response bytes directly for documented operations (no direct `httpx.WriteJSON` / `httpx.WriteError` / raw `w.Write` / `w.WriteHeader` for response emission). The wire format and the domain-error→status mapping are preserved: success bodies are the generated types serialized to JSON with the documented status and `Content-Type: application/json`, non-2xx bodies are the `{"error": string}` envelope, `204` responses carry no body, and the same domain errors map to the same documented status codes as before.

#### Scenario: Success body emitted by the encoder

- **WHEN** a list operation succeeds
- **THEN** the response body is the JSON of the generated resource type with the documented status and `Content-Type: application/json`, produced by the encoder rather than a handler-local write

#### Scenario: Error envelope emitted by the error encoder

- **WHEN** a get-by-id operation finds no visible resource
- **THEN** the response is `404` with the `{"error": string}` envelope, produced by the centralized error path

#### Scenario: Domain error mapping is preserved

- **WHEN** the ingest service reports an "exceeds size limit" error on an upload
- **THEN** the response is `413` with the `{"error": string}` envelope (the same mapping as before the change)

#### Scenario: No-content operation returns 204 with no body

- **WHEN** a delete or unlink operation succeeds
- **THEN** the response is `204` with an empty body

#### Scenario: Generated response objects produce the documented wire output

- **WHEN** a documented operation's handler returns a generated success or error response object
- **THEN** the centralized encoder emits the documented status, `Content-Type: application/json`, and body, and the resulting wire output (status, headers, body) is byte-identical to the pre-change output for the same inputs

#### Scenario: Wire parity before and after

- **WHEN** the same set of requests is issued against the API before and after the change
- **THEN** the status codes and response bodies are unchanged

### Requirement: Client per-operation typed SDK

The UI SHALL consume a per-operation typed client generated from the same `openapi.yaml` (orval or an equivalent per-operation client generator), so each documented `operationId` maps to a typed client function whose request and response are typed against that operation's generated schemas. The manual `apiJson` / `apiVoid` path-string calls and the hand-built resource→path string bridge in `ui/src/lib/api` (`client.ts`, `hooks.ts`, `upload.ts`) SHALL be replaced by the per-operation typed client for documented endpoints. Request bodies and responses SHALL be typed against the generated per-operation types (no `any`, no untyped bodies, no hand-constructed path strings for documented endpoints), and observable client behavior is preserved: the same endpoints and methods, the same `ApiError` error-envelope handling, the same TanStack Query keys and invalidation, and multipart uploads still report progress.

#### Scenario: Per-operation typed function per documented operation

- **WHEN** the per-operation client generator is run against the document
- **THEN** each documented `operationId` yields a typed client function whose request and response types match the operation's generated schemas

#### Scenario: Read calls route through the typed client

- **WHEN** the UI lists assets
- **THEN** it calls the generated list-assets operation (typed) rather than a manual `apiJson('assets')` path string, and the result is typed as the generated asset array

#### Scenario: Write calls use a typed request body

- **WHEN** the UI creates a movement
- **THEN** it calls the generated create-movement operation with a request body typed against the generated request schema (no `any`, no hand-built path)

#### Scenario: Multipart upload is typed and reports progress

- **WHEN** the UI uploads a document or statement
- **THEN** it uses the generated upload operation (typed multipart) and still reports upload progress

#### Scenario: UI calls route through typed per-operation functions

- **WHEN** the UI performs a documented read or write (e.g., lists assets, creates a movement, uploads a document)
- **THEN** it calls the generated per-operation typed function for that `operationId`, and the emitted HTTP request (method, path, body) and parsed response are identical to the pre-change behavior, with request/response typed against the generated schemas (no untyped bodies, no hand-constructed path strings)

#### Scenario: Contract field flows to the client

- **WHEN** a field is added to a response schema in the document and the per-operation client is regenerated
- **THEN** the generated per-operation response type includes the field and the UI compiles against it

#### Scenario: Error handling preserved

- **WHEN** a documented endpoint returns a non-2xx response
- **THEN** the client surfaces the same `ApiError` carrying the envelope's `error` detail as before

#### Scenario: Network failure surfaces the same error

- **WHEN** a documented endpoint call fails at the network layer (the request times out or no response is received)
- **THEN** the client surfaces an `ApiError` with the same network-failure status (`0`) and copy as before the change, so retry and error handling are unchanged

### Requirement: Deepened codegen reproducibility and budgets

The deepened codegen pipeline SHALL remain reproducible and within its time and size budgets, now covering the new/changed generators. The strict-mode server generation and the per-operation client generation SHALL be pinned (tool, version, flags) in the build wiring, their committed artifacts SHALL be version-controlled, and the drift check SHALL re-run all generators against the document and fail the build when a committed generated artifact (the regenerated server code or the regenerated per-operation client) does not match the fresh output. A full run of all generators SHALL complete in under 30 seconds on a 2 vCPU / 4 GB CI runner, the drift-check step SHALL add no more than 30 seconds, and the committed `openapi.yaml` SHALL remain under 200 KB.

#### Scenario: Deepened generation is deterministic

- **WHEN** the new or changed generators are run twice against the same document
- **THEN** the produced artifacts are byte-for-byte identical

#### Scenario: Stale per-operation client fails the drift check

- **WHEN** the committed per-operation client is edited (or the document is edited) but the generators are not re-run
- **THEN** the drift check fails, naming the stale generated artifact

#### Scenario: Re-running codegen clears drift

- **WHEN** the generators are re-run after a document edit and the output is committed
- **THEN** the drift check passes for the server and per-operation client artifacts

#### Scenario: Full deepened codegen is within the time budget

- **WHEN** all generators (strict server, per-operation client, TS types, docs) are run against the committed document on a 2 vCPU / 4 GB CI runner
- **THEN** the total elapsed time is under 30 seconds

#### Scenario: OpenAPI document is within the size budget

- **WHEN** the committed `openapi.yaml` is measured
- **THEN** its file size is under 200 KB

### Requirement: Contract declares the Basic Auth security scheme

The OpenAPI document SHALL declare a `basicAuth` security scheme of type `http` with scheme `basic`, and SHALL apply it globally to the API (or to every documented operation). The scheme declaration SHALL be visible in generated client and documentation artifacts so that generated code and docs reflect that the API requires Basic Auth. The document's existing error-envelope invariant for non-2xx responses is unchanged and extends to the newly documented `401` responses (see the api-basic-auth delta): they SHALL also use the `{"error": string}` envelope. Non-API transport-level responses (nothing else) violate this only insofar as they are documented — there SHALL be no separately documented empty-body 401.

#### Scenario: Security scheme present in the document

- **WHEN** the committed `openapi.yaml` is inspected
- **THEN** it contains a `basicAuth` `http` security scheme with scheme `basic` applied to the documented operations

