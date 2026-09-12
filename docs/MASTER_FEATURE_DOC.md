# Life Manager — Master Feature Doc (LIVING)

> **Status:** Resolved. After 3 architecture review passes + reviewer gate
> (cycle-1 FAIL findings closed 2026-09-11), the architecture below is
> **resolved**. Review → resolve → then this
> becomes the source for OpenSpec changes.
>
> **Last updated:** 2026-09-11 (architecture fully resolved after 3 architect passes + reviewer gate)
>
> **Model today:** React/Vite UI (`ui/src`) + Go backend (`procrastinator-backend`,
> Postgres with per-user RLS). Search = ILIKE substring only. No auth layer yet.
>
> **Status of resolved decisions:** All architectural decisions below have been
> resolved and review-gated. See §5 "Resolved Architectural Decisions" — treat
> that section as authoritative over any conflicting statement elsewhere.

---

## 0. What's True in the Code Today (verified)

These are facts, not opinions. Everything below builds on them.

### Domain model (backend `commons/entity`)
- **`Source`** = one uploaded file row. Fields: `Filename`, `ContentType`, `Size`, `Path`, `SHA256`, `UploadedAt`. The **original file + name are stored verbatim** at `{storage_dir}/{owner_id}/{uuid}.{ext}`.
- **`Document`** = one extraction/processing row, 1:1 with a `Source`. Fields: `SourceID`, `AssetID`, `DocType`, `ExtractedFields` (`map[string]any`), `RawExtraction` (full LLM JSON, unmodified), `Confidence`, `UserDirective` (the optional ingest note), `OwnerHouseholdID`, `DeletedAt` (soft delete), `PendingChoice` (duplicate-upload pending state).
  - `DocType` is a **6-value enum**: `invoice, receipt, warranty, amc, statement, other`. **No identity type.**
- **`Asset`** = derived product record: `Brand, Model, Name, SerialNumber` (+ normalized `norm_*` columns), `PurchaseDate, WarrantyEnd, Price/Currency, Metadata`, `AssetCategory` (9-value enum incl. `document_only`, `other`), `Confidence`, `CategoryUserSet` (user-set categories are sticky), soft-delete w/ retention.
- **Finance**: `Account`, `MoneyMovement` (has **`LinkedDocumentID`** — a real, live typed column; see the MONEY-MOVEMENT EXCEPTION in §5.1), `LinkCreator`, `LinkConflicting`, `ImportBatch`, `ImportLine`, `ImportSource`.
- **Search hit** (`Hit`): `Type, ID, Title, Subtitle, Confidence`. No preview/image/text blob.
- **Note (resolved):** the resolved model (§5.1) embeds these entities' *link*
  fields as structured JSONB inside each entity's payload for **new/derived**
  links. Typed Go struct fields like `Document.AssetID` therefore become
  payload-backed fields maintained by each entity's typed repository accessor
  (deprecated per §5.8). **`MoneyMovement.LinkedDocumentID` is NOT collapsed** —
  it remains a real typed column today and stays (MONEY-MOVEMENT EXCEPTION, §5.1).

### Search (backend `core/search`)
- Joins 5 entity types: **assets, accounts, movements, documents, import_batches**.
- **Pure substring** (`%q%` ILIKE), metacharacters escaped. **No fuzzy, no vector, no semantic.**
- Type-priority ordering: assets → accounts → movements → documents → import_batches.
- Quick (top-N) + Paged endpoints. Query capped at 200 chars.
- **Note (resolved):** this hardcoded 5-join union is retired by the provider-registry refactor (§5.4).

### Identity resolution (backend `core/identity`)
- `Resolve`/`Match`: deterministic **3-stage indexed lookup** — `norm_serial` → `norm_brand+norm_model` → `norm_name+norm_model`. Soft-deleted never match; ambiguous → held for review; merges into existing or creates new. **Asset-scoped only** (serial/brand/model).

### Ingest (UI `features/docs/composer`, `features/landing`)
- **`AddComposer`**: ONE unified surface — upload (drag/drop/file picker), camera, ONE optional free-text note. Text-only path wraps the text as a `pasted.txt` document (`client.addItems`). File path returns `committed` | `held` | `duplicate`.
- **No memory-from-text path** exists. Text notes are not stored as a separate entity.

### UI chrome
- **`AppShell`**: ☰ is fixed **top-left**; nav is a **sheet inside the ☰**. No cross-side sidebar, no back/home button anywhere. The "sheet opens from the top-left ☰, left-aligned with it" detail is a **Resolved target design — NOT yet built in code** (§5.5): today the live code still renders the nav sheet on the right (`nav-sheet.tsx` `side="right"`); moving it left is the Quick Fix #2 build work — see the §8 delta note.
- **`add-button.tsx`**: fixed top-right **per-section** `[+]` — each section page supplies its own label/flow (differs by screen by design).
- **Landing (`/`)** = home: hero wordmark, large search bar, centered `[+]`, disabled **Insights** section, disabled chat pill.
- **`documents-page.tsx`**: raw TanStack table (filename, uploaded, status badge, asset link, "…" actions). Statuses: `processed, in_review, failed, asset_less`.
- Router: **7 reachable routes** only (`/`, `/search`, `/ingest/reviews`, `/assets`, `/assets/:id`, `/documents`, `/finance/accounts`). Subscriptions, relationships, inventory, tasks, memory are **absent** (disabled/TBD).
- **No auth** — `user.UserFrom(ctx)` is context-only; accounts scoped by active-user context.

---

## 1. Guiding Principles (agreed)

1. **Docs are the source of truth.** A `Source`+`Document` is the root. Assets, movements, memories, persons, facets are **inferred + linked**, never duplicated. A doc may yield **0, 1, or many** facets.
2. **Originals are preserved.** Never overwrite the original filename/content.
3. **Search is the entry point**, not a screen. It should be intelligent, cross-entity, and understand unstructured metadata.
4. **The `[+]` is unified** — one add menu, behaves the same on every screen.
5. **Top chrome is constant** — always `[☰][search][+]`. Nav lives inside the ☰ sheet (opens from the top-left ☰ — §5.5). Home = the chat + insights landing screen.

---

## 2. The Features

### 2.1 Document-manager posture (#3) — *early*
- Keep `Source` as the permanent home of the original file + name.
- Never rename/rewrite originals; derived data lives on `Document`/facets.
- **Near-future ingestion sources:** abstract ingest so it can pull from a directory or object storage (S3/gCS) asynchronously. `core/processing` is already async-capable — extend the job/source layer.
- Deletion: deleting a source/doc **asks** whether to also delete linked facets (prompt; default off; never auto-delete). Delete behavior for **every** entity's linked/pointing payloads (Person, Memory, Contact, Asset, Source, Document) is defined by the **deletion matrix** (§5.1 guardrail 3) — each entry gets a WHEN/THEN scenario (e.g. delete asset → clean `assetids[]` from pointing memories/contacts).

### 2.2 Always upload even with no data (#4) — *early*
- A doc with no extractable fields is still saved as `Source`+`Document` (already the case via `asset_less`/`failed`).
- Surface these clearly; keep them **fully searchable** (filename + `RawExtraction` full text).

### 2.3 Memory (#10) — *early*
- New **Memory** entity: independent, type-tagged (`note`, `custom`…), links embedded in its JSONB payload (see §5.1, MONEY-MOVEMENT EXCEPTION).
- Sources: the `[+]` text, chat, and LLM-suggested.
- CRUD page; **vector-embedded** like docs.
- De-dupe by key, **per type** (resolved: ID number for person identity data, name+kind for notes — see §5.2). Structured identity/expiry data (ID number, name, expiry) belongs to **Person** (§2.4), not to a typed Memory. For such records a newer record **supersedes** an older one — see §2.4 / §5.2. Free-text notes stay as Memory rows.

### 2.4 Identity documents (#11 → #15) — *early*
- RESOLVED (2026-09-11, user decision D1): identity docs are identified by **tags**, NOT a `doc_type=identity` enum value — no ever-growing enum. Two tag kinds: key-value tags (`id_number=…`, `name=…`) and value-only tags (`passport`, `driving_license`, `pan`); the vocabulary is open-ended, assigned by extractor and/or user. Tags live on the Document payload per §5.1 and are searchable (vector-embeddable like other entity text; key/value-structured). Person derivation from a tagged doc is gated by the modular confidence pipeline (§5.10, decision D2).
- Extraction of ID number, name, expiry → **Person** (first-class structured entity; version-chain model — see below) **only when the §5.10 confidence pipeline crosses the derive threshold**. Below threshold: tagged Document only; below the lower hold threshold: review queue. Free text goes to **Memory**. Two writes, two contexts.
- **Person version-chain (resolved, §5.2):** a renewal (e.g. passport renewal) creates a **NEW Person row**; the earlier row is marked `superseded_at` (+ `superseded_by_id`) and is **NEVER deleted** — lineage is always preserved. Uniqueness is enforced with a partial unique index on `key_hash WHERE superseded_at IS NULL`. There is no "newer replaces/deletes older": superseded rows remain queryable history.
- Person is the LLM's source of truth for name/ID/expiry (feeds #15).
- The dedupe/uniqueness machinery reuses the **pattern** from `core/identity` (index + hash lookup + ambiguity holding) but is implemented as a dedicated Person-scoped helper — `core/identity.Resolve` itself stays Asset-scoped.

### 2.5 Intelligent multi-entity search (#1 + #5 + search-enrichment) — *early*
- Search across **all** entity types, including: docs, assets, movements, accounts, memories, persons, and **unstructured metadata** (`Document.ExtractedFields` + `Extraction.Metadata` — the snake_case JSON blob ≤8KB with untyped values, which "does not have fields to use" but is searchable text).
- **Mechanism (agreed): "all of it, with priority + config."** (resolved details in §5.4)
  - **Postgres hybrid stack:** `tsvector` (keyword + ranking) + `pg_trgm` (fuzzy) + `pgvector` (semantic) — embeddings only where they earn their keep, chunked for long OCR text.
  - **Embedding scope:** embed full extracted text / notes / JSONB-as-text in **256–512-token chunks** into a child table; keep a cheap document-level aggregate embedding for "find similar docs." Do NOT embed filename (structured) or raw OCR blobs wholesale.
  - **Hybrid ranking:** start with **RRF** (reciprocal rank fusion) — details resolved in §5.4 (global fusion, k=60; the type priority list is a tie-break precedence, not a boost); graduate to weighted sum only when type-boost control is needed.
  - **Fuzzy:** `microwave` should match `micro wave` — `pg_trgm` for intra-word typos; tokenized `tsvector` + synonyms for cross-token spelling variants.
- Result card = `[type] name + thumbnail/placeholder thumbnail`; click opens the appropriate page (document → doc detail, asset → asset detail, person → memory, etc.).
- **Verified gap today:** no fuzzy, no vector, no unstructured-metadata search, no memory/person type yet.

### 2.6 Facet / link model (#16) — *early* (build order step 3, §6 — foundation for memory + identity docs, not late scope)
- Generalize the doc→facet relationship. Today `Document.AssetID` is a **single 0..1 link**. Need **doc → 0..n linked items** (assets, movements, memories, persons).
- **Resolved (§5.1):** links on new/derived entities are denormalized as structured fields inside each entity's JSONB payload (`asset.payload.assetids[]`, `memory.payload.contactid`, `memory.payload.assetid`, `contact.payload.assetid`, …). Zero FK columns or link tables for new entities; `MoneyMovement.LinkedDocumentID` stays typed (MONEY-MOVEMENT EXCEPTION, §5.1). Reverse lookup is a JSONB-path scan accelerated by a GIN index within the quantified ceiling (§5.1), plus typed repository accessors with mandatory OCC.
- Doc detail page: show **all linked items**, open any of them, **download / view** the original doc, **thumbnail** per doc/asset.
- The current "actions" model (single "Open asset" per row) is wrong for 0..n facets.

### 2.7 Detailed review queue (#14) — *early*
- Per item: view the original doc, view inferred info (editable), **approve / decline**, plus **add comment** + **reprocess** buttons.
- Backend already exists: `core/review` (`Approve`/`Reject`) for the queue decision, and reprocess is the documents endpoint (`POST /documents/{id}/reprocess` → `core/processing.Reprocess`) — extend the UI surface.
- Also the home of **low-confidence derived contacts** for manual approval (§5.6).

### 2.8 Accounts page revamp (#8) — *early*
- Full UX redesign of `/finance/accounts` (currently basic). Cards/detail instead of raw tables.

### 2.9 App-native tables (#9) — *early*
- Tables don't look app-native (`documents-page.tsx` raw TanStack). Redesign rows: thumbnail + name + status + actions, better spacing, native feel. Restyle the shared `DataTable` or move to per-section card layouts.

### 2.10 Auth (#12) — *prerequisite*
- Basic auth now (token/session at `api/httpx` middleware + `commons/user`); **OAuth ready to drop in later**.
- Gates every other feature that touches user data.

---

## 3. Quick Fixes (contained)

| # | Comment | What |
|---|---------|------|
| 1 | Search preview + fuzzy | Compact detailed preview on each hit; fuzzy tokenization (`microwave`↔`micro wave`). |
| 2 | Sidebar alignment | Nav stays inside the ☰ sheet on the **left** (same side as ☰). No cross-side sidebar. |
| 6 | Back button | Home/back control returning to `/` (chat+insights home). |
| 17 | Unified `[+]` | One global add menu, identical on every screen; drop per-section `AddButton`. |

---

## 4. Late Scope (research / larger)

| # | Comment | Notes |
|---|---------|------|
| 7 | Tasks + disabled TBD sections | Subscriptions, relationships, inventory, **Tasks** are absent (confirmed). Show them as **"coming soon" placeholders** in the ☰ nav until built. Tasks live in TBDs — spec later. |
| 13 | Financial insights | Analytics on movements/accounts. Landing has a disabled **Insights** placeholder. Explicitly future. |
| 16 | Doc→0..n facets | See 2.6 — actually lands early (build order step 3; 2.6 tag corrected). |
| infra | External ingestion | fs-dir / object-storage source abstraction + async workers (see 2.1). |

---

## 5. Resolved Architectural Decisions

These are the **authoritative** architecture decisions (review-gated 2026-09-11,
source: `.tmp/architect-master-doc-review-20260911.md` + codebase follow-ups).
Where any earlier section conflicts, THIS section wins. Each subsection is
implementable as written; remaining genuinely-open items are listed at the end (§5.9).

### 5.1 R1 — Max-denormalized link model (links in JSONB payloads)

**Decision:** Links on **new/derived** entities are embedded as structured
fields **inside each entity's JSONB payload**. Zero FK columns or link tables
for new entities. Examples: `asset.payload.assetids[]`,
`memory.payload.contactid`, `memory.payload.assetid`, `contact.payload.assetid`.
*(C3 — deliberate choice, on the record:* the architect's initial recommendation
was typed FK columns for stable links; the user chose pure JSONB knowing
integrity becomes app-owned. That trade-off is accepted, not accidental.*)*

Reprocessing, re-parenting, derived-fact dedupe, and delete hygiene are
**application-owned** — they are first-class spec+test material, not schema
behavior.

**MONEY-MOVEMENT EXCEPTION (resolved, not open):**
`MoneyMovement.LinkedDocumentID` **keeps its real typed column** — it is live
code today (`commons/entity/movement.go`, migration `00001_schema.sql`, used by
`core/ledger` + finance queries — collapsing it would break shipped finance
functionality for no gain). This is a deliberate, documented exception to the
zero-FK rule for *pre-existing* entities: a movement→doc link is a stable,
always-indexed finance-transaction edge, not an inferred facet. "Zero FK" is
therefore scoped to **new/derived entities**; no migration is required. (For
contrast, `Document.AssetID` still collapses to a payload-backed read-only
derived field per §5.8.)

Reverse lookup ("which docs/assets/persons touch memory X", "which assets does
person Y link to") is a JSONB-path scan (`@>` containment) accelerated by a GIN
index, across ~6 tables. **Quantified ceiling (M1):** with a GIN `@>` prefilter
plus per-table btree indexes on scalar ordering columns (`created_at`,
`next_renewal_at`), reverse lookups stay single-digit-ms at **~10³–10⁴ rows per
entity** (per-user rows are hundreds→low-thousands today). Beyond **~10–20k
rows per entity** the cross-table scan + re-sort cost breaks the budget and a
maintained link-projection table (outbox-maintained materialized links) becomes
mandatory. Note honestly what GIN does *not* give you: it cannot order results
or accelerate cross-table ordering — ordering must come from scalar btree
columns and in-memory sort of the candidate set.

**Three guardrails (all mandatory):**

1. **Typed accessor per entity.** Every link payload read/write goes through
   that entity's repository struct with typed methods (`LinkFacet`,
   `UnlinkDoc`, `ReparentFacet`). No package hand-edits the jsonb of another
   package. Link reconciliation logic cannot drift.
    **OCC is a hard requirement (no silent lost updates):** the generic
    fetch-then-merge `pgRepository.Update` in `infra/postgres/repository.go`
    builds its `UPDATE` with a `WHERE` of `id = … AND <visibility>`
    (repository.go:483-485) and has **no version column and no conflict /
    `ErrConflict` path** — so updating a whole JSONB payload is a lost-update
    hazard under the app's concurrent async writers (ingest, orphan-reconcile,
    delete-cleanup — §5.3). **Every payload mutation MUST use optimistic
    concurrency** — a row-version token (compare-and-swap) or `SELECT … FOR
    UPDATE` — or carry a documented, tested waiver. This enforces the ST-007 /
    ST-008 concurrency invariants (check-then-act races are not accepted).
    The passing guard does **not** exist today: `infra/postgres/lostupdate_baseline_test.go`
    (`TestLostUpdateBaseline`, added in change `fix-review-cycle-criticals`,
    task 2.6) reproduces the A-fetch→B-fetch→A-write→B-write interleaving on a
    JSONB payload and on `MoneyMovement.LinkedDocumentID` and asserts the second
    write silently overwrites the first — it is a regression guard that pins the
    present no-OCC behavior, not a passing OCC guard. The mandatory guard that
    surfaces a conflict and preserves both writes comes from change
    `payload-link-model-guardrails` (change #3), at which point those baseline
    assertions flip.
2. **Filter-shaped fields are lifted to scalars — general rule (was the
   `next_renewal_at` one-off).** Any field used in a WHERE, ORDER BY, or join
   condition (range predicates, ordering, filtering — e.g. "warranty expires
   soon", "all contacts for asset Y", "memories referencing doc D") MUST be
   lifted to a real scalar column (or a dedicated derived/lookup table) **at
   write time**. Do NOT rely on `jsonb_path_ops` GIN for anything needing
   ordering or range predicates — it indexes containment only, not `#>>` cast
   expressions. `asset.next_renewal_at` is this rule's first instance,
   maintained by the assembly layer (recomputed when a facet links/unlinks;
   "high-confidence facet with later end-date wins").
3. **Deletion matrix as a named spec artifact.** Every entity's delete
   behavior with respect to pointing payloads gets a WHEN/THEN scenario —
   e.g. *WHEN an asset is deleted THEN `assetids[]` is cleaned from memories
   and contacts that point to it*. The matrix covers ALL entities: Person,
   Memory, Contact, Asset, Source, Document.

### 5.2 R2 — Version-chain Person (NOT "replace older")

- Person renewal (e.g. passport) → **NEW Person row**; the old row is marked
  `superseded_at` + `superseded_by_id` and is **never deleted**. Older records
  are preserved — lineage is permanent evidence.
- Uniqueness: **partial unique index on `key_hash WHERE superseded_at IS
  NULL`** (one active row per key per household).
- Memory de-dupe key is **per type** (resolved): ID number for person-derived
  records, name+kind for free-text notes. Structure/identity/expiry lives on
  Person; Memory holds only textual, non-structured notes.
- The dedupe machinery reuses `core/identity`'s pattern (indexed hash lookup +
  ambiguity holding) but as a dedicated Person-scoped helper — `identity.Resolve`
  itself remains Asset-scoped.
- Person derivation is triggered only by the modular confidence pipeline
  (§5.10): signal modules combine into an is-identity-doc confidence;
  thresholds decide derive vs tagged-doc-only vs review queue.

### 5.3 R3 — Asset assembly layer = event-driven targeted reconciliation (NOT rescan)

- Ingest pipeline: `source → extract → classify (doc role) → derive → link`.
  Add `core/assembly`; move identity consumption out of `core/processing`
  (processing ends at "extraction + consensus + Document committed-or-held").
- **Per-doc classifier:** `product-defining` (invoice/receipt with identity
  fields) vs `supporting` (warranty/AMC/service-visit).
- Product-defining → 3-stage resolve → owns/creates canonical Asset.
  Supporting → **read-only** match (`identity.Match`) → attaches as an
  **additive facet**; never merges brand/model/warranty fields into the parent.
- **Orphan facets** (supporting docs that matched nothing) enqueue a
  **targeted, bounded reconciliation job**: match the facet against
  product-defining docs sharing **household + brand/model/name** — indexed
  (household + doc_type + created_at window), never a full asset re-scan.
  **Event-driven:** when a new asset is created, fire ONE job to match orphan
  facets. **This replaces per-upload full rescans** (the earlier §5a.A
  "re-scans and auto-reorders" wording is retired).
- Parent selection is classifier-based, not confidence-only; confidence gates
  *projection* (e.g. warranty end date), conflicts surface to review.
- Ambiguous matches → review queue (§2.7).

### 5.4 R4 — Search = provider-registry engine + Postgres hybrid

- `core/search` becomes a **query engine over injected SearchProviders**; each
  entity registers its own provider. This retires the hardcoded 5-join union
  and makes new types (memory/person/contact) pluggable.
- **Postgres hybrid stack:** `tsvector` (keyword) + `pg_trgm` (fuzzy) +
  `pgvector` (semantic); embeddings only where they earn their keep.
- Long OCR text is **chunked (256–512 tokens)** into a child table; full
  chunk embeddings + a cheap document-level aggregate embedding for "find
  similar docs". Do NOT embed filename (structured) or raw OCR wholesale.
- **Ranking: RRF** (reciprocal rank fusion). **M4 — specified:** **global RRF**
  with the standard constant **k = 60** (`score = Σ 1/(k + rank)`) fused across
  all providers (tsvector / trgm / vector / per-entity), not per-type-local
  fusion. The default priority list (assets → accounts → movements → documents
  → import_batches → memories → persons → contacts) is a **precedence
  tie-break** applied only when RRF scores tie (or within rounding epsilon) —
  it is NOT a score boost. Graduating to a weighted sum (type-boost control),
  if ever needed, is the first config "graduation" (§5.9) — priority list then
  becomes explicit weights in a versioned config.
- **pgvector: RESOLVED — feasibility confirmed.** `CREATE EXTENSION vector` is
  trivial ops; requires Postgres 15+. Defer HNSW/approximate indexing until
  >~10–20k vectors per tenant (exact scan is fine below that). **M3 — the
  threshold is measured in CHUNK-VECTORS, not docs:** a doc chunks into many
  vectors (256–512-token chunks), so single tenants cross 10–20k vectors at
  only ~1–2k medium docs. Monitor the chunk-vector table's row count, not the
  documents table.
- **M3 — Embedding write-latency budget.** Embeddings are computed **eager on
  write** with an explicit budget: **≤5s p95 added ingest latency for a ≤10-page
  doc** (~10–40 chunks; at 50–300ms/chunk local bge-m3 or one API round-trip
  each this is otherwise unbounded). Mitigation is mandatory: chunk embeddings
  run **batched/in-parallel**, and for docs at/over the budget the embedding
  work is handed to a **background embedding worker** (doc is immediately
  searchable by keyword/trgm; vector result lands when the worker commits; the
  chunk row carries a `not-yet-embedded` marker until then). `core/embedding`
  isolates the provider (local model preferred, e.g. bge-m3, or an
  OpenAI-compatible endpoint) so a swap touches one package.
- Result card = `[type] name + thumbnail/placeholder`; cards are a registry
  matching the backend provider registry.

### 5.5 R5 — Nav position consistency

The nav lives **inside the ☰ sheet**, and ☰ is fixed **top-left**; the nav
sheet will open from and be left-aligned with the ☰. No cross-side sidebar.
**Resolved target design — NOT yet built in code**: live code today
(`nav-sheet.tsx`) still renders `side="right"`; moving the sheet left is the
Quick Fix #2 build work (the §8 log row records the current code reality).
Related quick fix #2 (#6 back-button, home = `/`) unchanged.

### 5.6 R6 — Contact auto-derivation (resolved)

- Contacts (first-class entity) are auto-derived from processed docs **only
  when extraction is high-confidence** (dealer name + contact present).
- **Low-confidence derived contacts go to the review queue for manual
  approval** — never silently discarded, never silently created.
- Derivation is an explicit pipeline with independent, individually
  confidence-gated steps (contact, facet, person) — the coupling worry was
  orchestration, not entity modeling.

### 5.7 R7 — Fuzzy + unstructured metadata specifics

- **Fuzzy:** `pg_trgm` for intra-word typos; tokenized `tsvector` + synonyms
  for cross-token variants (`microwave` ↔ `micro wave`).
- **Unstructured metadata:** `Extraction.Metadata` (snake_case JSON ≤8KB) and
  `Document.ExtractedFields` are **flattened to text** for `tsvector`
  ingestion and remain JSONB-path searchable — they are searchable even when
  values are untyped.

### 5.8 R8 — Miscellaneous resolutions

- **§2.1 deletion cascade** now references the deletion matrix (§5.1 guardrail
  3) covering ALL entities, not just Source/Document.
- **Download/view (resolved):** proxy stream through the API middleware today
  (auth/RLS-consistent); signed URLs when object storage arrives.
- **Memory `user_id` "link":** ownership is RLS context, not a modeled link.
- **`Document.AssetID` deprecation:** once 0..n facets land, it becomes a
  read-only derived compatibility column so `identity` stops writing it.

### 5.9 Still genuinely open

- **Identity doc type:** RESOLVED (2026-09-11, D1) — NOT an enum addition; identity docs are identified by tags (§2.4, §5.3-classifier still applies for product-defining vs supporting). Person-derivation *thresholds* remain configurable and unset until tuning data exists (§5.10).
- **Tag vocabulary:** free-form by design (extractor- + user-assigned, open-ended). No canonical value-only-tag list is enforced; conventional starters are `passport`, `driving_license`, `pan`.
- **Ranking-config detail:** exact field boost weights, if/when graduating
  from RRF to weighted sum (need not be decided for initial build).

---

### 5.10 D2 — Modular confidence pipeline for Person derivation (2026-09-11, user decision: architect researches + designs)

**Problem:** an identity-tagged doc yields a Person (with version-chain
semantics, §5.2) only when evidence is strong enough. A modular, composable
confidence system decides this — new signal modules can be added without
editing the pipeline.

**Architecture (`core/confidence`):**

- **Signal modules** — each implements `Signal.Evaluate(doc) SignalResult`
  returning a confidence in `[0.0, 1.0]` plus a machine-readable reason. Built-in
  initial set (`IdentitySignalRegistry`, extension point — register a new
  `Signal` implementation and the pipeline picks it up; no pipeline edits):
  1. **Tag signal:** kind/count of value-only identity tags (`passport`,
     `driving_license`, `pan` …) × presence/quality of key-value tags
     (`id_number`, `name`, `expiry`). Only recognized identity tag values
     contribute full weight; unknown tags contribute a partial floor.
  2. **Structural signal:** confidence-weighted presence of structured identity
     fields in the extraction (ID number shape/validated checksum, name, expiry
     date plausibility).
  3. **Extraction-classification signal:** the extractor's own
     is-identity-document assessment + its stated confidence.
- **Combination rule:** monotone **weighted geometric mean**
  `c = ∏ s_i^{w_i}` (Σw=1, weights configurable, equal-weight default) —
  chosen so a single weak signal drags the composite (no signal can be fully
  overridden by others; a 0 from the structural signal cannot be outvoted, and
  no signal can single-handedly cross threshold). No precedence.
- **Thresholds (configurable, versioned config; values TBD until tuning data):**
  - `c ≥ derive_threshold` → derive Person (version-chain per §5.2).
  - `review_threshold ≤ c < derive_threshold` → tagged Document + held-for-review (queue).
  - `c < review_threshold` → tagged Document only, no Person, no queue.
  Tags are assigned regardless of threshold — tagging and derivation are
  independent steps.
- **Person assembly:** approved derivation runs the Person-scoped dedupe
  (§5.2) → new Person row or `superseded_at` marking of prior chain member.
- **OCC note:** pipeline writes (tags, Person rows) go through the entity's
  typed accessor with mandatory OCC (§5.1 guardrail 1).

---

## 5a. Addons + Contacts (resolved in discussion, 2026-09-11)

These two pain points expand §2.6 (facets) and introduce the Contacts
entity. Decisions (with remaining resolution in §5):

### A. Asset order-independence + addon facets
- **Problem (pain 1):** asset creation is currently order-dependent. Upload the warranty/AMC doc first → stray asset; upload the warranty doc after the product → identity resolution wrongly *merges* it into the product asset (same brand/model).
- **Resolution:** fully specified in §5.3 (event-driven assembly layer: per-doc classifier, additive facets, event-driven targeted orphan reconciliation — no per-upload rescan) with the link mechanics in §5.1 (MONEY-MOVEMENT EXCEPTION, guardrails 1–3 incl. OCC).
- **Addon-facet UI (the §5a.A-specific point):** the parent asset's detail shows its base info + all attached add-ons (warranty status, AMC, service visits) as children.

### B. Contacts entity
- **Resolution (pain 2):** a first-class **Contacts** entity, structured (`name`, `type=dealer/support/receipt`, phone/email, `linkable` to a person/asset/doc) — links embedded per §5.1 (MONEY-MOVEMENT EXCEPTION).
- **Derivation coupling with addons — resolved → §5.6** (confidence-gated auto-derivation; low-confidence goes to review; explicit pipeline with independently confidence-gated steps: contact, facet, person).

---

## 6. Recommended Build Order

1. **Auth (#12)** — small, gates everything.
2. **Doc-first hardening (#3 + #4)** — preserve originals (already true), persist no-extract docs, generic ingest-source abstraction.
3. **Facet model (#16)** — max-denormalized JSONB links (§5.1, MONEY-MOVEMENT EXCEPTION) + assembly layer with typed accessors; doc detail with linked items + download/view + thumbnails. Foundation for #11/#15.
4. **Memory (#10)** — entity + CRUD page + vector index.
5. **Identity docs (#11→#15)** — identity tags (D1, §2.4) + §5.10 confidence pipeline → Person version-chain (§5.2, supersede — no replace-delete).
6. **Search enrichment (#5 + #1)** — provider-registry search + hybrid tsvector/pg_trgm/pgvector (pgvector feasibility: RESOLVED, see §5.4), unstructured-metadata search, mixed-type preview cards.
7. **Review queue (#14)** — doc + editable inferred fields + comment/reprocess. Also the home of low-confidence derived contacts (§5.6).
8. **UI redesign (#8 + #9)** — accounts page + table restyling.
9. **Quick fixes (#2 + #6 + #17)** — global `[+]`, search chrome, back/home.
10. **Late scope (#7 placeholders, #13 insights, infra, OAuth).**

---

## 7. Open Questions (need a decision)

Most former questions are now RESOLVED in §5 (pgvector/R4, memory dedupe key
/R2, search ranking algorithm/R4, facet link model/R1, contact auto-derivation
/R6, download-view proxy/R8, identity doc type → tags/D1 §2.4). Remaining open:

- **Person-derivation confidence thresholds:** the pipeline architecture is
  resolved (§5.10) but the numeric threshold values are configurable and unset
  until tuning data exists.
- **Tag vocabulary (D1):** free-form by design — no canonical enum enforced.
- **Ranking-config detail:** exact weighted-sum boost weights, if/when graduating from RRF (deferred until needed; not required for initial build).

---

## 8. Verified-vs-Needs-Checking Log

Rows marked 🔁 were verified against *historical* code; the resolved decisions
(§5) supersede the historical behavior where noted.

| Statement | Source | Status |
|-----------|--------|--------|
| Source stores original filename + file verbatim | `entity/source.go`, `config.StorageDir` | ✅ verified |
| Document is 1:1 with Source, single `AssetID` | `entity/document.go`, `entity/asset.go` | 🔁 verified historically; links become JSONB payload fields (§5.1, MONEY-MOVEMENT EXCEPTION) |
| `doc_type` has 6 values, no identity | `entity/doc_type.go` | ✅ verified |
| `Extraction.Metadata` = snake_case JSON ≤8KB, untyped | `parse/parse.go` | ✅ verified |
| `UserDirective` = ingest note | `entity/document.go` | ✅ verified |
| Search is ILIKE substring, no fuzzy/vector | `core/search/service.go` | ✅ verified (retired by §5.4 hybrid stack) |
| Search joins 5 entity types, priority order | `core/search/service.go:allHits` | ✅ verified (retired by provider registry, §5.4) |
| No auth layer; `user.UserFrom` context-only | `config`, `commons/user` | ✅ verified |
| ☰ top-left, nav right sheet, no back/home | `app-shell.tsx`, `router.tsx` | 🔁 verified historically; **code reality today: right-side sheet** (`nav-sheet.tsx` `side="right"`). The "left" nav (sheet inside the top-left ☰) is the **Resolved target design — NOT yet built in code**; landing it is Quick Fix #2 work. |
| per-section `[+]` differs by screen | `add-button.tsx` | ✅ verified |
| Subscriptions/relationships/inventory/tasks absent | `router.tsx` (7 routes) | ✅ verified |
| Chat/LLM note storage | none found | ⚠️ needs check |
