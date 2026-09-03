# Spec: api-contract

New capability for change `openapi-codegen-pipeline`. Establishes a single OpenAPI 3.1 document as the source of truth for the HTTP API surface, with the server DTOs/route scaffolding, the client TypeScript SDK, the API documentation, and a reproducible drift-checked build all generated from it.

## ADDED Requirements

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
