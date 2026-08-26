# Proposal: statement-ledger-ingestion

## Why

Life Manager's finance facet is deliberately unimplemented: the design docs require a minimal financial ontology *derived from existing research* before any finance code ("Finance should derive current balances from underlying movements"; "a dedicated finance modeling pass is required before finance implementation" — Design §28/§34; Research Backlog §2). Today the backend has only the asset/document pipeline from `invoice-warranty-asset-flow` — an invoice's price and currency are captured but connect to nothing, because there is no ledger, no account, and no money movement to link them to.

Real life produces bank statements, wallet statements, and receipts that describe the same money from different angles. Without a canonical ledger and a disciplined import path, every such document stays an isolated fact. This is a solved shape in the research corpus — Firefly III (accounts, withdrawal/deposit/transfer movements, external-ID dedup), Maybe/Sure (account-centric model), LifeStack (deterministic import batches with source metadata and rollback hooks), Paperless-ngx (preview/classify before persist) — so this change reuses those proven patterns rather than inventing a ledger.

## What Changes

- **New capability `financial-ledger`**: the minimal canonical finance ontology distilled from the research — Financial Account (controlled types: bank, wallet, cash, credit card), Money Movement (expense / income / transfer between owned accounts or external counterparties), exact-decimal money semantics, derived balances that are never independently editable, manual movement entry with an immutable-ledger escape hatch, and a reversible, conflict-preserving link between a money movement and an already-captured document (e.g., a receipt ingested earlier via `document-ingestion`).
- **New capability `statement-import`**: the Import Batch lifecycle for bank/wallet statement files (CSV and text-based PDF) — upload with source retention, parse, preview with per-line validation, deterministic duplicate detection (external reference IDs + content fingerprints), explicit atomic commit, idempotent re-commit/re-import, batch discard, and deterministic auto-linking of committed movements to existing captured documents.
- No existing capability requirements change. Statement ingestion uses dedicated `/api/finance/*` endpoints and does not alter the `document-ingestion` asset pipeline; documents ingested there become *link targets*, never automatic movements.

**Explicitly out of scope for this change**: UI screens, reconciliation UX (statement-balance vs derived-balance workflows), net worth, investments, loans, FX / multi-currency movements, budgets, goals. Also deferred to later changes (consistent with Design §28 and Research Backlog §2): categories/allocations, debts/receivables and settlements, OCR of scanned/image-only statement PDFs, connector-based auto-fetch imports, rollback of committed batches, and reversal/correction of committed imported movements.

## Capabilities

### New Capabilities

- `financial-ledger`: canonical financial accounts, money movements (expense/income/transfer), exact money representation, derived balances, movement provenance, movement–document linkage with conflict retention, ledger immutability rules, and tenant scoping of all ledger data.
- `statement-import`: statement file (CSV/PDF) ingestion as an Import Batch — source retention, format parsing, per-line validation, external references, deterministic duplicate/possible-duplicate classification, preview without canonical writes, atomic idempotent commit, batch lifecycle states, auto-linking to captured documents, and tenant scoping of import data.

### Modified Capabilities

(none)

Rationale: tenant-scoping rules for the new tables are specified *inside* the two new capabilities rather than as a `multitenancy` delta, because the in-flight `invoice-warranty-asset-flow` change already modifies `multitenancy` and `backend-platform`; a parallel edit of those deltas would create a merge conflict between the two changes. The new capabilities state the same fail-closed tenant rules for their own tables.

## Impact

- **Code**: new domain services under `core/` (ledger, statement import) following the `backend-platform` clean-architecture layering; new entities and boundary interfaces in `commons/`; new persistence in `infra/postgres`; new handlers under `api/`. The change follows the conventions landing in `invoice-warranty-asset-flow` (flat `api`/`core`/`infra`/`commons` roots, generic `repo.Repository[T]` with functional options, explicit `repo.Tenant` at call sites, `TxFactory` transactions) but touches none of that change's files.
- **APIs**: additive only — `POST/GET /api/finance/accounts`, `GET /api/finance/accounts/{id}` (with derived balance), `POST/GET /api/finance/movements`, `GET /api/finance/movements/{id}`, `PATCH /api/finance/movements/{id}` (description correction), `DELETE /api/finance/movements/{id}` (manual movements only), `POST /api/finance/movements/{id}/link`, `DELETE /api/finance/movements/{id}/link`, `POST/GET /api/finance/import-batches`, `GET /api/finance/import-batches/{id}`, `POST /api/finance/import-batches/{id}/commit`, `POST /api/finance/import-batches/{id}/discard`. Existing endpoints (`/api/documents`, `/api/assets`) are unchanged.
- **Database**: one new versioned goose migration adding tenant-scoped finance tables (accounts, movements, import batches, import lines). Existing tables untouched. Note: the PoC schema's per-tenant account-name unique constraint (`uq_account_tenant_name` in `docs/Life_Manager_DB_Schema.md`) is intentionally dropped — the `financial-ledger` spec explicitly allows duplicate account names; likewise the PoC `movement_type` vocabulary (`debit`/`credit`/`transfer`) maps to the spec's `expense`/`income`/`transfer` at design time.
- **Dependencies**: none mandated by this spec beyond existing conventions (exact decimal arithmetic per the established money-representation rule; CSV/PDF parsing library selection is a design-phase decision).
- **Coordination**: builds on `document-ingestion` (Source retention semantics; Document records as link targets standing in for a future canonical Purchase), `asset-registry` (money representation convention), and `multitenancy` (`X-Tenant-ID` header, fail-closed scoping). Linkage semantics (dedup, conflict retention, reversible typed links) are defined so the link target can later migrate from Document to a canonical Purchase without behavioral breakage.
