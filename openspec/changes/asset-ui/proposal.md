# Proposal: asset-ui — Web UI for the Procrastinator personal finance app

## Why

The Procrastinator backend is implemented and verified (document ingestion, asset registry, financial ledger, statement import, multi-tenancy), but it is only reachable via raw HTTP. A user cannot upload a receipt, see their assets, record a movement, or commit a statement import without crafting API calls by hand. This change adds the missing web frontend so the app is usable end-to-end by a non-technical user, covering every user-facing backend capability.

## What Changes

- Add a new single-page web application (`ui/`), built with the stack fixed by prior research (`.tmp/websearcher-ui-stack-research-20260825.md`): **Vite (react-ts) + Tailwind CSS v4 + shadcn/ui + React Router + TanStack Query + react-dropzone + XHR upload progress**. One responsive codebase serves desktop and mobile browsers.
- The SPA consumes the existing REST API under `/api/users/{userId}/…` (per `multitenancy`); the active user is explicit in the UI and every request is scoped to it.
- New screens:
  1. **Document upload** — drag-and-drop multi-file upload with per-file progress and per-file success/error results (`POST /api/documents`).
  2. **Asset list** — all assets with their key fields and document-type badges (`GET /api/assets`).
  3. **Asset detail** — one asset's extracted fields plus its linked documents with source filenames and upload timestamps (`GET /api/assets/{id}`, `GET /api/assets/{id}/documents`).
  4. **Finance accounts** — account list with derived balances and an account-creation flow (`GET/POST /api/finance/accounts`).
  5. **Money movements** — movement list with account/date filters; create manual expense/income/transfer; correct descriptions; delete manual movements; link/unlink a movement to a document; surface provenance (`manual` vs `import`) and link conflicts (`/api/finance/movements…`).
  6. **Statement import** — upload a statement for an account, review per-line preview statuses (`valid` / `duplicate` / `possible-duplicate` / `error`), commit or discard the batch, and browse import history (`/api/finance/import-batches…`).
- Development wiring: Vite dev-server proxy to the Go backend; production: `dist/` build servable as static assets. No backend API changes.

## Capabilities

### New Capabilities

- `asset-ui`: the responsive web frontend — application shell and routing, active-user context, API data layer, and the six screens above with their error, loading, and empty states.

### Modified Capabilities

(none — the backend API surface is unchanged)

## Impact

- **Code**: new `ui/` workspace at the repo root (Vite + React + TypeScript); no changes under `procrastinator-backend/`.
- **APIs**: consumed as specified in `openspec/specs/` (document-ingestion, asset-registry, financial-ledger, statement-import, multitenancy); no additions or modifications.
- **Dependencies**: new npm dependencies only (React 18+/19, Vite 8, Tailwind v4, shadcn/ui components, react-router-dom, @tanstack/react-query, react-dropzone). Node 20.19+/22.12+ required for local dev.
- **Infrastructure**: unchanged. Dev proxy targets `http://localhost:8080`; production build is a static `dist/` directory deployable from any static host or alongside the backend.
- **Out of scope**: authentication/authorization (single-user selection per multitenancy's explicit-provisioning model), document file download/preview of raw bytes (no backend endpoint exists), OCR of scanned PDFs, household-scope UI (backend scope dimension is not exposed via API), import batch rollback.
