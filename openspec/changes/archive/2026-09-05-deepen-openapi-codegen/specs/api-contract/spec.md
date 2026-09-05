# Spec: api-contract (delta)

Deepens the `api-contract` capability established by `openapi-codegen-pipeline`. The server binds documented requests and encodes documented responses through the generated machinery (no hand-parsed inputs, no ad-hoc response writes), and the UI consumes a per-operation typed client (no manual `apiJson` path strings). The deepened pipeline remains reproducible and within its time/size budgets. These are ADDED requirements: the base capability spec is the archived `openapi-codegen-pipeline` delta (not yet folded into `openspec/specs/api-contract/`), so this change contributes its requirements as additions rather than in-place edits.

## ADDED Requirements

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
