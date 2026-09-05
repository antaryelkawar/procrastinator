# Design: proper-multi-tenant-search

Technical design for HOW to implement the broadened intelligent multi-tenant
ingestion/search surface: **search** (new capability), **confidence-review**
(new capability), and the **llm-extraction** / **document-ingestion** deltas
(confidence carried by extraction; upload response gated by confidence).
Requirements live in `specs/`; motivation lives in `proposal.md`. This document
explains the architecture and the "why" behind each decision, grounded in the
existing backend (module `procrastinator-backend`).

## Context

The Go backend already implements the pieces this change composes:

- **Path tenancy** — every route lives under `/api/users/{userId}` and is
  resolved by `httpx.UserMiddleware` (`api/httpx/user.go`): malformed id → `400`,
  unregistered id → `404`, success → `user.WithUser(ctx, id)`. Routes are NOT
  hand-registered; the generated `gen.HandlerWithOptions` derives them from
  `api/openapi.yaml`, and every handler implements a method on the generated
  `gen.ServerInterface` (`api/gen/openapi.gen.go`). The 1:1 routes↔operations
  lockstep is enforced by `api/drift_test.go` (`TestCodegenDrift`) and
  `api/gen/codegen_test.go` (`TestNewOperationYieldsScaffolding`).
- **Owner-model visibility (rule D-8)** — `infra/postgres/scope_access.go
  visibilityCond`: a row is visible iff `owner_id = me` OR
  (`owner_household_id` IS NOT NULL AND `owner_household_id IN households(me)`).
  It is applied in the generic repo engine (`infra/postgres/repository.go`
  `Get`/`List`/`Update`) and mirrored by RLS policies + a membership trigger
  (`migrations/00004_row_level_security.sql`), using the `fn_visible_households()`
  helper and the transaction-local `set_config('app.user_id', …)` binding in
  `infra/postgres/scope.go`.
- **Ingest pipeline** — `core/ingest/service.go
  `Service.Process(ctx, filename, payload, contentType, ownerHouseholdID)
  (entity.Asset, error)`: size check → `storage.Put` → `Sources.Create` (pre-tx,
  so the Source survives later failures) → `extractor.Extract` (LLM) →
  `parse.ParseExtraction` → `InTx{ identity.Resolve + Documents.Create }`.
- **Identity resolution** — `core/identity/resolve.go
  `Resolve(ctx, assets, ext, ownerHouseholdID) (entity.Asset, created bool,
  error)`: serial-first then brand+model hierarchy, `scopeFence` (household vs
  `IS NULL`), `mergeInto` (non-nil fields only) / `createNew` / `newAsset`.
  `ErrNoIdentity` when no usable identity. Backstop: unique index
  `uniq_assets_owner_norm_serial`.
- **Extraction parsing** — `commons/parse/parse.go
  `ParseExtraction(raw) (entity.Extraction, error)`; `rawExtraction` is
  all-`any` with permissive absence (bad value → nil, never an error);
  `entity.Extraction` (`commons/entity/extraction.go`). The LLM prompt is
  `infra/llm/prompt.go SystemPrompt()`.
- **Repositories** — generic `repo.Repository[T]` (`commons/repo/generic.go`)
  with the `Option`/`Options`/`Filter` pattern (`commons/repo/options.go`:
  `Owner`, `Where`, `Limit`, `Offset`, `OrderBy`). Concrete repos in
  `infra/postgres/` (`AssetRepository`, `AccountRepository`,
  `MovementRepository`, `DocumentRepository`, `ImportBatchRepository`, …) plus a
  `repo.Factory` that also carries `InTx`. Custom query methods already exist as
  precedent: `MovementRepository.BalanceForAccount`, `DocumentRepository
  .LinkCandidates` — the model for search's per-type queries.
- **OpenAPI source-of-truth** — `api/openapi.yaml` (OpenAPI 3.1); `make
  codegen-go` regenerates `api/gen/openapi.gen.go` (Go DTOs + `ServerInterface` +
  chi routes). `asset` and `document` schemas are at `openapi.yaml` lines ~1042 /
  ~1092.
- **Config** — `config/config.go Load(src)`: `PROCRASTINATOR_`-prefixed vars;
  the `PROCRASTINATOR_MAX_UPLOAD_BYTES` fail-fast pattern (getOrDefault → parse →
  `config: invalid …`) is the model for the new threshold var.
- **Migrations** — goose, `migrations/NNNNN_name.sql` (5-digit, next = `00005`).
  Every owned table carries `owner_id NOT NULL REFERENCES users(id)` + nullable
  `owner_household_id REFERENCES households(id)`.

**Constraints driving the design:** MVP (data is not real; matching/confidence
strategy expected to evolve → keep it simple and easy to redesign); strict
multi-tenancy (no cross-owner leakage, RLS backstop); no new auth/RLS model; no
per-field confidence, no relevance ranking, no full-text search; additive
contract except the upload `202` outcome.

## Goals / Non-Goals

**Goals:**
- Add tenant-scoped, read-only general search across the user's owned
  resources (asset, account, movement, document, import batch) with a fast
  capped quick endpoint and a paged results endpoint, a shared hit shape, and a
  single deterministic combined ordering (quick results = a prefix of the
  results-page ordering).
- Make ingest confidence-aware: the extraction carries a single `confidence`;
  identity resolution is gated by a configurable threshold (≥ threshold
  auto-commits exactly as today; below/absent is held as a pending review); the
  pending review is owner-scoped + RLS-protected with approve (commit, reusing
  identity-resolution/merge) and reject (discard, retain Source) endpoints.
- Keep every stage within the existing owner-model visibility rule, RLS backstop,
  path tenancy, and OpenAPI source-of-truth conventions.

**Non-Goals:**
- No new authentication/authorization model, no header tenancy, no new middleware.
- No per-field confidence, no relevance ranking, no full-text/tokenized search,
  no search indexes (ILIKE substring is literal; `pg_trgm` is a deferred
  enhancement).
- No re-extraction, no candidate editing, no un-approve / rollback.
- Households are not searchable. No UI beyond the three surfaces (quick-search
  typeahead, paged search results page, ingest review queue) plus the upload `202`
  held-for-review state; no new per-resource detail pages, no theming, no
  search-history persistence, no client-side full-text filtering.

## Decisions

### D1 — Confidence is a single nullable per-record value in `[0,1]`

`confidence` is a single real in `[0.0, 1.0]`, stored as a **nullable** column,
introduced at three layers:
- `entity.Extraction.Confidence *float64` (`commons/entity/extraction.go`) —
  parsed by `commons/parse` (`rawExtraction.Confidence any` → a new
  `confidencePtr` validator). Absent / non-numeric / out-of-range → **nil**
  (never an error, and it never drops other valid fields) — consistent with the
  existing permissive absence handling. `infra/llm/prompt.go SystemPrompt()`
  gains an instruction to emit a `confidence` field.
- `entity.Asset.Confidence *float64` → `assets.confidence real NULL`;
  `entity.Document.Confidence *float64` → `documents.confidence real NULL`
  (migration `00005`). NULL preserves "created before confidence existed" and
  "absent"; search surfaces it where present.
- `ingest_reviews.confidence real NULL` (the extraction's confidence at hold
  time — below threshold or absent).

*Rationale:* single overall confidence is the MVP contract (spec: no per-field).
Nullable (not default) is required so an absent confidence is distinguishable
from a real 0.0, and so pre-existing rows are valid. *Alternatives rejected:*
per-field confidence (out of scope, over-engineered); integer 0–100 (spec
mandates real in [0,1]); NOT NULL with a sentinel (conflates "unknown" with a
value — defeats the fail-safe).

### D2 — The confidence gate is a pure predicate; the commit/hold actions live in the pipeline

The gate is one decision on `(confidence, threshold)`:

```
commit := confidence != nil && *confidence >= threshold   // ≥ threshold commits
```

- **No usable identity** (no serial, no brand+model) → `422`, checked
  independently of the gate (it arises from `identity.Resolve`/`Match` returning
  `ErrNoIdentity`).
- **commit** → the existing `InTx{ identity.Resolve + Documents.Create }` runs
  unchanged → `201` with the Asset.
- **hold** (confidence below threshold OR absent) → the Source is already
  retained; a pending review is created (no Asset, no Document) → `202` with a
  review reference.

`ingest.Service.Process` changes its result from `(entity.Asset, error)` to
`(ingest.Result, error)` where

```go
type Result struct {
    Committed *entity.Asset   // set on auto-commit (≥ threshold)
    Review    *entity.IngestReview // set on hold (below/absent)
}
```

`error` still carries the hard-failure paths (`ErrTooLarge`→413,
`filestorage.ErrUnsupportedType`→415, `ErrExtraction`→502,
`identity.ErrNoIdentity`→422, default→500) — `writeProcessError` is unchanged.
The handler (`api/handlers.go UploadDocument`) maps `Committed`→`201`,
`Review`→`202`.

The **threshold** comes from `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD`
(default `0.7`, range `[0,1]`, fail-fast on invalid — the `MaxUploadBytes`
pattern). It is held on `ingest.Service` (the only place it is consulted).

*Rationale:* a one-line, pure, table-testable predicate keeps the safety
property (fail-safe: absent is never committed) trivially verifiable, and the
`Result` shape preserves the existing error-mapping while adding the `202`
outcome. *Alternatives rejected:* encode the outcome in the error (can't
express 201-vs-202 cleanly); separate endpoints (spec is one upload endpoint);
a dedicated gate service (a one-liner doesn't warrant it).

### D3 — `identity` gains a non-committing `Match` and a `CommitCandidate`; `Resolve` shares the matcher

The hold path needs the **best-matched existing asset without committing**; the
approve path must **commit a stored candidate** against its recorded best-match.
Both reuse the existing, tested matching/merge/create logic:

- Extract the Limit(1) match query from `resolveBySerial` /
  `resolveByBrandModel` into a shared `lookup(…)` helper.
- `Match(ctx, assets, ext, ownerHouseholdID) (entity.Asset, found bool, error)`:
  normalizes, applies the same hierarchy, returns `ErrNoIdentity` when no usable
  identity, else the best match (`found=true`) or `(zero, false, nil)` (a new
  asset would be created). **Does not write.**
- `CommitCandidate(ctx, assets, ext, matchedAssetID *string, ownerHouseholdID)
  (entity.Asset, error)`: if `matchedAssetID != nil`, `assets.Get(…, fence…)` —
  on `ErrNotFound` (deleted / now-invisible) fall back to `createNew`; else
  `mergeInto`. If `matchedAssetID == nil`, `createNew`. Reuses
  `mergeInto` / `createNew` / `newAsset` / `scopeFence`.

**Confidence stamping** (so search can surface it): `newAsset` sets
`Confidence: ext.Confidence`; `mergeInto` sets `updated.Confidence` **only when
`ext.Confidence != nil`** (non-empty-only, identical to how Brand/Model/etc. are
merged). Thus both auto-commit and approve stamp the committed/merged asset's
confidence, and a nil candidate confidence never erases an existing one.

*Rationale:* reuses the existing resolution/merge logic verbatim (spec: "reusing
the existing identity-resolution and field-merge logic"). Committing against the
**recorded snapshot** best-match (not re-matching) makes approve deterministic
and safe — it commits into exactly the asset the reviewer was shown. *Alternatives
rejected:* re-run full `Resolve` on approve (re-matches against a mutated DB and
could merge into a *different* asset than the one displayed); store a full
candidate asset row (duplication).

### D4 — `ingest_reviews`: owner-scoped, RLS-protected, snapshot reference

New table (migration `00005`), following the exact shape of the seven existing
owned tables:

| column | type / constraint |
|---|---|
| `id` | `uuid PK DEFAULT gen_random_uuid()` |
| `owner_id` | `text NOT NULL REFERENCES users(id)` |
| `owner_household_id` | `text REFERENCES households(id)` (nullable; NULL = personal) |
| `source_id` | `uuid NOT NULL REFERENCES sources(id)` (the retained upload) |
| `doc_type` | `text NOT NULL CHECK (doc_type IN ('invoice','warranty','amc','other'))` |
| `candidate_fields` | `jsonb NOT NULL` (the candidate's structured fields) |
| `raw_extraction` | `jsonb NOT NULL DEFAULT '{}'` (the original LLM raw output) |
| `confidence` | `real NULL` |
| `best_matched_asset_id` | `uuid REFERENCES assets(id) ON DELETE SET NULL` (nullable snapshot) |
| `state` | `text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','approved','rejected'))` |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` |
| `decided_at` | `timestamptz NULL` |
| `decided_by` | `text REFERENCES users(id)` (nullable) |

- `candidate_fields` is **the exact `extractionFields` map vocabulary**
  (classification, brand, model, serial_number, price, currency, purchase_date,
  warranty_end, metadata) — the same map `ingest.extractionFields` builds today.
  On approve it is rehydrated into an `entity.Extraction` to feed
  `CommitCandidate`, and stored verbatim as the committed document's
  `extracted_fields`. This resolves the spec-review gap (SPX-011) that named no
  field set.
- `raw_extraction` is stored so an approve-created document is identical in
  fidelity to an auto-committed one (satisfies `documents.raw_extraction
  NOT NULL`).
- **`best_matched_asset_id` is a snapshot FK with `ON DELETE SET NULL`** (resolves
  the CRIT-S-04 lifecycle gap): if the matched Asset is later deleted, the
  reference nulls and approve falls back to `createNew` — no dangling id, no
  re-match surprise.
- **RLS + membership trigger** in `00005` (reusing `fn_visible_households()` and
  `enforce_household_membership()` from `00004`): `ENABLE`+`FORCE ROW LEVEL
  SECURITY` + `ingest_reviews_owner_isolation` (the D-8 predicate) +
  `ingest_reviews_household_membership` trigger. New `ReviewRepository`
  (`infra/postgres`) with `shareable: true`.
- **Lifecycle state machine:** `pending → approved` (approve) or
  `pending → rejected` (reject); both terminal. **Approve is atomic** in a single
  `InTx`: `CommitCandidate` + `Documents.Create` + review transition — all or
  none. **Reject** is a state transition only (no Asset/Document change; the
  Source is retained). Approving a non-`pending` review → `409`; unknown /
  another-owner id → `404`.

*Rationale:* matches the existing owned-tables pattern exactly (policy + trigger
+ shareable repo), so the RLS backstop and the app-level visibility rule both
cover reviews with no new machinery. The snapshot FK makes the asset lifecycle
self-documenting at the DB level. *Alternatives rejected:* live reference +
Go-side Get-or-create (leaves dangling ids; less self-documenting); a separate
`candidates` table (over-engineered for a 1:1 review↔candidate).

### D5 — Search is federated per-type custom queries merged under one deterministic order

New `core/search` service depending on a `repo.SearchBackend` interface (defined
in `commons/repo`, implemented in `infra/postgres`) with five methods:
`SearchAssets`, `SearchAccounts`, `SearchMovements`, `SearchDocuments`,
`SearchImportBatches`. Each method is a **custom SQL query** (not the generic
`Where`, which ANDs its filters — search needs OR across a type's fields). Each
query:

- runs in `scope.run(ctx, tid, …)` (binds `app.user_id` → RLS backstop applies),
- applies `visibilityCond` (D-8) for app-level owner scoping,
- matches a **case-insensitive literal substring** via `ILIKE` on the type's
  fields, with `%` / `_` / `\` escaped in the (single, parameterized `$1`)
  pattern so metacharacters match literally,
- is `ORDER BY created_at DESC, id ASC`.

Searchable fields per type: `asset` → brand OR model OR serial_number; `account`
→ name OR type OR institution; `movement` → description OR external_reference;
`document` → **joined** `sources.filename` (a `documents ⋈ sources` query on
`source_id`, reusing the same-scope invariant that a document and its source
share `owner_id`/`owner_household_id`); `import_batch` → filename.

**Combined order** is the concatenation of the five ordered streams in the fixed
type priority `asset, account, movement, document, import_batch`. Because type
priority dominates, this yields: quick(top-N) = the first N of the concatenation;
paged = a slice of the concatenation with `total =` the sum of the five lengths.
This construction **guarantees quick is a strict prefix of the results page** for
the same query — the spec's core determinism invariant — with no global
cross-type `ORDER BY` to get wrong.

**MVP fetches each type's full matching list** (no per-type pagination) and merges
in Go; `total = len(combined)`. *Rationale:* the data is not real; correctness
and the prefix invariant are prioritized over query optimization, and the
quick-endpoint "does not compute a whole-collection count" NFR is marked **soft**
in the proposal. *Documented future enhancement:* per-type `COUNT` + offset
math, or a single `UNION ALL` with a `type_rank` column in `ORDER BY`.

**Hit shape** `search_hit{ type, id, title, subtitle?, confidence? }`. `title`
is a non-blank display string derived per type (asset: brand+model, else model,
else serial; account: name; movement: description; document/import_batch:
filename), falling back to the type name so it is never blank. `subtitle` is a
secondary line (e.g. asset serial, account type+institution, movement external
reference). `confidence` is present only where the record carries one: asset →
`asset.confidence`, document → `document.confidence`; absent for account /
movement / import_batch and for NULLs.

**Param rules:** blank/empty `q` → `200` empty (no error); `q` > 200 chars →
`400`; quick `limit` ∈ [1,50] default 10, no pagination metadata; paged `page`
≥ 1 default 1, `page_size` ∈ [1,100] default 20, returns `results`/`page`/
`page_size`/`total`; a page beyond the last → `200` empty `results` + unchanged
`total`.

*Rationale:* custom queries are the only way to express OR-across-fields plus the
document→source join while keeping D-8 visibility + RLS; the concatenation model
is the simplest correct way to get a stable combined order where quick is a
prefix. *Alternatives rejected:* generic-engine multi-filter (AND-only, no OR);
single `UNION ALL` with `type_rank` (correct + faster, but more SQL and harder
to keep per-type field lists/visibility in one place — deferred); full-text /
`pg_trgm` (out of scope for MVP).

### D6 — OpenAPI + codegen stay additive with 1:1 lockstep

`api/openapi.yaml` gains, all additive: two search operations
(`GET /api/users/{userId}/search/quick`, `GET /api/users/{userId}/search`) and
four review operations (`GET …/ingest/reviews`, `GET …/ingest/reviews/{id}`,
`POST …/ingest/reviews/{id}/approve`, `POST …/ingest/reviews/{id}/reject`); new
schemas `search_hit`, `search_quick_response`, `search_results_page`,
`ingest_review`, `approve_review_response`; and an optional nullable
`confidence` on the existing `asset` and `document` schemas. Every non-2xx
response references the standard `error` envelope.

`make codegen-go` regenerates `api/gen/openapi.gen.go` (new `ServerInterface`
methods + DTOs + chi routes). The six new `ServerInterface` methods are
implemented on `*api.Server`. `TestCodegenDrift` and
`TestNewOperationYieldsScaffolding` must pass.

*Rationale:* preserves the source-of-truth convention and the lockstep the
drift tests enforce. *Query params* (`q`, `limit`, `page`, `page_size`) follow
the existing `listMovements` precedent (a generated `…Params` struct).

### D7 — One additive migration, `00005_confidence_reviews_search.sql`

A single migration, backward-compatible and non-destructive:
1. `ALTER TABLE assets ADD COLUMN confidence real;` and
   `ALTER TABLE documents ADD COLUMN confidence real;` (nullable; NULL for
   existing rows).
2. `CREATE TABLE ingest_reviews (…)` per D4, with a composite index on
   `(owner_id, state, created_at)` for the list endpoint.
3. RLS on `ingest_reviews` (policy + FORCE) + the membership trigger, per D4.
4. **No search indexes** — ILIKE with a leading `%` cannot use a btree, and
   `pg_trgm` is a real extension dependency deferred as an enhancement. The
   proposal's "optional text-column indexes for search" is therefore intentionally
   omitted in MVP.

*Rationale:* one additive migration keeps the deploy atomic and trivially
reversible; omitting search indexes avoids a new extension and matches the
"deliberately simple" MVP stance.

### D8 — UI is OpenAPI-first, tenant-scoped, and layered over the existing client/hooks

The three UI surfaces consume the `search` and `confidence-review` contracts
through the **existing, layered UI data path** — no new data path:

- **Generated (never hand-edited):** `ui/src/lib/api/generated/orval/procrastinator.ts`
  (orval per-operation functions + types) and `ui/src/lib/api/generated/paths.d.ts`
  (openapi-typescript). P9's `make codegen` already regenerates both with the six
  new operations (`quickSearch`/`search`, `listReviews`/`getReview`/`approveReview`/
  `rejectReview`) and schemas (`search_hit`, `search_quick_response`,
  `search_results_page`, `ingest_review`, `approve_review_response`) plus
  `confidence` on `asset`/`document`. The TS drift checks
  (`ui/src/lib/api/codegen.drift.test.ts`, `ui/src/lib/api/codegen.client.drift.test.ts`)
  prove they are codegen output — this is the OpenAPI-first lockstep.
- **Hand-written app code (the only place new search/review API code lives):**
  `ui/src/lib/api/schema.ts` (re-export the new generated types),
  `ui/src/lib/api/client.ts` (thin typed wrappers that unwrap `{data,status,headers}`
  into just the data, mirroring the existing `listAssets`/`createAccount` wrappers),
  and `ui/src/lib/api/hooks.ts` (one `useQuery` per read + one `useMutation` per
  write, following D4/D5: every query key carries the active user id, requests are
  disabled while no user is active, and each mutation invalidates exactly the named
  prefixes).

**Query keys + invalidation (D4/D5 extension):**
- Reads: `useQuickSearch(q, limit)` → `['search-quick', uid, q, limit]`;
  `useSearch(q, page, pageSize)` → `['search', uid, q, page, pageSize]`;
  `useReviews(status)` → `['reviews', uid, status]`;
  `useReview(id)` → `['review', uid, id]`.
- Writes: `useApproveReview()` → invalidates `['reviews', uid]`,
  `['review', uid, id]`, **and** `['assets', uid]` (approve creates/merges an Asset
  + a Document, mirroring `useCommitBatch`'s multi-prefix invalidation);
  `useRejectReview()` → invalidates `['reviews', uid]`, `['review', uid, id]`
  (no canonical record changes, so assets/movements are untouched).
- `useQuickSearch`/`useSearch` are read-only and invalidate nothing.

**Surfaces + routes (added to `ui/src/router.tsx`, rendered in `<AppShell />`):**
- **Quick-search typeahead** — `ui/src/components/search/quick-search.tsx`, mounted
  in `ui/src/components/layout/app-shell.tsx` (top of the main column next to the
  brand at `md+`; in the mobile top bar below `md`). Debounced (250 ms) input →
  `useQuickSearch`; dropdown of hits (title + optional subtitle + optional
  confidence); click/Enter → navigate; Escape → dismiss; combobox/listbox ARIA.
- **Results page** — `ui/src/pages/search/search-results-page.tsx` at `/search`
  (query `?q=…`), paged via `useSearch`, type indicator + title + subtitle +
  confidence, pagination controls (page/page_size/total), empty/loading/error
  states, hits rendered in endpoint order (no client re-sort).
- **Review queue** — `ui/src/pages/reviews/review-queue-page.tsx` at
  `/ingest/reviews` (optionally `/ingest/reviews/:id` for the detail), a status
  filter (default `pending`; also `approved`/`rejected`) via `useReviews`, per-row
  approve/reject behind the existing `ui/src/components/confirm-dialog.tsx` →
  `useApproveReview`/`useRejectReview`, success via the existing toast
  (`ui/src/components/ui/sonner.tsx`), 409/404 surfaced as errors.

**Navigation mapping (hit → route):** `asset` → `/assets/{id}`; `import_batch` →
`/finance/import/{id}`; `account` → `/finance/accounts`; `movement` →
`/finance/movements`; `document` → `/assets` (a document's owning asset is not
addressable from the hit's document id alone; a document detail page is a documented
future enhancement). Types without a detail page land on their list page.

**Upload `202`:** `ui/src/lib/api/upload.ts` + `useUploadDocument` currently model
only the `201` Asset. They gain the `202` outcome: the mutation resolves to a
discriminated union `{ kind: 'committed', asset } | { kind: 'held', review }`; the
upload page (`ui/src/pages/upload/upload-page.tsx`) shows a "held for review" state
(no asset) with a link to `/ingest/reviews`; `422`/`502` errors are unchanged.
`schema.ts`/`client.ts` wire the generated `202`/`ingest_review` types.

**Confidence display:** `ui/src/lib/format/` gains a confidence formatter (absent →
no indicator, never `0`); used on search hits, the asset detail, and review rows.

**Testing (MSW + Testing Library + axe):** component/integration tests in
`ui/src/pages/**`, `ui/src/components/search/**`, and `ui/src/components/reviews/**`
use the existing MSW harness (see `ui/src/lib/api/upload.test.ts` /
`ui/src/lib/api/hooks.test.tsx` for the mock pattern) to assert the
dropdown/results/queue behavior, the upload `202` state, and the approve/reject
flows; `vitest-axe` asserts accessibility. Verify: `npm run test` (vitest) in `ui/`,
`tsc -b` clean, and `make codegen-drift-check` (proves no hand-edited generated
files).

*Rationale:* layering the UI over the existing generated-client → wrapper → hooks
path (D4/D5) keeps a single data path, preserves the tenancy invariant (every
request is per-active-user, and the cache is cleared on user switch), and makes the
OpenAPI-first constraint **structural** — the only new API code is thin app wiring,
and the generated files stay codegen output and drift-checked. The navigation mapping
and the `202` discriminated union are the two places the existing UI had to change to
absorb the new contract. *Alternatives rejected:* a dedicated UI API client separate
from `client.ts` (breaks the single-path convention); client-side full-text filtering
(out of scope — the server does the matching); per-resource detail pages for
account/movement/document (scope creep — land on list pages instead).

## Risks / Trade-offs

- **Accidental cross-owner leakage** (search + review both span multiple tables)
  → Reuse the existing D-8 `visibilityCond` **and** the RLS backstop **and** the
  membership trigger on `ingest_reviews`; both layers enforce the same rule
  (defense in depth). Dedicated isolation scenarios: another owner's personal row
  is never returned; a member sees only their households' rows; an unbound
  `app.user_id` session sees nothing.
- **A low-confidence read being auto-committed** → fail-safe gate: absent or
  below-threshold confidence is held, never committed; `≥ threshold` (inclusive)
  commits. Threshold is configurable; the inclusive boundary is tested.
- **Approving a review whose best-matched Asset was deleted / became invisible**
  → snapshot FK + `ON DELETE SET NULL` → approve falls back to `createNew`;
  atomic in one transaction; no dangling id.
- **Non-atomic approve** → single `InTx`; on failure the review stays `pending`
  and no Asset/Document changed; `409` for non-`pending`, `404` for unknown /
  invisible.
- **Search cost on large data** → MVP full-fetch per type; acceptable because the
  data is not real; documented `UNION ALL` / `pg_trgm` enhancement.
- **SQL injection / metacharacter abuse via `q`** → all search queries are
  parameterized (`$1`); `%`/`_`/`\` are escaped so the query matches literally.
- **`documents.raw_extraction` / `extracted_fields` NOT NULL on the approve path**
  → the review stores both `candidate_fields` and `raw_extraction`; approve copies
  them onto the new document.
- **Confidence backfill** → existing assets/documents have NULL confidence; search
  treats NULL as "absent" (valid hit), and merge never overwrites an existing
  confidence with nil.

## Migration Plan

1. **Schema** — deploy `00005` (additive, non-destructive). Applied at server
   startup via `Store.Migrate` (goose.Up); also applied per private schema in the
   DB integration / e2e suites. No existing column is altered in a breaking way.
2. **Code** — additive operations + schemas; the upload endpoint gains the `202`
   outcome. Behavior change is gated by the threshold (default 0.7): real
   high-confidence extractions still auto-commit (`201`), while **absent- or
   low-confidence extractions now hold (`202`) instead of committing** — the
   intended change.
3. **Rollback** — `00005` has a Down (drop RLS + trigger, drop
   `ingest_reviews`, drop the two confidence columns). Rolling back the **code**
   while keeping the schema is safe: the old `Process` ignores the confidence
   columns and the reviews table. Residual: rows already held as `pending` reviews
   would have no approve/reject path in the old code — acceptable for MVP (data
   is not real).

## Open Questions

- **Should pending (held) uploads appear in search?** Decided **no** — search
  surfaces committed rows (documents/assets/…); held uploads surface via the
  review queue. Recorded here for visibility, not a blocker.
- **Household search** — deferred (out of scope per proposal); would be a small
  additive type in the `SearchBackend`.
- **Per-field confidence / relevance / FTS / `pg_trgm`** — documented future
  enhancements; the `search_hit` and per-type query seams are designed so these
  slot in without changing the combined-order invariant.
