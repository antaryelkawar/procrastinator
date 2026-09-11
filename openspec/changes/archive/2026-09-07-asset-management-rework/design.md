# Design: asset-management-rework

Status: **proposed** (SUBSPEC phase — cycle-2 delta D14–D22 appended)

Technical design for HOW to implement the asset-management rework: the six new
capabilities (`asset-lifecycle`, `processing-pipeline`, `asset-taxonomy`,
`unified-add`, `search-experience`, `web-platform`) and the five modified
capabilities (`asset-registry`, `asset-identity-resolution`, `document-ingestion`,
`llm-extraction`, `api-contract`). Requirements live in `specs/`; motivation in
`proposal.md`. This document explains the architecture and the "why" behind each
decision, grounded in the existing Go backend (`procrastinator-backend/`, flat
`api/ core/ infra/ commons/` layout) and the web UI (`ui/`).

## Context

The backend already implements the pieces this change reworks (verified against
code):

- **Ingest pipeline** — `core/ingest/service.go`
  `Service.Process(ctx, filename, payload, contentType, ownerHouseholdID)
  (Result, error)`: size check → `storage.Put` (→ `Source` with `SHA256`,
  `infra/filestorage/storage.go:67`) → `Sources.Create` (pre-tx, survives later
  failures) → **single** `extractor.Extract` (one LLM call, `infra/llm/extractor.go`)
  → `parse.ParseExtraction` (`commons/parse/parse.go`) → a confidence gate
  (`ext.Confidence >= threshold`, default `0.7` from
  `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD`) → either `InTx{identity.Resolve +
  Documents.Create}` (commit) or `reviewer.Hold` (hold). `Result` is a
  `{Committed *Asset, Review *IngestReview}` pair.
- **Identity resolution** — `core/identity/resolve.go`: `Resolve` / `Match` /
  `CommitCandidate` / `mergeInto` / `createNew` / `newAsset` / `scopeFence`.
  Hierarchy is **serial → brand+model** (two stages) via `List(..., Limit(1))`
  on `norm_serial` / (`norm_brand`,`norm_model`). `mergeInto` is non-nil-only and
  currently stamps `DocType` **last-write-wins** (line 277-279). Backstop: unique
  index `uniq_assets_owner_norm_serial` (`migrations/00002_core_owned.sql:39`).
- **Asset model** — `commons/entity/asset.go`: structured core (brand, model,
  serial, norm_* , dates, price, currency, `DocType`, `Metadata` JSONB,
  `Confidence`, `OwnerHouseholdID`). **No** `name`, **no** `asset_category`, **no**
  `deleted_at`. `commons/entity/extraction.go` has no `name`, no
  `warranty_duration`, no `asset_category`.
- **Extraction** — `infra/llm/client.go` `Client.Chat` (one OpenAI-compatible
  `/chat/completions` call); `infra/llm/prompt.go` `SystemPrompt()`.
  `commons/parse/parse.go` `ParseExtraction` is permissive-absence (bad value →
  nil, never an error). `commons/normalize.go` already provides
  `NormalizeSerial` / `NormalizeName` / `CollapseWhitespace`.
- **Review / search / statement** — `core/review/service.go`
  (`Hold`/`Approve`/`Reject`/`List`/`Get`, reusing `identity.Match`/
  `CommitCandidate`); `core/search/service.go` over `repo.SearchBackend`
  (`commons/repo/search.go` — five per-type ILIKE custom queries, D-8 visibility +
  RLS); `core/statement/service.go` (import-batch preview/commit for the ledger).
- **Repositories** — generic `repo.Repository[T]` (`commons/repo/generic.go`) with
  the `Option`/`Options`/`Filter` pattern (`commons/repo/options.go`: `Owner`,
  `Where`, `Limit`, `OrderBy`); concrete repos in `infra/postgres/`; `repo.Factory`
  carries `InTx`. Owner-model visibility (`infra/postgres/scope_access.go
  visibilityCond`) + RLS backstop + membership trigger (`migrations/00004_*`)
  protect every owned table.
- **API + codegen** — path-tenancy under `/api/users/{userId}`; routes are derived
  from `api/openapi.yaml` by `make codegen-go` (→ `api/gen/openapi.gen.go`, strict
  `ServerInterface`); handlers implement methods on `*api.Server`
  (`api/handlers.go`, `api/search.go`, …). 1:1 routes↔operations lockstep is
  enforced by `api/gen/drift_test.go` + `api/gen/codegen_test.go`. The UI consumes
  the same contract via `make codegen-ts` (→ `ui/src/lib/api/generated/paths.d.ts`)
  and `make codegen-ts-client` (→ `ui/src/lib/api/generated/orval/`), both
  drift-checked.
- **Composition root** — `api/cmd/procrastinator/main.go`: builds
  `llm.New` → `llm.NewExtractor` → `ingest.New(factory, extractor, storage,
  maxBytes, threshold, reviewSvc)` → `api.New(svc, factory, …)`.
- **Migrations** — goose, `migrations/NNNNN_name.sql` (5-digit; next is `00006`).
  `assets`/`documents`/`sources` in `00002`, confidence + `ingest_reviews` in
  `00005`. Every owned table carries `owner_id NOT NULL` + nullable
  `owner_household_id`.
- **UI** — Vite react-ts + Tailwind v4 + shadcn/ui + React Router 7 + TanStack
  Query 5 + react-dropzone + orval/openapi-typescript. Routes in `ui/src/router.tsx`
  (`/upload`, `/assets`, `/assets/:assetId`, `/finance/*`, `/search`,
  `/ingest/reviews`); data path is generated-client → `lib/api/client.ts` wrappers →
  `lib/api/hooks.ts`; screens render **hand-rolled** `<Table>` per page
  (e.g. `ui/src/pages/assets/asset-list-page.tsx:34`). Current pinned majors
  (`ui/package.json`): React 19, Vite 6, Tailwind 4, TanStack Query 5, react-router 7.
- **Sample fixtures** — `sample-data/` (repo root): `INVNAG2302754.pdf` (invoice),
  `Annual Maintenance Contract.pdf` (AMC, "MICRO WAVE OVEN CONVECTION 30BRC2"), and
  four product-label photos (incl. "CABINET COOLER MASTER CD600 BLACK"). These are the
  canonical extraction test corpus.

**Constraints driving the design:** strict multi-tenancy (no cross-owner leakage,
RLS backstop, path tenancy); OpenAPI source-of-truth with 1:1 codegen lockstep;
additive, reversible migrations; MVP data (not real) — prefer simple, easy to
redesign; reuse the existing pipeline/identity/repo/codegen conventions; no new
auth/RLS model; latest-stable frontend stack (no pre-release/canary); the workflow
only ends when every flow — including the UI — is verified running.

## Goals / Non-Goals

**Goals:**
- Replace the single-shot ingest path with a **standalone `core/processing`
  module** (`Process(input) → Outcome`) with **parallel multi-model extraction**,
  **dual consensus**, **product-description splitting** (brand/name/model),
  **deterministic warranty-duration** arithmetic, **intrinsic asset categorization**,
  and **efficient indexed existing-asset lookup** for linking.
- Add **asset lifecycle**: soft-delete + restore (retention window) + purge, and
  **merge/dedupe** (field strategy, document preservation, audit).
- Guarantee **dedupe-on-reupload** by content hash (source-level + identity-level).
- Add a **unified `POST /add`** entry (file/text/statement) with **uniform
  outcomes**; fold statement files into the ledger import pipeline.
- Extend **search** with structured filters (category, brand, date range, warranty
  status, has-documents, doc classification) and kind-labeled hits; make search the
  **primary UI navigation**.
- Rebuild the UI on a **latest-stable stack**, a **shared feature-rich DataTable**
  (TanStack Table), **consolidated primitives**, and a simple-but-modern direction
  with a single Add surface.

**Non-Goals** (per proposal "Out of Scope" — treated as hard constraints):
- **Mobile/native apps** — web UI only.
- **Handwritten-document OCR** — photos of printed labels/receipts only.
- **Cross-owner / marketplace sharing** — assets stay owner/household-scoped.
- **Ledger behavior changes** — accounts, movements, balances, import-batch
  semantics are untouched; only the UI entry point to statement import moves.
- **Email watcher / automatic ingestion sources** — the module admits them, none
  are built.
- **Historical data backfill** — migration adds columns; reprocessing legacy
  documents to populate `name`/`asset_category` is a follow-up.
- **Hard-delete of sources/documents** — soft-deleted assets purge after retention,
  but source files and document rows are retained indefinitely.
- **Per-field confidence / relevance ranking / full-text search / `pg_trgm`** —
  consensus confidence is a single value; search stays ILIKE-substring (existing).
- **Un-merge / rollback of a merge** — a merge is terminal (duplicate soft-deleted).

## Decisions

### D1 — Standalone `core/processing` module replaces `core/ingest`; one entry point, four outcomes

We will move the pipeline from `core/ingest` into a new standalone module
`core/processing` with a single entry point:

```go
// core/processing
type Input struct {
    Filename         string
    Payload          []byte
    ContentType      string
    OwnerHouseholdID *string
}

type OutcomeKind string
const (
    OutcomeCommitted     OutcomeKind = "committed"       // asset created/merged
    OutcomeHeldForReview OutcomeKind = "held_for_review" // below confidence / ambiguous
    OutcomeDuplicate     OutcomeKind = "duplicate"       // re-upload of a known document
    OutcomeFailed        OutcomeKind = "failed"          // hard failure (all workers down, oversize, …)
    OutcomeStatement     OutcomeKind = "statement"       // internal: classified as a statement → route to ledger
)

type Outcome struct {
    Kind       OutcomeKind
    Asset      *entity.Asset
    Review     *entity.IngestReview
    Duplicate  *Duplicate     // existing source/document/asset refs
    Statement  *entity.Source // set when Kind == OutcomeStatement
    Reason     string         // for OutcomeFailed
    Confidence *float64       // consensus confidence
    Provenance Provenance     // per-worker results + errors
}
type Duplicate struct {
    SourceID     string
    DocumentID   string
    AssetID      string
    AssetDeleted bool // true → UI should offer restore
}
```

`Process(ctx, Input) (Outcome, error)` runs: size check → **content-hash dedupe**
(D7) → `storage.Put` + `Sources.Create` (pre-tx, retained on failure) → **parallel
extraction + consensus** (D2) → **warranty** (D3) + **category** (D5) →
**indexed identity lookup** (D6) → commit / hold / duplicate. The existing
`POST /documents` handler (`api/handlers.go UploadDocument`) is re-pointed at
`processing` (201 committed / 200 duplicate / 202 held); `core/ingest` is deleted.

*Why:* the proposal mandates a standalone module so any ingestion path (file,
pasted text, statement, future email watcher) shares one pipeline. The `Outcome`
discriminated union preserves the current error-mapping while expressing the four
asset outcomes plus the internal `statement` signal (D9). `error` still carries
hard failures (`ErrTooLarge`→413, unsupported type→415, all-workers-failed→502).
*Alternatives rejected:* keep `ingest` and bolt a statement branch onto the HTTP
handler (scatters the pipeline; violates the "same pipeline" requirement); return
the statement decision as `OutcomeFailed` (wrong semantics — a statement is a
successful routing, not a failure); a separate `candidates` table (over-engineered
for a 1:1 review↔candidate).

### D2 — Parallel multi-worker extraction + a pure consensus step

We will run **at least two independent extraction workers** concurrently and combine
them through a pure consensus function before any canonical write.

- **Workers.** A `Worker` = `{Model, BaseURL, Strategy}`. `infra/llm` keeps the
  existing `Client.Chat` (one OpenAI `/chat/completions` call); a new
  `core/processing/extract.go` runs N workers via `errgroup` (each with its own
  timeout = `PROCRASTINATOR_LLM_TIMEOUT`), each producing a raw response parsed by
  `parse.ParseExtraction` into an `entity.Extraction`. A worker's transport/parse
  failure is recorded, **not fatal**.
- **Config.** `PROCRASTINATOR_LLM_WORKERS` (comma list of `model[@baseURL]`),
  default derived from the existing `PROCRASTINATOR_LLM_MODEL`: two workers on the
  same model with two strategies (`extract`, `verify`) — "dual consensus" without
  requiring two API keys. Parallelism is **mandatory** in the default path
  (llm-extraction: "a single-request (non-parallel) path does not exist outside an
  explicit degraded-mode configuration").
- **Consensus** — `core/processing/consensus.go`, a pure, table-tested function
  `Consensus(workers []WorkerResult) (entity.Extraction, float64, []string)`
  (canonical extraction, confidence, unresolved-field names). Identity fields =
  `{serial, brand, model}`. Let `s` = number of successful (parseable) workers:
  - `s == 0` → `OutcomeFailed`, per-worker errors in `Provenance`.
  - `s == 1` → canonical = that worker's extraction; **confidence = 0.6** (cap).
  - `s >= 2`:
    - per identity field: if two workers return different non-nil values → the
      field is **unresolved (nil)** and a conflict is recorded.
    - if **any** conflict → **confidence = 0.5** (cap).
    - else (all agree on identity) → **confidence = 0.9** (in [0.8, 1.0]).
    - non-identity fields (name, dates, price, currency, category, classification,
      metadata) are taken as the first non-nil value in a fixed worker order;
      disagreement on them does **not** reduce confidence.
   - The consensus confidence is written to `ext.Confidence`, so the **existing
     auto-commit gate** (`ext.Confidence >= PROCRASTINATOR_INGEST_REVIEW_THRESHOLD`,
     default `0.7`) is reused unchanged: ≥ 0.7 commits, else `OutcomeHeldForReview`
     (via `repo.Reviewer.Hold` extended with a `Provenance map[string]any` field
     that persists to the new `ingest_reviews.provenance` JSONB column, storing
     per-worker results + the candidate set; the existing `best_matched_asset_id`
     snapshot is preserved).

*Why:* parallel workers + a numeric consensus give the "dual consensus" the spec
demands and make per-worker failure a bounded degradation (one worker suffices at
the 0.6 cap) instead of an abort. Keeping the confidence gate as a single predicate
on `ext.Confidence` reuses the proven, tested commit/hold machinery. *Alternatives
rejected:* weighted voting / relevance scoring (over-engineered, out of scope —
non-goal); a single worker with a retry loop (not "parallel", no true consensus);
store a full candidate per worker (duplication).

### D3 — Extraction schema expansion + deterministic warranty computation

We will extend `entity.Extraction` with `Name *string`, `WarrantyDuration string`,
`AssetCategory *string`; `commons/parse/parse.go` parses them with the existing
permissive-absence rule (bad value → absent, never an error). `infra/llm/prompt.go
SystemPrompt()` is updated to (a) request the split **brand / canonical name /
model** (never the whole description in `model`), (b) request `asset_category` from
the vocabulary (D5), and (c) request `warranty_duration` as written ("2 years",
"24 months").

**Warranty arithmetic is done in code, never by the model.** New helper in
`commons/dates.go`:

```go
// AddWarrantyEnd returns (end, ok). If explicitEnd is non-nil it wins;
// otherwise purchase + parsed duration. Unparseable duration → ok=false.
func AddWarrantyEnd(purchase *time.Time, duration string, explicitEnd *time.Time) (*time.Time, bool)
```

with a duration parser understanding `N years|months|days`, `N yr|mo|d`, and
ISO-8601 (`P2Y`, `P1M`, `P2Y6M`). Precedence: explicit `warranty_end` > computed.
`"warranty 2 years"` + purchase 2026-01-15 → `2028-01-15`.

*Why:* the spec is explicit that date arithmetic must be deterministic ("rather
than relying on the model"). A pure, table-tested helper with explicit-end
precedence is the smallest correct mechanism. *Alternatives rejected:* let the LLM
return the computed end (non-deterministic, the exact defect being fixed); a
date-library dependency (stdlib `time` is sufficient for year/month/day addition).

### D4 — Known-brand lexicon + deterministic product-description splitting (consensus-time corrector)

We will add a **configurable known-brand lexicon** and a deterministic
**description splitter** applied at consensus time (D2), in
`core/processing/brands.go` + `core/processing/split.go`:

- **Lexicon** — an embedded, canonical brand list (`core/processing/brands.go`,
  e.g. "Cooler Master", "LG", "Samsung", "Whirlpool", "IFB", "Bosch", …) with
  case-insensitive, word-boundary, longest-match lookup; overridable via
  `PROCRASTINATOR_BRAND_LEXICON` (JSON path).
- **Splitter** — `SplitDescription(desc string, lex *BrandLexicon) (brand, name,
  model string)`: normalize (collapse whitespace, title-case), extract a lexicon
  brand (if present), take the trailing model-like token (contains a digit, e.g.
  `30BRC2`, `CD600`) as `model`, and treat the remaining middle tokens as `name`.
  `"MICRO WAVE OVEN CONVECTION 30BRC2"` → name `Microwave Oven`, model `30BRC2`;
  `"CABINET COOLER MASTER CD600 BLACK"` → brand `Cooler Master`, name `Cabinet`,
  model `CD600`.
- **Reconciliation** — the LLM is the primary splitter (D3 prompt); consensus
  then: validates/corrects `brand` against the lexicon, and if the returned `model`
  still looks like a whole description (≥ 3 words and a lexicon brand or a trailing
  model token is present), runs the splitter and overwrites brand/name/model. This
  guarantees the two canonical sample strings split correctly even if the model
  under-splits.

*Why:* the spec requires a "configurable known-brand lexicon consulted during
consensus" and forbids storing the whole description in `model`. A deterministic
corrector makes the two canonical examples testable without depending on LLM
flakiness, while the LLM remains the general-purpose splitter. *Alternatives
rejected:* pure regex on every field (brittle for real documents); a full NER
dependency (heavy, out of scope); trust the LLM alone (non-deterministic — the
defect being fixed).

### D5 — Intrinsic asset taxonomy replaces the asset's `doc_type`; document classification vocabulary expands

We will make `asset_category` an intrinsic property of the asset, decoupled from
document classification, and **remove `doc_type` from `assets`**:

- `entity.Asset` gains `Name *string`, `AssetCategory *string`,
  `CategoryConfidence *float64`, `CategoryUserSet bool`. `DocType` is removed.
- Category vocabulary (constants in `commons/entity`): `appliance`, `electronics`,
  `computing`, `furniture`, `vehicle`, `tool`, `clothing`, `document_only`, `other`.
- Document classification vocabulary (`documents.doc_type`,
  `ingest_reviews.doc_type`): expanded from
  `{invoice, warranty, amc, other}` to
  `{invoice, receipt, warranty, amc, statement, other}`.
- **Category inference** — `core/processing/category.go`: consensus prefers the
  LLM's `asset_category` (if in-vocabulary), else falls back to a deterministic
  keyword classifier on name/brand/description; `category_confidence` reflects
  agreement (0.9 agree / 0.6 single source / 0.5 none). `category_user_set` marks
  a user-corrected value (set by `PATCH`), which **must not** be overwritten by
  later extraction or merge (sticky).
- **Sticky category in identity merge** — `core/identity` `mergeInto` stops stamping
  `DocType` (removed) and, for `asset_category`, only overwrites when
  `!survivor.CategoryUserSet` (and an inferred value is present).

*Why:* the "asset type" field today is really the source document's classification
stamped last-write-wins — the exact defect. Splitting intrinsic category (what the
item *is*) from document classification (what the paper *is*) and adding a canonical
`name` fixes it. *Alternatives rejected:* keep `doc_type` and add `asset_category`
alongside (leaves the confusing field, violates "no field behaves as a document
type"); a free-text category (no vocabulary → no filtering/facets, violates the
spec's enumerated set).

### D6 — Efficient 3-stage indexed existing-asset lookup

We will extend identity resolution to a **deterministic 3-stage order**
(serial → brand+model → name+model), **bounded** per stage (default 10),
**indexed** so cost is independent of registry size:

- Migration adds `assets.norm_name` and indexes
  `(owner_id, owner_household_id, norm_brand, norm_model)` and
  `(owner_id, owner_household_id, norm_name, norm_model)`; `norm_serial` already
  has the unique index `uniq_assets_owner_norm_serial` (→ ≤1 candidate, O(log n)).
- `core/identity` `Resolve`/`Match` run stages in order, each `List(...,
  Limit(candidateLimit))`:
  1. `norm_serial = X` (unique → ≤1) — **definitive**; short-circuits.
  2. `norm_brand = B AND norm_model = M` (≤10).
  3. `norm_name = N AND norm_model = M` (≤10).
   The **first stage returning ≥1 candidate short-circuits** the rest. If a
   non-serial stage returns **exactly 1** candidate → merge into it (definitive).
   If it returns **>1** (ambiguous) → the processing module **holds for review**
   with the candidate set recorded in the new `ingest_reviews.provenance` JSONB
   column (safe default; reuses `repo.Reviewer.Hold` extended with a
   `Provenance map[string]any` field) rather than guessing.
- `candidateLimit` from `PROCRASTINATOR_LOOKUP_CANDIDATE_LIMIT` (default 10).
- **Soft-deleted assets are excluded** from matching (D8); if the only match is a
  soft-deleted asset, the caller is surfaced the deleted match (restore offer).

*Why:* the spec requires lookup "independent of registry size (no full-registry
scan)" and a bounded candidate set. Index-backed, bounded, ordered stages deliver
that observably (Y's lookup < 5× X's). *Alternatives rejected:* in-Go full-list
filter (O(n), the defect); `pg_trgm`/ILIKE for identity (fuzzy, out of scope);
a single "best" composite index (can't express the 3-stage fallback). *(The index
shape is a design decision per the spec; the requirement is observable latency +
bounded set.)*

### D7 — Content-hash dedupe (source-level + identity-level)

We will dedupe re-uploads by content hash at two layers, reusing the existing
`sources.sha256` (already computed in `infra/filestorage/storage.go:67`):

- **Source-level (early, in `Process` before extraction):** after the size check,
  query `sources` for the owner with `sha256 = H` (bounded). If found → this is a
  re-upload: load the linked `Document` (by `source_id`) and its `Asset`. Return
  `Outcome{Kind: OutcomeDuplicate, Duplicate: {SourceID, DocumentID, AssetID,
  AssetDeleted: asset.DeletedAt != nil}}`. **No LLM call, no new source stored.**
  If `AssetDeleted` is true the UI offers restore (asset-lifecycle scenario).
- **Identity-level (belt-and-suspenders, in `core/identity`):** when an extraction
  resolves to an existing asset and the new source's hash matches a document
  already linked to that asset → do not modify any asset field, do not create a
  second document; report the existing document as a duplicate.

*Why:* reprocessing a byte-identical document through the LLM is both wasteful and
the cause of duplicate assets (the core complaint). The source-level check is keyed
on the hash that already exists, so it is free and airtight; the identity-level
check covers any scope edge the source check misses. *Alternatives rejected:* a new
`documents.content_hash` column (redundant — `sources.sha256` is authoritative and
the document links a source); client-side dedupe (unreliable, not owner-scoped).

### D8 — Asset lifecycle: soft-delete + restore (retention) + purge; merge/dedupe

We will add lifecycle operations as a `core/lifecycle` service (owner-scoped,
failing closed without a bound user):

- **Soft-delete** — `DELETE` sets `assets.deleted_at = now()` (no row removal).
  List/search/get exclude soft-deleted (404 on direct get); `GET /assets` supports
  `include_deleted=true`. Owner-scoped: another owner's asset → 404, unchanged.
- **Restore** — clears `deleted_at`, but only within a configurable retention
  window (`PROCRASTINATOR_ASSET_DELETE_RETENTION_DAYS`, default 30); beyond it →
  409. Restore recovers all prior fields and document links.
- **Purge (MAY, implemented + tested, cron deferred)** — `PurgeExpired(ctx)`
  hard-deletes assets where `deleted_at < now() - retention` and sets
  `documents.asset_id = NULL` for their documents (links removed; **documents and
  sources retained**). Migration makes `documents.asset_id` nullable to permit
  this. Not wired to a scheduler in this change (spec marks purge "MAY").
- **Merge/dedupe** — `Merge(ctx, survivorID, duplicateID)`:
  - *Field strategy:* survivor keeps its identity; on conflict the survivor's
    non-null value wins; null-on-survivor/non-null-on-duplicate is copied over;
    `metadata` shallow-merged (survivor wins on conflict); a survivor with
    `category_user_set=true` keeps its category.
  - *Document preservation:* `UPDATE documents SET asset_id = survivor WHERE
    asset_id = duplicate` — every document linked to either asset appears on the
    survivor afterwards; none erased/orphaned/reclassified.
  - *Outcome + audit:* duplicate is soft-deleted with `merged_into = survivor` and
    `merged_at = now()`; it is excluded from list/search/get and not matchable by
    identity resolution; the survivor's detail lists the merged-in assets + when
    (audit query on `merged_into`, which bypasses the soft-delete filter for audit
    only).

*Why:* delete/merge are absent today and erode trust (duplicate assets can't be
removed or consolidated). Soft-delete + a restore window + a bounded purge is the
standard safe pattern; making the merge field strategy explicit (survivor-wins,
null-fill, metadata shallow-merge, sticky category) makes it deterministic and
testable. *Alternatives rejected:* hard-delete on `DELETE` (irreversible, loses the
restore + dedupe-after-delete flows); a separate `merge_audit` table (the
`merged_into`/`merged_at` columns on the duplicate are sufficient and
self-documenting); cascade-delete documents (violates "documents are retained").

### D9 — Unified `POST /add` endpoint + statement folding (uniform outcomes)

We will add one primary add endpoint `POST /api/users/{userId}/add` accepting
multipart file(s) and/or a text body, returning a **uniform per-item outcome**:

```
POST /add  ->  { items: [ { kind, asset?, review?, duplicate?, statement_preview?, reason? } ] }
kind ∈ { asset_committed, held_for_review, duplicate, statement_preview, failed }
```

Routing per item (auto-detected; the user never chooses a flow):
- **CSV / text that is clearly a statement** → straight to the existing
  `statement.Service.Upload(ctx, accountID, filename, data)` (import batch,
  preview state) → `statement_preview`. The add surface includes an **account
  selector** for statement items (the user picks which account the statement
  belongs to; required by `statement.Service.Upload`).
- **Any other file / pasted text** → `processing.Process`:
  - `OutcomeCommitted` → `asset_committed` (asset ref)
  - `OutcomeHeldForReview` → `held_for_review` (review ref)
  - `OutcomeDuplicate` → `duplicate` (existing doc/asset refs, `asset_deleted` flag)
  - `OutcomeStatement` (LLM classified a PDF/image as a statement) → call
    `statement.Service.Upload(ctx, accountID, filename, data)` on the retained
    source bytes → `statement_preview`
  - `OutcomeFailed` → `failed` (reason)
- **Pasted text** is stored as a `Source` (content type `text/plain`) before
  processing, so it flows through the identical pipeline as files.
- The existing `POST /documents` remains and **delegates to the same pipeline**
  (backward-compatible); statement import-batch endpoints (`/finance/import-batches*`)
  are unchanged.

*Why:* add is a primary action but is today fragmented (document upload vs
statement import). One entry point that auto-detects and returns uniform outcomes
makes add a first-class, predictable surface, and folding statements keeps the
ledger pipeline untouched. *Alternatives rejected:* keep two endpoints and hide
one in the UI (the user still "chooses a flow" — the defect); a new statement
pipeline (duplicates the ledger import; violates the non-goal).

### D10 — Enhanced search: structured filters + kind-labeled hits over the existing federation

We will extend the existing `core/search` + `repo.SearchBackend` federation with
**structured filters** combinable with free text, and keep the kind-labeled,
deterministic combined order:

- New `search.Filters{Category, Brand, PurchaseFrom, PurchaseTo, WarrantyStatus,
  HasDocuments, DocClassification}`. `WarrantyStatus ∈ {active, expired,
  expiring_within:<N>, unknown}` is computed in SQL from `warranty_end` vs `now()`.
- `repo.SearchBackend.SearchAssets` accepts the filters; the asset ILIKE fields
  **gain `name`** (so "microwave" matches canonical name "Microwave Oven"). The
  `SearchDocuments` query honors `DocClassification`. Filters combine with `q`
  (AND) and with each other; the existing type-priority concatenation + quick-is-a-
  prefix invariant is preserved.
- API: `GET /search` (and `/search/quick`) gain the filter query params; hits stay
  kind-labeled (`asset`/`document`/`movement`/…) and paginated.

*Why:* the spec requires filters (category, brand, date range, warranty status,
has-documents, doc classification) reflected in the URL and combinable with free
text. Extending the existing per-type custom queries is the only way to express
AND-of-filters + the document join while keeping D-8 visibility + RLS. *Alternatives
rejected:* client-side filtering (unscalable, not URL-shareable); a new search
index (out of scope — search stays ILIKE, a documented non-goal).

### D11 — OpenAPI + codegen stay additive with 1:1 lockstep

`api/openapi.yaml` gains (all additive or schema-level), and `make codegen`
regenerates both the Go server types and the UI client:

- **Operations:** `DELETE /assets/{assetId}`, `POST /assets/{assetId}/restore`,
  `POST /assets/{assetId}/merge` (body `{duplicate_asset_id}`),
  `PATCH /assets/{assetId}` (body incl. `asset_category`), `POST /add`, the
  `GET /search` filter params, and `GET /assets` / `GET /assets/{assetId}` gain an
  `include_deleted` boolean query param (default `false`; when `true` the
  soft-deleted row is returned).
- **Schemas:** `asset` gains `name`, `asset_category`, `category_confidence`
  (+ optional `deleted_at`, `merged_into`, `merged_at`, and a `merged_assets[]`
  audit array), **drops `doc_type`**; document `classification` vocabulary extended;
  new `add_request`, `add_item_outcome`, `merge_request`, `patch_asset_request`,
  search filter params. Every non-2xx references the standard `error` envelope.
- `make codegen-go` → `api/gen/openapi.gen.go` (new `ServerInterface` methods +
  DTOs + chi routes); `make codegen-ts` → `paths.d.ts`; `make codegen-ts-client` →
  orval per-operation client. `TestCodegenDrift`, `TestNewOperationYieldsScaffolding`,
  and the UI TS drift tests must pass.

*Why:* preserves the source-of-truth convention and the lockstep the drift tests
enforce; the UI stays OpenAPI-first. *Alternatives rejected:* hand-adding routes to
the generated file (breaks lockstep); a separate v2 contract (over-engineered; the
change is additive).

### D12 — Web platform: latest-stable stack (audit + pin) + shared TanStack-Table DataTable + consolidated primitives

We will bring the UI onto a verified latest-stable stack and consolidate its table
and primitive implementations:

- **Dependency audit** (task-gated artifact `ui/DEPS-AUDIT.md`): for each UI
  dependency record pinned version vs. latest stable upstream + a justification for
  any gap. Current majors are already on stable lines (React 19, Vite 6, Tailwind 4,
  TanStack Query 5, react-router 7) — the audit confirms and pins them; no
  pre-release/canary is used.
- **New dependency:** add **TanStack Table** (`@tanstack/react-table`) at its latest
  stable, lockfile-pinned — the "feature-rich data-table library".
- **Shared `DataTable`** (`ui/src/components/data-table.tsx`) built on TanStack
  Table providing **column visibility selection, multi-column sorting, and
  pagination**. It backs the assets list, search results, review queue, and
  statement preview; **hand-rolled per-page `<Table>` usages are removed**.
- **Consolidated primitives:** one select (the existing `components/ui/select.tsx`
  is the single pattern), one upload/dropzone (react-dropzone, single shared
  wrapper), one dialog (`components/ui/dialog.tsx` + the existing
  `confirm-dialog.tsx`). Duplicate/parallel implementations removed.
- **Simple-but-modern direction:** few clear surfaces; **Add** and the **search bar**
  are always visible in the app shell; no overlapping flows (single Add entry);
  consistent shadcn/ui-style tokens (typography/spacing), no page-specific ad-hoc
  styles.

*Why:* the UI today re-implements tables/selects per page on a stack that should be
pinned-current. A single shared, actively-maintained headless table + consolidated
primitives on a latest-stable stack is the smallest change that satisfies
"feature-rich tables", "one primitive per job", and "latest stable". *Alternatives
rejected:* a server-side-rendered table or a heavyweight grid (over-weight for the
surfaces); upgrading to canary majors (violates the stable-only rule); keep
hand-rolled tables (the defect).

### D13 — UI: unified Add surface + search as primary navigation

We will rework the UI surfaces to make add + search primary:

- **Unified Add** (`ui/src/pages/add/add-page.tsx` at `/add`): one dropzone (file)
  + one paste-text box; calls `useAdd`; renders each item's uniform outcome
  (asset / review / duplicate-with-restore-offer / statement-preview / failed) in a
  single summary; per-item navigation. The `/upload` route is removed (folded into
  `/add`); the nav has **one Add entry** (no separate "upload document" vs "import
  statement").
- **Search as primary nav** (`ui/src/components/search/**`, `/search`): a persistent
  search bar in the app shell with a **global keyboard shortcut** (e.g. `/` or
  ⌘/Ctrl-K) and full keyboard navigation; filter controls (category, brand,
  purchase-date range, warranty status, has-documents, doc classification) that are
  **reflected in the URL** (shareable); results rendered in the shared `DataTable`,
  **grouped/labeled by kind**, each hit navigating to its detail.
- **Asset list/detail rework** (`ui/src/pages/assets/*`): list in `DataTable` with
  **name + brand + model** as distinct columns (column visibility/sort/pagination);
  detail shows the item's canonical **name** alongside brand and model as separate
  fields, `asset_category` + confidence, the documents list (classification, source
  filename, upload timestamp), and **delete / restore / merge** actions plus
  **user category correction** (`PATCH`).
- **Data layer** (`ui/src/lib/api/hooks.ts`): new hooks `useDeleteAsset`,
  `useRestoreAsset`, `useMergeAsset`, `usePatchAsset`, `useAdd`, `useSearch(filters)`
  following the existing D4/D5 convention (query keys carry the active user id;
  mutations invalidate the named prefixes). `schema.ts` re-exports the new generated
  types; `client.ts` adds thin typed wrappers.

*Why:* add/search are the two primary actions but are today fragmented and
secondary. A single Add surface and a global, filterable, URL-shareable,
keyboard-first search bar make them first-class, and reworking list/detail to show
name/brand/model + lifecycle actions surfaces the new data model. *Alternatives
rejected:* keep `/upload` + a separate import page (the defect); client-side full-
text search (out of scope); a modal command-palette instead of a persistent bar
(the spec wants a prominent, always-visible bar).

## Data flow: the unified add / processing pipeline

```
 UI /add (file | text)                 UI /finance/import (ledger preview/commit, unchanged)
        │  POST /add (multipart/text)                                  ▲
        ▼                                                             │
  api.Add handler  ── per item, auto-detect ──────────────────────────┤
        │                                                            │
        │  CSV / obvious statement ───────────────────────────────────┤
        │  else: processing.Process(input)                            │
        ▼                                                            │
 ┌────────────────────────────────────────────────────────┐          │
 │ 1 size check (ErrTooLarge→413)                          │          │
 │ 2 content-hash dedupe: sources.sha256=H (owner) ?      │          │
 │      └─ yes → OutcomeDuplicate{doc,asset,assetDeleted} │          │
 │ 3 storage.Put + Sources.Create (retained on failure)    │          │
 │ 4 parallel workers (≥2) ──► per-worker Extraction       │          │
 │        + per-worker errors (errgroup, own timeout)      │          │
 │ 5 consensus: identity agree/conflict → confidence      │          │
 │        + brand-lexicon validate + desc-split + category │          │
 │ 6 warranty: explicit_end > purchase+duration           │          │
 │ 7 indexed lookup (serial→brand+model→name+model) ≤10   │          │
 │ 8 gate: conf≥0.7 & definitive match                     │          │
 │      ├ commit: identity.Resolve + Documents.Create      │          │
 │      ├ ambiguous / low-conf → reviewer.Hold            │          │
 │      └ classification=statement → OutcomeStatement     │          │
 └────────────────────────────────────────────────────────┘          │
        │ Outcome                                                    │
        ├ committed        → asset_committed                          │
        ├ held_for_review  → held_for_review (→ /ingest/reviews)      │
        ├ duplicate        → duplicate (restore offer if deleted)     │
        ├ failed           → failed(reason)                           │
         └ statement        → statement.Service.Upload(ctx, acctID,
                                filename, data) ─────────────────────┘
                                  └→ statement_preview (import batch)
```

## Risks / Trade-offs

- [Parallel workers double LLM cost/latency per document] → workers run
  concurrently (wall-clock ≈ one worker), each bounded by
   `PROCRASTINATOR_LLM_TIMEOUT`; the pipeline has a configurable end-to-end budget
   (default 30s, `PROCRASTINATOR_PROCESS_TIMEOUT`) — on overrun the document is
   **held for review with partial provenance** stored in
   `ingest_reviews.provenance` (per-worker results obtained so far), never dropped
   (processing-pipeline latency scenario).
- [Two workers disagreeing on a serial/model → the field is nil'd and the doc is
  held] → intended fail-safe (never auto-commit a guessed identity); the review
  queue is the recovery path; single-worker (one succeeded) proceeds at the 0.6 cap.
- [Consensus confidence replaces the LLM's self-reported confidence] → the gate
  (`≥ 0.7`) is unchanged and still fail-safe (absent/low → hold); the consensus
  value is stamped on the asset/document exactly as the old `ext.Confidence` was.
- [Accidental cross-owner leakage (lifecycle/merge/search span owned rows)] → reuse
  the existing D-8 `visibilityCond` + RLS backstop + membership trigger on every
  new query; merge/delete/restore are owner-scoped and fail closed (unknown or
  another owner → 404, unchanged). Isolation covered by the existing
  `infra/postgres/*_test.go` patterns + new DB integration tests.
- [`documents.asset_id` made nullable (for purge) weakens "every doc links one
  asset"] → only reachable after a soft-delete + retention + purge (a MAY path);
  live documents always link exactly one asset; purged assets leave their documents
  retained but asset-less (documented). The guaranteed-linkage invariant holds for
  all non-purged data.
- [Dropping `assets.doc_type` is a schema change] → additive migration is
  otherwise non-destructive; `doc_type` was `NOT NULL DEFAULT 'other'` and is fully
  replaced by `asset_category` + the document's own `doc_type`; data is MVP
  (not real). Down migration restores the column.
- [UI table/primitive consolidation touches every list surface] → done as focused
  per-surface tasks that each keep their tests green; the shared `DataTable` is
  built and tested first, then adopted surface-by-surface; a hand-rolled-`<Table>`
  lint/grep gate catches stragglers.
- [Statement detection for PDFs depends on LLM classification] → CSV is routed
  deterministically (no LLM); a mis-classified statement simply yields no asset
  (held/review), not a corrupt ledger row; the ledger import pipeline is untouched.
- [Brand lexicon is incomplete for arbitrary brands] → the LLM is the primary
  splitter; the lexicon only *validates/corrects* known brands and the deterministic
  splitter handles the trailing-model case; unknown brands degrade gracefully (no
  brand), never a wrong split.

## Migration Plan

1. **Schema** — deploy `00006` (additive + one column drop + one nullability
   change), applied at startup via `Store.Migrate` (goose) and per private schema in
   the DB integration/e2e suites. Up: add `assets.{name, norm_name, asset_category,
   category_confidence, category_user_set, deleted_at, merged_into, merged_at}`,
   drop `assets.doc_type`, expand `documents.doc_type` + `ingest_reviews.doc_type`
   CHECKs to the 6-value vocabulary, make `documents.asset_id` nullable, add
   `ingest_reviews.provenance jsonb NOT NULL DEFAULT '{}'`, add the
   `norm_brand+norm_model` / `norm_name+norm_model` lookup indexes. Down reverses
   each step.
2. **Code** — `core/ingest` → `core/processing` (+ `core/lifecycle`); `api` gains
   delete/restore/merge/patch/add + search filters; OpenAPI + codegen regenerate
   both server and client. Behavior change: single-LLM → parallel+consensus (the
   intended change); re-uploads now dedupe; the asset's `doc_type` is gone.
3. **UI** — dependency audit + TanStack Table; shared `DataTable` adopted across
   surfaces; `/add` + global search; `/upload` removed.
4. **Rollback** — `00006` has a Down; rolling back code while keeping the schema is
   safe (old code ignores the new columns; `doc_type` is restored). Residual: assets
   already soft-deleted/merged have no path in the old code — acceptable for MVP.

## Open Questions

- **Purge scheduling.** Purge logic is implemented + tested but not wired to a
  cron (spec marks it "MAY"). Record: defer scheduling to a follow-up; a manual
  admin trigger suffices for MVP. Not a blocker.
- **Cycle-2 delta (user-directed).** All entities store data in a jsonb column
  (D14); migrations are rebuilt from scratch (D15); API schemas mirror the
  entity structure (D16); plus landing (D17), framework pass (D18),
  modularization (D19), documents section (D20), duplicate prompt (D21),
  dark mode + UX test-plan gate (D22). Supersedes the cycle-1 Migration Plan
  section and D11's asset-schema clause insofar as stated below.

- **`PROCRASTINATOR_LLM_WORKERS` default.** Default to two workers on the existing
  `PROCRASTINATOR_LLM_MODEL` (strategies `extract`/`verify`). If a second model is
  later configured it slots in with no code change. Reviewer to confirm the
  same-model dual-worker default satisfies "parallel models".
- **Search scope for the reworked bar.** The reworked search keeps the existing
  five-type federation (asset, account, movement, document, import_batch); the
  spec's filters primarily constrain the asset/document streams. No account/
  import-batch filter is added. Recorded for visibility; not a blocker.

---

# Cycle-2 design delta (D14–D22)

User-directed for this cycle: (a) **jsonb entity storage for ALL entities**;
(b) **migrations rebuilt from scratch** (WIP, no data); (c) Postgres jsonb
indexing is acceptable and relied upon; (d) **API schemas mirror the entity
structure** (no unnecessary flattening). Entity column splits were decided by
the designer from actual query patterns (RLS scoping, lookup stages, search
filters, sort orders, FK joins) and gated with the user via the question tool;
the user confirmed: real columns limited to "common basics" — key columns,
audit info (created_at, updated_at, deleted_at) — and the jsonb column always
carries the full entity even where a few basics are also real columns;
created_at/updated_at are maintained by a database trigger.

### D14 — Jsonb entity storage pattern for every entity table

**Rule.** For every entity table the real columns are ONLY:

1. **Key columns** — the PK (single `id uuid` or composite), and the FK /
   tenancy columns that must be real to scope or join the row: `owner_id`
   (FK users, NOT NULL, RLS), `owner_household_id` (FK households, nullable),
   and direct parent FKs that key child rows
   (`documents.asset_id`, `documents.source_id`,
   `money_movements.source_account_id` / `destination_account_id` /
   `linked_document_id`, `import_batches.account_id` / `source_id`,
   `import_lines.batch_id`, `households.owner_id`,
   `household_members(household_id, user_id)` composite PK).
2. **Audit columns** — `created_at timestamptz NOT NULL DEFAULT now()`,
   `updated_at timestamptz NOT NULL DEFAULT now()` (all tables,
   trigger-maintained), `deleted_at timestamptz` (assets, documents;
   soft-delete audit, real because lifecycle restore/purge and every
   list/search exclusion filters on it directly).

Everything else lives in **`data jsonb NOT NULL DEFAULT '{}'`**, which always
carries the **complete entity payload** (duplicated even for the few basics
that are also real columns — except `created_at`/`updated_at`/`deleted_at`,
which are audit-only and never meaningfully in `data`; avoid divergence).

Per-entity payloads (columns REMOVED in favor of data):

| table | stays real (key+audit) | moves into `data` |
|---|---|---|
| users | id, created_at | (empty today) |
| households | id, owner_id + audit | display_name |
| household_members | (household_id, user_id) + created_at | — (no payload fields today; **the one exception** to the universal `data` column rule — an empty column would be pure ceremony) |
| sources | id, owner_id, owner_household_id + created_at | filename, content_type, byte_size, storage_path, sha256 |
| assets | id, owner_id, owner_household_id, deleted_at + audit | brand, model, serial_number, norm_serial, norm_brand, norm_model, purchase_date, warranty_end, price, currency, name, norm_name, asset_category, category_confidence, category_user_set, metadata, merged_into, merged_at |
| documents | id, owner_id, owner_household_id, asset_id, source_id, deleted_at + audit | doc_type, extracted_fields, raw_extraction |
| ingest_reviews | id, owner_id, owner_household_id + audit | doc_type, payload/raw extraction, confidence, best_matched_asset_id, candidate set, provenance |
| financial_accounts | id, owner_id, owner_household_id + audit | name, account_type, currency, institution, external_descriptor |
| money_movements | id, owner_id, owner_household_id, source_account_id, destination_account_id, linked_document_id + audit | kind, amount, currency, occurred_on, description, norm_description, origin, import_batch_id, import_line, external_reference, link_creator, link_conflicting |
| import_batches | id, owner_id, owner_household_id, account_id, source_id + audit | state, filename, format, line_count_* |
| import_lines | id, owner_id, owner_household_id, batch_id + created_at | line_ref, raw_line, occurred_on, amount, direction, description, norm_description, external_reference, status, error_reason |

**Indexing (validating the user's premise).** Lookup, uniqueness and filters
are provided by expression / functional indexes over `data`:

- Identity lookup (D6 unchanged): functional indexes
  `ON assets((data->>'norm_serial')) WHERE ...` plus composite
  `(owner_id, (data->>'norm_brand'), (data->>'norm_model'))` and
  `(owner_id, (data->>'norm_name'), (data->>'norm_model'))`; uniqueness on
  serial via a **functional unique index** (replaces the real-column unique).
- `sources` dedupe: expression index `(owner_id, (data->>'sha256'))`. The
  sha256 dedupe lookup (D7) is a **single-owner-scoped equality query**
  (`sha256 = H`), for which a jsonb expression index is functionally
  equivalent to a real-column index. No range scan or IN-list is needed on
  this path, so the expression-index form is sufficient.
- Uniqueness that relied on a real column (`documents.source_id`,
  `money_movements.linked_document_id`) keeps a plain unique index because
  those columns stay real (FK columns). `import_lines` uniqueness becomes a
  functional index `(batch_id, ((data->>'line_ref')))` replacing the
  `(batch_id, line_ref)` real-column unique.
- Search ILIKE filters (D10) target expressions such as
  `(data->>'brand') ILIKE …`; plain b-tree/expression indexes preserve the
  current ILIKE behavior (search stays ILIKE, a documented non-goal).
- FK/tenancy indexes re-created on the (unchanged) real columns; RLS policies
  (D-8) are unchanged in shape — they filter on the same real key columns.

**Go mapping.** Entity structs in `commons/entity` keep one canonical struct
per entity whose data-payload fields carry `json` tags; the whole struct minus
the audit fields is what `data` holds. `infra/postgres` stores/loads rows by
`json.Marshal`/`json.Unmarshal` of the payload into a row struct
(`id`, key/audit columns, `data json.RawMessage`); a per-entity marshal/unmarshal
helper pairs `entity.X` ↔ row. Repositories stay type-parametric
(`repo.Repository[T]`); their `Filter` set mechanically rewrites to the
expression predicates above. Concretely, the generic predicate engine lives in
`infra/postgres/repository.go` (`addFilterCond`/`validateFilters`/`validOps`/
`pgRepository[T]`), and the per-entity filter whitelists (assetFieldCols,
accountFilters, movementFilters, importBatchFilters, importLineFilters,
householdFilters, reviewFilters) plus the search SQL (`SearchAssets` in
`repos.go`, `SearchAccounts`/`SearchMovements`/`SearchImportBatches` in
`finance.go`, `SearchDocuments` in `finance_queries.go`, with the
`searchBackend` forwarder in `factory.go`) live in
`infra/postgres/{repository.go,repos.go,finance.go,finance_queries.go,
household_repo.go,review_repo.go,factory.go}` — these are
the files the cycle-2 delta task 10.7 rewrites (there are no
`generic.go`/`filters.go`/`search.go` files in `infra/postgres`).
API DTO mapping disappears where the contract
mirrors the entity (D16): the entity payload serializes as-is.

**updated_at trigger.** A single trigger function `touch_updated_at()`
(`updated_at = now()`) is applied by the baseline to every table with
`updated_at`; `created_at` defaults to `now()`. The Go layer never sets audit
timestamps.

*Why:* this is the user's directed storage model. Key+audit columns give
RLS, FK joins and lifecycle filtering a stable, typed surface; everything
project-shape-specific (taxonomy, extraction payloads, provenance, import
line fields) evolves inside jsonb without migrations; expression indexing
covers the bounded lookup and search workloads. *Alternatives rejected:*
hot filter columns as real columns (user overrode: keep only common basics);
CHECK constraints over jsonb (not supported in the baseline style —
validation happens in Go vocabulary validators, which already exist).

### D15 — Fresh consolidated baseline migrations (from scratch)

The six existing migrations (`00001_identity`…`00006_asset_rework`) are
**deleted** and replaced by a fresh, logically split baseline:

- `00001_identity.sql` — users, households, household_members (identity
  tables keep key-only real columns + `data` for the payload fields), seed
  test users (`test-user`, `test-user-b`).
- `00002_entity_tables.sql` — the eight entity tables above in the D14
  shape, expression/functional indexes, functional unique indexes, FK
  constraints, the `touch_updated_at()` trigger function + trigger binds.
- `00003_row_level_security.sql` — RLS enable + policies per table (same
  D-8 control set re-applied, same membership trigger).

Down migrations drop everything in reverse. The codegen/e2e schema-gate
("schema v6") is updated to the new top version (3). MVP data means no data
migration, no hand-off, preserved compatibility with prior schema.

*Why:* WIP project, no data to preserve; a from-scratch baseline eliminates
the accreted 00006 delta and makes the jsonb rework the only schema reality.
*Alternatives rejected:* keep 00006 as an additive (leaves two shapes in
history; contradicts the direction); one mega-file (user chose a small set of
logically-split baselines).

### D16 — API mirrors the entity structure (no unnecessary flattening)

OpenAPI request/response schemas are kept/restructured so each entity surface
mirrors the entity payload's natural structure — nested sub-objects stay
nested (`asset.metadata`, `asset_category`, …; `document.extracted_fields`,
`document.raw_extraction`; review includes `provenance` and the candidate
set), and no query-row-style flattening DTOs are introduced. Where an API
needs a filter or a path parameter it stays a query/path param (HTTP
mechanics), but the **body** shapes equal the entity payloads. Cycle-2
additions (D20/D21) follow the same rule.
**Documented exception:** the `merged_assets[]` audit array on the asset
response is a **computed field** — derived from a query on `merged_into`
across other assets — not part of the entity payload. It is the one
exception to the body-shapes-equal-entity-payloads rule; the API layer adds
it on top of the mirrored payload (it is not denormalized into `data`).
`make codegen-go` /
`codegen-ts` / `codegen-ts-client` regenerate; drift tests keep the
lockstep. (Restates and sharpens D11's schema clause.)

### D17 — Landing experience (root, never a list)

- Routes: `/` renders `features/landing` (search bar top, big [+] center,
  Insights placeholder bottom); `/assets`, `/assets/:id`, `/documents`,
  `/finance/*`, `/search`, `/ingest/reviews` remain specific views on their
  own routes; returning to `/` restores the landing state.
- **Hamburger-only shell.** The app shell renders a top bar with the
  hamburger button on every screen; the hamburger opens a navigation sheet
  containing nav links + the active-user control + the theme toggle.
  **No permanent sidebar on any screen** — the previous shell's persistent
  rail/side bar is removed.
- **Add picker.** [+] reveals exactly 📷 Camera / 📎 Upload / ✎ Text
  (≥44px touch targets at 360px); no document-type or account selection is
  requested before input. Camera (mobile capture) and Upload feed the same
  `useAdd` file path; Text opens the paste box.
- **Companion text.** Upload/Camera flows include an optional companion-text
  field; each (file + text) pair is submitted as **one add item**
  (text = additional processing context, passed to extraction as a note —
  the D20 reprocess shares the same mechanics). Text alone is an item.
- **Deferred account selection.** When an item is detected as a statement
  without an account, the backend yields a new per-item outcome
  `statement_pending_account` carrying the retained source reference;
  the UI prompts for the account at that step and calls the new
  `POST /sources/{sourceId}/import-statement` endpoint; non-statement items
  never ask. (Restates/extends D9's account-selector rule — no pre-selection.)
- **Insights.** Labeled placeholder section, intentionally empty; no data.

*Why:* the spec fixes the landing contract (never a list, fixed layout
order, no pre-selection, deferred account); building it from the existing
Add/search/search-filters primitives keeps one pipeline per input method.
*Alternatives rejected:* landing = the assets list (the defect); account
selector up-front (the defect the spec calls out).

### D18 — Framework pass: adopt the deferred newer majors

The cycle-1 audit held six newer majors as justified gaps. This cycle the
audit re-runs **at implementation time** and adopts them (with re-verification
per package): Vite 8, `@vitejs/plugin-react` 6, TypeScript 7, vitest 5,
jsdom 30, `@testing-library/jest-dom` 7 — keeping Vite + React 19 +
react-router 7 (SPA-over-Go-API decision stands, incl. confirming the SvelteKit
switch is not warranted) at their current stable pins, Tailwind 4.x line,
TanStack Table v9 line, shadcn CLI line at current stable. `ui/DEPS-AUDIT.md`
is rewritten with the swap-vs-majors rationale ("keep, upgrade majors") and the
final resolved pin set; no pre-release/canary.

### D19 — Modularization: api/ domain subpackages + UI feature folders

- **Backend.** Handler files under `procrastinator-backend/api/` split into
  domain subpackages `api/assets`, `api/add`, `api/search`, `api/documents`,
  `api/reviews`, `api/finance`, `api/lifecycle` (mirroring where the surface
  warrants it). The root keeps `api/cmd`, `api/gen`, `api/openapi.yaml`,
  and server composition/route registration. Handlers in subpackages
  implement the same strict `ServerInterface` methods as today; the
  OpenAPI contract stays the source of truth and drift/codegen
  tests still gate.
- **core/ boundaries verified** (not re-structured): `identity`,
  `processing`, `lifecycle`, `search`, `review`, `statement`, plus the new
  `documents` (D20). Recorded as package responsibility statements +
  evidence from the tests that pin behaviors; a lightweight test (go test)
  guards the boundary rule ("domain logic stays in `core/<domain>`" — no
  cross-domain behavior dumped into sibling packages or handlers).
- **UI.** `ui/src/pages/*` and their hooks/components reorganize into
  `ui/src/features/<feature>/` (`add`, `search`, `assets`, `reviews`,
  `finance`, `documents`, `landing`) with co-located feature hooks and
  feature components; `ui/src/components/` keeps shared primitives and the
  shell/design system only. Route bindings point into the feature folders.

*Why:* matches the code-organization spec; bounded domain structure keeps
the rework maintainable while preserving behavior and lockstep codegen.

### D20 — Documents section + reprocess/edit/delete actions

- **core/documents service** (new `core/documents/`): owner-scoped
  list joining `sources` → `documents` → `ingest_reviews` → `assets`, each
  row carrying `source_id`, label (filename or text label), `uploaded_at`,
  computed **status** ∈ `processed | in_review | failed | asset_less`
  (processed = document row exists and its `asset_id` resolves; `in_review` =
  a review row exists for the source; `failed` = source retained, no
  document/review row; `asset_less` = document row whose `asset_id` is gone),
  and the linked asset (id + display name). Supports `status` and free-text
  filters; hard fail-closed owner scoping unchanged.
- **Reprocess.** New `processing.Reprocess(ctx, sourceID, note string)`
  re-runs the pipeline against the **retained source bytes** (no new source
  row for the note: the note flows into extraction as an additional context
  like companion text and is recorded in `ingest_reviews`/document
  provenance); it respects the consensus gate as a normal run (ambiguous /
  low-confidence → hold), keeps the existing document↔asset link on a
  commit, and updates fields per the standard merge rules (sticky user-set
  category — D5). Reprocess catches the duplicate scenario: if the run
  commits against the *same* asset identity it updates in place; it does
  not create a duplicate asset.
- **Document soft-delete/restore.** `DELETE /documents/{documentId}` sets
  `documents.deleted_at` (excluded from asset document lists, search, and the
  Documents view by default); a restore operation clears `deleted_at` within
  the same retention window as assets (retention config reused). Source bytes
  are never deleted.
- **Reprocess an asset.** `POST /assets/{assetId}/reprocess` iterates the
  asset's linked documents, invoking reprocess per document (or until the
  user cancels), applying the same rules — the asset survives, identity and
  linkage preserved, sticky fields intact.
- **Extended asset edit.** `PATCH /assets/{assetId}` widens to the
  structured core (name, brand, model, purchase_date, warranty_end, price,
  currency, metadata…) under the user-set sticky semantics: values set by
  a user PATCH are marked sticky and are never overwritten by later
  extraction/merge.
  **Sticky-field generalization (additive, not a rewrite).** Because the
  migrations are built from scratch (D15), the asset payload carries a
  `user_set_fields` set (a list of field names present on the asset
  payload) from day one; D5's `category_user_set` boolean is generalized —
  a user-set category is expressed as `"asset_category" ∈ user_set_fields`.
  Merge/identity logic (D2/D5/D8) checks membership in `user_set_fields`
  instead of the boolean; cycle-1's sticky-category behavior
  (tasks 3.2 / 5.2) is preserved as the single-member special case. The
  `core/lifecycle` files implementing the merge rules are **re-owned** by
  the cycle-2 sticky task (see tasks 10.6→10.10) — cycle-1 tasks 5.1/5.2
  are superseded on those files only for this membership swap.

### D21 — Duplicate reprocess prompt (UI semantics; no new backend op)

The add outcome `duplicate` already carries the existing source/document/asset
references (D7) with `asset_deleted`; the backend therefore needs **no new
endpoint** for the prompt itself. The UI, on receiving a `duplicate`
outcome item, renders a prompt — the existing file name, the linked asset
display name, and two actions: **Reprocess** (calls the D20 reprocess
endpoint with the `source_id`, optionally with a comment) and **Keep
existing** (a no-op; dismisses). The system never silently marks a
duplicate: the prompt is rendered for every `duplicate` item in the add
summary. When `asset_deleted=true`, the prompt also offers the asset
restore action (cycle-1 restore offer preserved) as a third option.

### D22 — Dark mode + UX test plan gate

- **Dark mode.** A theme toggle (light / dark / system) lives in the
  hamburger/navigation area (and on the landing top bar), implemented via
  Tailwind v4 `@custom-variant dark` + shadcn token classes, persisted
  (localStorage/system-following), applied **flash-free** (first-paint
  inline class strategy, no hydration flash); toggling is instant, no page
  override regressions.
- **Mobile-first quality bar.** Touch targets ≥44px, cards over tables at
  ≤360px widths (already established 7.4), tokens per component
  primitives, no page-level horizontal scroll at either width; the existing
  single-DataTable + shadcn primitive rules continue to apply.
- **UX test plan (gating).** The cycle-2 browser-driven E2E pass — covering
  mobile (360px) + desktop, landing flows (camera/upload/text,
  ± accompanying text), search-as-nav from the root, dark mode on/off,
  Documents section (statuses + per-row actions), reprocess/delete/edit
  flows, the duplicate reprocess prompt, and the snappy/native-feel bar —
  is the final, sign-off-gating task (cycle-2 11.8); the workflow does not
  close until it passes. A defect found by the plan is fixed in the owning
  task's files and the plan is re-run.
