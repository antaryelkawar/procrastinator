# Proposal: invoice-warranty-asset-flow

## Why

Life Manager's core promise is turning raw life evidence (receipts, invoices, warranty cards) into queryable canonical data (`Source → extraction → canonical objects → relationships`). Today the repository has design documents but no running system. This change builds the first end-to-end vertical slice of the ingestion pipeline: upload an invoice or warranty document, let a BYO OpenAI-compatible LLM classify and extract it, resolve which real-world asset it describes, and create or update that Asset — proving the document-to-asset loop that later facets (maintenance, warranty expiry notifications, finance) will build on.

## What Changes

- New Go HTTP API (modular monolith, `net/http` + chi) exposing:
  - `POST /api/documents` — multipart document upload that runs the full ingestion flow synchronously and returns the resulting Asset as JSON.
  - `GET /api/assets` — list all assets.
  - `GET /api/assets/{id}` — fetch one asset.
  - `GET /api/assets/{id}/documents` — list documents linked to an asset.
- Minimal canonical persistence model in PostgreSQL (goose migrations, pgx v5):
  - **Source** — the retained uploaded file record (fidelity/provenance).
  - **Asset** — the canonical physical asset (brand, model, serial number, purchase date, price/currency, warranty start/end).
  - **Document** — the typed link (`invoice` | `warranty` | `other`) between a Source and an Asset, carrying the extracted structured fields.
- LLM extraction behind a provider boundary: OpenAI-compatible chat-completions client, configured entirely via environment variables (endpoint, model, API key), classifying the document type and extracting structured fields as JSON.
- Deterministic identity resolution: match an incoming extraction to an existing Asset by normalized serial number, else by normalized brand+model, else create a new Asset. Matching documents update the existing Asset (new non-empty values win; empty values never erase).
- Local filesystem storage for uploaded file bytes (S3-compatible object storage deferred).
- Test infrastructure: TDD with unit tests plus integration tests against a local Docker PostgreSQL; the LLM is mocked with a fake HTTP server returning canned extraction JSON; one opt-in, env-gated integration test exercises a real LLM endpoint.

## Capabilities

### New Capabilities

- `document-ingestion`: Multipart upload of invoice/warranty documents, retention of the raw file as a Source, synchronous processing, and the typed Document link between Source and Asset.
- `llm-extraction`: BYO OpenAI-compatible LLM client (env-configured endpoint/model/key), document type classification, structured field extraction, and failure handling.
- `asset-identity-resolution`: Deterministic matching of extracted document data to an existing Asset via serial number or brand+model identity, and the create-vs-update decision including field-merge semantics.
- `asset-registry`: The canonical Asset model with populated fields, uniqueness invariants, and the read APIs for assets and their documents.
- `test-infrastructure`: TDD workflow, Docker PostgreSQL integration tests, fake-LLM HTTP server mocking, and the env-gated real-LLM integration test.

### Modified Capabilities

(none — no existing specs in `openspec/specs/`)

## Impact

- **New code**: entire initial Go application — `cmd/` API server, `internal/` modules (ingestion, extraction/LLM client, identity resolution, asset registry, persistence), `migrations/` (goose SQL).
- **APIs introduced**: `POST /api/documents`, `GET /api/assets`, `GET /api/assets/{id}`, `GET /api/assets/{id}/documents` (HTTP/JSON). No breaking changes — no prior API exists.
- **Dependencies**: Go 1.27.x (latest stable), chi v5.3.2, pgx v5.10.0, goose v3.27.3, PostgreSQL 18 (Docker `postgres:18`) for tests and local runs. Versions verified against official release sources on 2026-08-23; final pins are set in the design phase.
- **Infrastructure**: local Docker PostgreSQL for development/testing; uploaded files on local filesystem; no UI, no auth, no async workers, no object storage in this slice (all deliberate deferrals per `docs/Life_Manager_Technical_Spec.md`).
- **Configuration**: new env vars for database connection, LLM endpoint/model/API key, storage directory, and the opt-in LLM integration test gate.
- **Alignment**: implements the design doc's ingestion pipeline (`Source → LLM interpretation → canonical data`), typed Source/Document/Asset relationship, raw-source preservation, and "LLMs are interpreters, not canonical truth" principles for the appliance/asset scenario (design doc stress test #3).
