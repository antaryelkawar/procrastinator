# Tasks: proper-multi-tenant-search

Implementation checklist. Ordered by dependency; each package is independently
implementable and names its file ownership. See `design.md` for the "how" and
`specs/` for the requirements.

**Constraint: OpenAPI-first.** API operations and schemas MUST be defined in
`api/openapi.yaml` first (Package 9), then regenerated via `make codegen`.
Manual changes to generated files (`api/gen/*`, `ui/src/lib/api/generated/*`)
or to handler signatures that deviate from the generated contract are not
acceptable.

## 1. Extraction confidence (DONE)

Adds `confidence` to the extraction pipeline: entity field, permissive parsing,
LLM prompt instruction, and their tests.

- [x] 1.1 `entity.Extraction.Confidence *float64` — `procrastinator-backend/commons/entity/extraction.go`
- [x] 1.2 Parse + validate `confidence` in `ParseExtraction` via `confidencePtr` (absent/non-numeric/out-of-range → nil, never error) — `procrastinator-backend/commons/parse/parse.go`
- [x] 1.3 Table-driven parse tests for confidence (valid, out-of-range, absent, co-existence) — `procrastinator-backend/commons/parse/parse_test.go`
- [x] 1.4 Instruct LLM to emit `confidence` in `SystemPrompt()` — `procrastinator-backend/infra/llm/prompt.go`
- [x] 1.5 Prompt test asserting `confidence` is requested — `procrastinator-backend/infra/llm/prompt_test.go`

## 2. Migration 00005: confidence columns + ingest_reviews data model

Single additive migration (`00005_confidence_reviews_search.sql`) adding
nullable `confidence real` to `assets` and `documents`, creating the
`ingest_reviews` table with RLS + membership trigger, plus the Go entity
fields, scan/map wiring, and `ReviewRepository` with factory integration.

- [x] 2.1 `Confidence *float64` on `entity.Asset` and `entity.Document` — `procrastinator-backend/commons/entity/asset.go`, `procrastinator-backend/commons/entity/document.go`
- [x] 2.2 Scan + map `confidence` column (scanAsset, scanDocument, assetToMap, documentToMap) — `procrastinator-backend/infra/postgres/scans.go`, `procrastinator-backend/infra/postgres/repos.go`
- [x] 2.3 `entity.IngestReview` + `IngestReviewState` constants — `procrastinator-backend/commons/entity/review.go`
- [x] 2.4 Migration `00005`: `ALTER TABLE assets ADD COLUMN confidence real;`, `ALTER TABLE documents ADD COLUMN confidence real;`, `CREATE TABLE ingest_reviews (…)`, composite index `(owner_id, state, created_at)`, RLS (ENABLE + FORCE + policy + trigger), `-- +goose Down` — `procrastinator-backend/migrations/00005_confidence_reviews_search.sql`
- [x] 2.5 `ReviewRepository` (embeds `*pgRepository[entity.IngestReview]`, shareable, filterConfig) + `NewReviewRepository` + `newReviewRepoForTx` — `procrastinator-backend/infra/postgres/review_repo.go`
- [x] 2.6 Wire `Reviews` into `repo.Factory` + `repo.Repos` + `postgres.NewFactory` (pool + tx-bound) — `procrastinator-backend/commons/repo/factory.go`, `procrastinator-backend/infra/postgres/factory.go`

## 3. Identity: Match + CommitCandidate + confidence stamping

Extends `core/identity/resolve.go` with non-committing `Match` and
`CommitCandidate` (reusing the existing merge/create logic), plus confidence
stamping on create and merge. All tests included.

- [x] 3.1 Extract shared `lookup` helper from `resolveBySerial`/`resolveByBrandModel` — `procrastinator-backend/core/identity/resolve.go`
- [x] 3.2 `Match(ctx, assets, ext, ownerHouseholdID) (entity.Asset, found bool, error)` — `procrastinator-backend/core/identity/resolve.go`
- [x] 3.3 `CommitCandidate(ctx, assets, ext, matchedAssetID *string, ownerHouseholdID) (entity.Asset, error)` — `procrastinator-backend/core/identity/resolve.go`
- [x] 3.4 Confidence stamping: `newAsset` sets `Confidence: ext.Confidence`; `mergeInto` sets only when non-nil — `procrastinator-backend/core/identity/resolve.go`
- [x] 3.5 Tests: Match (serial/brand+model hit/miss, no-identity, fence), CommitCandidate (merge/create/fallback), confidence stamping (create + merge nil-preserving) — `procrastinator-backend/core/identity/resolve_test.go`

## 4. Review service (confidence-review capability)

`core/review.Service` with `Hold`, `Approve` (atomic), `Reject`, `List`, `Get`
plus the `repo.Reviewer` interface. All tests included.

- [x] 4.1 `repo.Reviewer` interface + `repo.HoldInput` struct — `procrastinator-backend/commons/repo/review.go`
- [x] 4.2 `review.Service` + `New(factory)` + `Hold` (InTx: Match → create pending review) — `procrastinator-backend/core/review/service.go`
- [x] 4.3 `Approve(ctx, id)` — atomic InTx: load, non-pending→409, CommitCandidate, Documents.Create, transition — `procrastinator-backend/core/review/service.go`
- [x] 4.4 `Reject(ctx, id)` — InTx: load, non-pending→409, transition — `procrastinator-backend/core/review/service.go`
- [x] 4.5 `List(ctx, status)` + `Get(ctx, id)` (owner-scoped, ordered) — `procrastinator-backend/core/review/service.go`
- [x] 4.6 Tests: Hold (low/absent/no-identity/no-match), Approve (merge/create/409/404/rollback), Reject (pending/409/retained), List (default/filter/empty/cross-owner), Get (existing/404/cross-owner) — `procrastinator-backend/core/review/service_test.go`

## 5. Review threshold config

`PROCRASTINATOR_INGEST_REVIEW_THRESHOLD` environment variable: read, validate
(range `[0,1]`), fail-fast, default `0.7`. Includes tests and `.env.example`
documentation.

- [x] 5.1 `IngestReviewThreshold float64` on `Config` + env read + validation — `procrastinator-backend/config/config.go`
- [x] 5.2 Tests: unset→0.7, valid 0.9/0.0/1.0, invalid abc/1.5/-0.1 fail-fast — `procrastinator-backend/config/config_test.go`
- [x] 5.3 Document `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD` — `procrastinator-backend/.env.example`

## 6. Ingest confidence gate (document-ingestion capability)

Rewrites `ingest.Service.Process` to gate on `(confidence >= threshold)`:
commit (201 path) or hold (202 path). Includes the `ingest.Result` type and
all tests.

- [x] 6.1 `ingest.Result{Committed, Review}` + `New(factory, extractor, storage, maxBytes, threshold, reviewer)` — `procrastinator-backend/core/ingest/service.go`
- [x] 6.2 Rewrite `Process` → `(Result, error)`: gate commit/hold, `ErrNoIdentity` propagates both branches — `procrastinator-backend/core/ingest/service.go`
- [x] 6.3 Tests: high→Committed, low→Review, absent→Review (fail-safe), at-threshold→Committed, no-identity→ErrNoIdentity — `procrastinator-backend/core/ingest/service_test.go`

## 7. Search DB layer

`repo.SearchBackend` interface, all five per-type ILIKE search queries
(assets, accounts, movements, documents ⋈ sources, import batches), the
`searchBackend` adapter, factory wiring, and DB integration isolation tests.

- [x] 7.1 `repo.SearchBackend` interface (5 methods) — `procrastinator-backend/commons/repo/search.go`
- [x] 7.2 `SearchAssets`, `SearchAccounts`, `SearchMovements`, `SearchImportBatches` (scope.run + visibilityCond + ILIKE OR + ORDER BY) — `procrastinator-backend/infra/postgres/repos.go`, `procrastinator-backend/infra/postgres/finance.go`
- [x] 7.3 `SearchDocuments` (documents ⋈ sources ON source_id, sources.filename ILIKE) — `procrastinator-backend/infra/postgres/finance_queries.go`
- [x] 7.4 `searchBackend` adapter + `factory.Search` wiring — `procrastinator-backend/infra/postgres/factory.go`, `procrastinator-backend/commons/repo/factory.go`
- [x] 7.5 DB integration tests: cross-owner isolation, household member/non-member, unbound RLS backstop, document-join — `procrastinator-backend/infra/postgres/search_test.go`

## 8. Search core service

`core/search.Service`: param validation, ILIKE metachar escaping, combined
type-priority ordering (quick = prefix of paged), hit mapping with confidence.
All unit tests included.

- [x] 8.1 `search.Service` + `New(backend)`: q validation, escaping, combined order, Quick(top-N) + Paged(slice+total), hit mapping — `procrastinator-backend/core/search/service.go`
- [x] 8.2 Tests: escaping (`100%`, `_`, `\`), case-insensitive, per-type fields, no-match→empty, combined-order stability, quick=prefix-of-paged, limit/page bounds, hit shape (confidence present/absent) — `procrastinator-backend/core/search/service_test.go`

## 9. OpenAPI + codegen

**Must precede Package 10.** All new operations (search ×2, review ×4) and
schemas (`search_hit`, `search_quick_response`, `search_results_page`,
`ingest_review`, `approve_review_response`) plus `confidence` on existing
`asset`/`document` schemas are defined in `api/openapi.yaml` first, then
regenerated via `make codegen`. Drift tests must pass.

- [x] 9.1 Add operations + schemas to `api/openapi.yaml` (6 new ops, 5 new schemas, confidence on asset+document, error envelope on all non-2xx) — `procrastinator-backend/api/openapi.yaml`
- [x] 9.2 `make codegen` (regenerate Go + TS), confirm 6 new `ServerInterface` methods exist, `TestCodegenDrift` + TS drift test pass — `procrastinator-backend/api/gen/openapi.gen.go`, `ui/src/lib/api/generated/paths.d.ts`

## 10. HTTP layer: handlers + DTOs + composition root

All HTTP handlers (upload 201/202, search quick/paged, review CRUD), DTO
converters, composition root wiring, and full-stack API integration tests.
Depends on Package 9 (generated types) and Packages 4, 6, 8 (services).

- [x] 10.1 `UploadDocument` → map `ingest.Result` to 201 (asset) / 202 (review ref) — `procrastinator-backend/api/handlers.go`
- [x] 10.2 `QuickSearch` + `Search` handlers (param validation, delegate to core/search) — `procrastinator-backend/api/search.go`
- [x] 10.3 `ListReviews`, `GetReview`, `ApproveReview`, `RejectReview` handlers (delegate to core/review) — `procrastinator-backend/api/reviews.go`
- [x] 10.4 DTO converters: `toReview`, `toReviewRef`, `toSearchHit`, `toSearchQuickResponse`, `toSearchResultsPage`; extend `toAsset`/`toDocument` with confidence — `procrastinator-backend/api/dto.go`
- [x] 10.5 Composition root: extend `api.New` + `Server`, construct review/search services, pass threshold + reviewer into ingest, verify `*Server` satisfies `gen.ServerInterface` — `procrastinator-backend/api/server.go`, `procrastinator-backend/api/cmd/procrastinator/main.go`
- [x] 10.6 API integration tests (real chi + PG + fake LLM): upload 201/202/422/502, search quick/paged/400s/blank, review list/get/approve/reject/404/409, malformed/unregistered userId — `procrastinator-backend/api/handlers_test.go`, `procrastinator-backend/api/search_test.go`, `procrastinator-backend/api/reviews_test.go`

## 11. E2E + full verification

End-to-end scenarios over the real HTTP stack (confidence gate + review
lifecycle, search isolation) and the full test suite + codegen drift check.

- [x] 11.1 E2E: confidence gate + review lifecycle (low→202→list→approve→asset+doc; separate low→reject→no asset, source retained) — `procrastinator-backend/e2e/tenancy_e2e_test.go`
- [x] 11.2 E2E: search isolation (cross-owner, household member/non-member, unbound RLS) — `procrastinator-backend/e2e/tenancy_e2e_test.go`
- [x] 11.3 Full suite green: `go test ./...` (unit + DB integration) + `make codegen-drift-check` (Go + TS) — `procrastinator-backend` (all packages)

## 12. UI: search + review queue

React surfaces that consume the `search` and `confidence-review` contracts, layered
over the **existing OpenAPI-first data path**. Depends on **Package 9** (the
generated TS client + types already exist after `make codegen`) for compilation,
and on **Package 10** (the HTTP endpoints) for full verification. **OpenAPI-first
constraint:** generated files (`ui/src/lib/api/generated/*`) are codegen output and
are NEVER hand-edited; all new search/review API code is thin app wiring in
`schema.ts` / `client.ts` / `hooks.ts`. See design D8 for the "how" and
`specs/search-review-ui/` for the requirements.

- [x] 12.1 Re-export the new generated types in `ui/src/lib/api/schema.ts` (`SearchHit`, `SearchQuickResponse`, `SearchResultsPage`, `IngestReview`, `ApproveReviewResponse`; `confidence` is already on `Asset`/`Document`) — verify: `tsc -b` clean
- [x] 12.2 Thin typed wrappers in `ui/src/lib/api/client.ts` for the six new operations (`quickSearch`, `search`, `listReviews`, `getReview`, `approveReview`, `rejectReview`), unwrapping `{data,status,headers}` like the existing `listAssets`/`createAccount` wrappers — verify: `tsc -b` clean + `client.test.ts` green
- [x] 12.3 Query/mutation hooks in `ui/src/lib/api/hooks.ts`: `useQuickSearch`, `useSearch`, `useReviews`, `useReview`, `useApproveReview`, `useRejectReview`, with D4 keys (carry `uid`) + D5 invalidation (approve → `['reviews',uid]`,`['review',uid,id]`,`['assets',uid]`; reject → `['reviews',uid]`,`['review',uid,id]`) — verify: `hooks.test.tsx` green
- [x] 12.4 Confidence formatter in `ui/src/lib/format/` (absent → no indicator, never `0`) + unit test — verify: vitest green
- [x] 12.5 Quick-search typeahead `ui/src/components/search/quick-search.tsx` (250 ms debounce, ≤10 hits, title/subtitle/confidence, ArrowUp/Down+Enter+Escape, combobox/listbox ARIA) + mounted in `ui/src/components/layout/app-shell.tsx` (md+ and mobile top bar) — verify: `quick-search.test.tsx` (MSW + `vitest-axe`) green
- [x] 12.6 Search results page `ui/src/pages/search/search-results-page.tsx` at `/search` (paged, type indicator, pagination controls from `page`/`page_size`/`total`, endpoint order, empty/loading/error) + `ui/src/router.tsx` route — verify: `search-results-page.test.tsx` (MSW + `vitest-axe`) green
- [x] 12.7 Review queue `ui/src/pages/reviews/review-queue-page.tsx` at `/ingest/reviews` (default `pending`, status filter pending/approved/rejected, rows: doc_type / confidence / source filename / best-matched asset or "no match" / created; empty/loading/error) + `ui/src/router.tsx` route — verify: `review-queue-page.test.tsx` (MSW + `vitest-axe`) green
- [x] 12.8 Approve/reject from the queue: per-row actions behind `ui/src/components/confirm-dialog.tsx` → `useApproveReview`/`useRejectReview`, success toast (`ui/src/components/ui/sonner.tsx`), 409/404 → error (no success) + queue refresh; approve success links to the resulting asset — verify: approve/reject tests (MSW) green
- [x] 12.9 Upload `202` held-for-review: `ui/src/lib/api/upload.ts` + `useUploadDocument` resolve the union `{kind:'committed',asset} | {kind:'held',review}`; `ui/src/pages/upload/upload-page.tsx` shows the held state + link to `/ingest/reviews`; `422`/`502` unchanged — verify: `upload.test.ts` green
- [x] 12.10 Nav: add Search + "Review queue" to `NAV_ITEMS` in `ui/src/components/layout/app-shell.tsx` (sidebar + mobile) — verify: `app-shell.test.tsx` green
- [x] 12.11 UI verification: `npm run test` (vitest) green in `ui/` + `tsc -b` clean + `make codegen-drift-check` (proves no hand-edited generated files)
