# Proposal: invoice-warranty-asset-flow (redesign)

## Why

The first vertical slice (document upload → LLM extraction → canonical Asset) is implemented and archived, but review of real AMC/invoice samples (LG, IFB) exposed structural gaps: no multitenancy (a hard requirement — no cross-tenant data leakage), an Asset model too rigid for real documents (no place for AMC card numbers, Indian tax breakups, ICR numbers), an LLM integration that wastes tokens on `<thought>` blocks, a codebase that still carries the old `life-manager` naming, and a flat `internal/` layout with a duplicated SQL adapter (`txAssetStore`) instead of clean repository/service boundaries. This change is a full redesign of the same feature on the committed code.

## What Changes

- **BREAKING** — Rename all `life-manager`/`lifemanager`/`LM_*` identifiers: binary `procrastinator-api`, env prefix `PROCRASTINATOR_*`, database `procrastinator`, config struct `Config`.
- **BREAKING** — Restructure `procrastinator-backend/` from `cmd/`+`internal/` into clean-architecture packages: `commons-data` (entities, boundary interfaces, pure helpers), `commons-server` (config, HTTP/middleware utilities), `procrastinator-core` (identity resolution, ingest orchestration), `procrastinator-infra` (PostgreSQL repos, LLM client, file storage), `procrastinator-api` (HTTP handlers + binary). Single Go module retained.
- **BREAKING** — Introduce repository/service interfaces in `commons-data` with compile-time interface guards (`var _ Iface = (*impl)(nil)`) in `procrastinator-infra`; a `RepoFactory`/`InTransaction` abstraction replaces the hand-duplicated `txAssetStore` SQL.
- **BREAKING** — Multitenancy everywhere: `tenant_id` on every table, `X-Tenant-ID` header on every API request, tenant carried in `context.Context` by middleware, every repository query tenant-scoped, tenant-scoped serial uniqueness.
- **BREAKING** — Asset model: structured core (brand, model, serial_number, purchase_date, warranty_end, price, currency, doc_type now including `amc`) **plus** a generic `metadata` JSONB bag the LLM populates with unstructured fields (AMC card no, tax breakup, ICR no, customer details, …). `warranty_start` leaves the typed core (available via metadata such as `amc_start`/`warranty_start`).
- **BREAKING** — New LLM extraction prompt: strict-JSON/no-thinking system prompt (measured to eliminate `<thought>` blocks), document-type-agnostic classify-then-extract flow, snake_case metadata key guidance with examples, date normalization (DD-MMM-YY, DD.MM.YYYY, DD-MMM-YYYY → ISO), Indian tax (CGST/SGST/IGST/CESS) and currency extraction. `<thought>` stripping retained as parser-level safety net.
- Test conventions unchanged (table-driven, `t.Parallel`, no env in tests, `<prod>_test.go`); the gated real-LLM test now exercises the two real sample PDFs in `sample-data/`.

## Capabilities

### New Capabilities

- `multitenancy`: Tenant identification via `X-Tenant-ID` header, tenant in request context, tenant-scoped persistence and queries, cross-tenant isolation, test-tenant conventions.
- `backend-platform`: Clean-architecture layering and dependency rules, interface definitions with compile-time guards, `procrastinator-*` naming (binary, env vars, database), single-module directory layout.

### Modified Capabilities

- `asset-registry`: Asset gains `doc_type` (incl. `amc`) and `metadata` JSONB; typed core drops `warranty_start`; all invariants become tenant-scoped.
- `asset-identity-resolution`: Matching and uniqueness become tenant-scoped; merge gains metadata shallow-overlay and doc_type last-write-wins semantics.
- `document-ingestion`: Upload requires a tenant header; Source/Document rows carry `tenant_id`; document types include `amc`; `extracted_fields` carries metadata.
- `llm-extraction`: Env vars renamed `PROCRASTINATOR_*`; classification adds `amc`; extraction produces a typed core **and** an open-ended metadata object; no-thinking system prompt with `<thought>` stripping as safety net.
- `test-infrastructure`: Test env vars renamed; gated real-LLM test uses the real `sample-data/` PDFs.

## Impact

- **Code**: near-total rewrite of `procrastinator-backend/` — old `cmd/` and `internal/` trees deleted; five new package roots; new schema in `migrations/00001_init.sql` (dev database is disposable, no data migration).
- **APIs**: same four endpoints, but every request now requires `X-Tenant-ID` (missing → 400); Asset JSON gains `doc_type` + `metadata`, loses `warranty_start`; Document types include `amc`.
- **Dependencies**: unchanged (Go 1.27, chi v5.3.2, pgx v5.10.0, goose v3.27.3, PostgreSQL 18); LLM still `gemma-4-26b-a4b-it` via the OpenAI-compatible Gemini endpoint.
- **Infrastructure**: `docker-compose.yml` database renamed `procrastinator`; `.env`/`.env.example` renamed to `PROCRASTINATOR_*`.
