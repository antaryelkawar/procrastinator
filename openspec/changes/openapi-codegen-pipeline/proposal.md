# Proposal: openapi-codegen-pipeline

## Why

The Procrastinator HTTP API is defined in three places that must be kept in sync by hand: the Go handler/DTO layer (`procrastinator-backend/api/*.go`), the hand-maintained TypeScript DTO interfaces (`ui/src/lib/api/types.ts`), and informal documentation. Every endpoint or field change is duplicated across all three, and drift is only caught (if at all) at runtime. This change makes a single OpenAPI 3.1 document the source of truth for the API surface and generates the server DTOs/route scaffolding, the client TypeScript SDK, and the API documentation from it — so a contract change is made once and propagated everywhere, with a build-time drift check catching stale generated code.

## What Changes

- **NEW** — A single committed OpenAPI 3.1 document becomes the authoritative definition of every route, operation, request/response schema, and error envelope for the HTTP API. It covers all existing endpoints (document upload, assets, asset documents, finance accounts, movements, import-batches, households) and the standard `{"error": string}` JSON error envelope.
- **NEW** — Server-side transport types (request/response DTOs) and endpoint scaffolding are generated from the document; the hand-written wire-DTO structs (`assetJSON`, `accountJSON`, `movementJSON`, `importBatchJSON`, `importLineJSON`, etc.) and their duplicate field lists are replaced by generated types that the handlers use.
- **NEW** — The UI consumes a TypeScript SDK (generated types + a typed fetch client) from the same document; the hand-authored `ui/src/lib/api/types.ts` DTO interfaces are replaced by the generated types.
- **NEW** — Build wiring (pinned generators + committed generated artifacts + a drift check) makes generation reproducible and fails the build/CI when committed generated code no longer matches the document.
- **NEW** — API documentation is generated from the document rather than hand-maintained.
- **RECONCILE** — The UI client currently special-cases `finance/*` routes with an `X-Tenant-ID` header while the server already serves *all* routes under `/api/users/{userId}` (path tenancy). The generated contract and client model path tenancy uniformly, removing the stale header special-case.
- **PRESERVED** — HTTP behavior, status codes, and wire field names are unchanged. The endpoints' observable contracts are preserved; they are simply expressed in one machine-readable document and generated from it.

## Out of Scope

- **No new API endpoints or behavioral changes.** This change alters only the mechanism for producing the transport layer (server DTOs, route scaffolding, client SDK) and documentation. The observable HTTP contract — routes, methods, status codes, request/response wire shapes, and error envelopes — is preserved unchanged.
- **No changes to server-side business logic, domain entities, or database schema.** Code generation targets the HTTP transport layer only; the domain model, persistence layer, and application services are untouched.
- **No authentication or authorization changes.** The existing path-based tenancy model (`/api/users/{userId}`) is preserved; no new auth mechanisms, token schemes, or middleware are introduced.
- **No UI component or page-level changes.** Only the API transport layer in the UI (replacing `types.ts` with generated types, removing the `finance/*` `X-Tenant-ID` header special-case in `client.ts`) is affected; React components, page routes, and UI behavior are untouched.
- **No real-time, WebSocket, or Server-Sent Events support.** The OpenAPI 3.1 document covers HTTP request/response only.
- **No multi-language SDK generation.** Only the Go server types and the TypeScript client SDK are generated; other language targets are out of scope.
- **No data migration or backfill.** No existing stored data is modified or migrated.

## Capabilities

### New Capabilities

- `api-contract`: The HTTP API surface is defined by a single OpenAPI 3.1 document (source of truth) from which the server DTOs/route scaffolding, the client TypeScript SDK, the API documentation, and a reproducible drift-checked build are all generated.

### Modified Capabilities

(none — existing spec-level behavior is unchanged; only the mechanism for producing the transport layer and docs changes)

## Impact

- **Code (backend)**: `procrastinator-backend/api/` gains a committed OpenAPI document and a generated-types package; the hand-written `*JSON` DTO structs and their `to*JSON` field lists are replaced by generated types (entity→DTO conversion remains, now targeting generated types); the chi router wiring aligns to the generated route/operation scaffolding.
- **Code (UI)**: `ui/src/lib/api/types.ts` is replaced by generated types; `client.ts` drops the `finance/*` `X-Tenant-ID` special-case in favor of uniform path tenancy.
- **Dependencies**: adds an OpenAPI toolchain — a Go generator (server types + route scaffolding), a TypeScript generator (client types/SDK), and a docs generator; versions are pinned in the build wiring.
- **Build/CI**: a codegen target (Makefile / npm script / `go:generate`) produces the artifacts; a drift check re-runs the generators and fails on mismatch; generated artifacts are committed.
- **Documentation**: replaces the informal per-endpoint reference with generated API reference derived from the same document.
- **Risk**: the generated client must reproduce the current wire conventions exactly (snake_case keys, `omitempty`→optional, exact-decimal string money, `link_conflicting` always present, `lines` null in list responses) or the UI breaks. The drift check plus the existing UI/backend tests guard this.

## Non-Functional Requirements

- **Codegen execution time:** A full run of all generators (Go server types, TypeScript client SDK, documentation) against the committed OpenAPI document SHALL complete in under **30 seconds** on a CI runner (2 vCPU, 4 GB RAM baseline).
- **OpenAPI document size:** The committed OpenAPI 3.1 document SHALL remain under **200 KB**. If the document exceeds this threshold, the change that caused the growth SHALL include a justification or a document split.
- **Drift-check CI overhead:** The drift-check step (re-run all generators, diff against committed artifacts) SHALL add no more than **30 seconds** to total CI pipeline duration.
- **Deterministic output:** Generated artifacts SHALL be byte-for-byte reproducible across consecutive runs with identical inputs (tool version, flags, document).
