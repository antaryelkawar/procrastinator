# Life Manager — Master Feature Doc (LIVING)

> **Status:** Living doc. Drafted to capture all discussed direction until
> everything is sorted. Not yet a committed spec. Review → resolve → then this
> becomes the source for OpenSpec changes.
>
> **Last updated:** 2026-09-11
>
> **Model today:** React/Vite UI (`ui/src`) + Go backend (`procrastinator-backend`,
> Postgres with per-user RLS). Search = ILIKE substring only. No auth layer yet.

---

## 0. What's True in the Code Today (verified)

These are facts, not opinions. Everything below builds on them.

### Domain model (backend `commons/entity`)
- **`Source`** = one uploaded file row. Fields: `Filename`, `ContentType`, `Size`, `Path`, `SHA256`, `UploadedAt`. The **original file + name are stored verbatim** at `{storage_dir}/{owner_id}/{uuid}.{ext}`.
- **`Document`** = one extraction/processing row, 1:1 with a `Source`. Fields: `SourceID`, `AssetID`, `DocType`, `ExtractedFields` (`map[string]any`), `RawExtraction` (full LLM JSON, unmodified), `Confidence`, `UserDirective` (the optional ingest note), `OwnerHouseholdID`, `DeletedAt` (soft delete), `PendingChoice` (duplicate-upload pending state).
  - `DocType` is a **6-value enum**: `invoice, receipt, warranty, amc, statement, other`. **No identity type.**
- **`Asset`** = derived product record: `Brand, Model, Name, SerialNumber` (+ normalized `norm_*` columns), `PurchaseDate, WarrantyEnd, Price/Currency, Metadata`, `AssetCategory` (10-value enum incl. `document_only`, `other`), `Confidence`, `CategoryUserSet` (user-set categories are sticky), soft-delete w/ retention.
- **Finance**: `Account`, `MoneyMovement` (has `LinkedDocumentID`, `LinkCreator`, `LinkConflicting` — the only facet-link pattern that exists), `ImportBatch`, `ImportLine`, `ImportSource`.
- **Search hit** (`Hit`): `Type, ID, Title, Subtitle, Confidence`. No preview/image/text blob.

### Search (backend `core/search`)
- Joins 5 entity types: **assets, accounts, movements, documents, import_batches**.
- **Pure substring** (`%q%` ILIKE), metacharacters escaped. **No fuzzy, no vector, no semantic.**
- Type-priority ordering: assets → accounts → movements → documents → import_batches.
- Quick (top-N) + Paged endpoints. Query capped at 200 chars.

### Identity resolution (backend `core/identity`)
- `Resolve`/`Match`: deterministic **3-stage indexed lookup** — `norm_serial` → `norm_brand+norm_model` → `norm_name+norm_model`. Soft-deleted never match; ambiguous → held for review; merges into existing or creates new. **Asset-scoped only** (serial/brand/model).

### Ingest (UI `features/docs/composer`, `features/landing`)
- **`AddComposer`**: ONE unified surface — upload (drag/drop/file picker), camera, ONE optional free-text note. Text-only path wraps the text as a `pasted.txt` document (`client.addItems`). File path returns `committed` | `held` | `duplicate`.
- **No memory-from-text path** exists. Text notes are not stored as a separate entity.

### UI chrome
- **`AppShell`**: ☰ is fixed **top-left**; nav is a **right-side sheet**; no sidebar, no back/home button anywhere.
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
5. **Top chrome is constant** — always `[☰][search][+]`. Nav lives inside the ☰ sheet. Home = the chat + insights landing screen.

---

## 2. The Features

### 2.1 Document-manager posture (#3) — *early*
- Keep `Source` as the permanent home of the original file + name.
- Never rename/rewrite originals; derived data lives on `Document`/facets.
- **Near-future ingestion sources:** abstract ingest so it can pull from a directory or object storage (S3/gCS) asynchronously. `core/processing` is already async-capable — extend the job/source layer.
- Deletion: deleting a source/doc **asks** whether to also delete linked facets (prompt; default off; never auto-delete).

### 2.2 Always upload even with no data (#4) — *early*
- A doc with no extractable fields is still saved as `Source`+`Document` (already the case via `asset_less`/`failed`).
- Surface these clearly; keep them **fully searchable** (filename + `RawExtraction` full text).

### 2.3 Memory (#10) — *early*
- New **Memory** entity: independent, type-tagged (`person`, `note`, `custom`…), `optional document_id` link, `optional user_id` link.
- Sources: the `[+]` text, chat, and LLM-suggested.
- CRUD page; **vector-embedded** like docs.
- De-dupe by key (e.g., ID number); a newer record **replaces** an older one (e.g., passport renewal).

### 2.4 Identity documents (#11 → #15) — *early*
- Add `doc_type = identity` (and/or identity subtype: license/passport/PAN).
- Extraction: ID number, name, expiry → **Person memory** (dedupe by ID number; newer replaces older; optional user link).
- Person memories are the LLM's source of truth for name/ID/expiry (feeds #15).
- Reuse `core/identity` resolve/dedupe machinery but **scope it to ID numbers**, not asset serials.

### 2.5 Intelligent multi-entity search (#1 + #5 + search-enrichment) — *early*
- Search across **all** entity types, including: docs, assets, movements, accounts, memories, persons, and **unstructured metadata** (`Document.ExtractedFields` + `Extraction.Metadata` — the snake_case JSON blob ≤8KB with untyped values, which "does not have fields to use" but is searchable text).
- **Mechanism (agreed): "all of it, with priority + config."**
  - Embed everything textual: filename, OCR/full text (`RawExtraction`), extracted fields, unstructured metadata, and free-text notes.
  - Hybrid: keyword (ILIKE/structured) + semantic (vector), **ranked by type priority** and **configurable boost weights**.
  - Need an embedding store (e.g., **pgvector** on Postgres) — verify feasibility before committing.
- Fuzzy: `microwave` should match `micro wave`. Add tokenization/q-gram/fuzzy matching (not pure substring).
- Result card = `[type] name + thumbnail/placeholder thumbnail`; click opens the appropriate page (document → doc detail, asset → asset detail, person → memory, etc.).
- **Verified gap today:** no fuzzy, no vector, no unstructured-metadata search, no memory/person type yet.

### 2.6 Facet / link model (#16) — *late*
- Generalize the doc→facet relationship. Today `Document.AssetID` is a **single 0..1 link**. Need **doc → 0..n linked items** (assets, movements, memories, persons).
- Doc detail page: show **all linked items**, open any of them, **download / view** the original doc, **thumbnail** per doc/asset.
- The current "actions" model (single "Open asset" per row) is wrong for 0..n facets.

### 2.7 Detailed review queue (#14) — *early*
- Per item: view the original doc, view inferred info (editable), **approve / decline**, plus **add comment** + **reprocess** buttons.
- Backend (`core/review`, `approve`/`reprocess`) already exists; extend the UI surface.

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
| 16 | Doc→0..n facets | See 2.6. |
| infra | External ingestion | fs-dir / object-storage source abstraction + async workers (see 2.1). |

---

## 5. Recommended Build Order

1. **Auth (#12)** — small, gates everything.
2. **Doc-first hardening (#3 + #4)** — preserve originals (already true), persist no-extract docs, generic ingest-source abstraction.
3. **Facet model (#16)** — doc→0..n link table; doc detail with linked items + download/view + thumbnails. Foundation for #11/#15.
4. **Memory (#10)** — entity + CRUD page + vector index.
5. **Identity docs (#11→#15)** — `doc_type=identity` → Person memory, dedupe by ID, replace-on-renewal.
6. **Search enrichment (#5 + #1)** — tags, hybrid vector/fuzzy, unstructured-metadata search, mixed-type preview cards. **Verify pgvector feasibility first.**
7. **Review queue (#14)** — doc + editable inferred fields + comment/reprocess.
8. **UI redesign (#8 + #9)** — accounts page + table restyling.
9. **Quick fixes (#2 + #6 + #17)** — global `[+]`, search chrome, back/home.
10. **Late scope (#7 placeholders, #13 insights, infra, OAuth).**

---

## 5a. Addons + Contacts (resolved in discussion, 2026-09-11)

These two pain points expand §2.5 (facets) and §2.8 (contacts). Decisions:

### A. Asset order-independence + addon facets
- **Problem (pain 1):** asset creation is currently order-dependent. Upload the warranty/AMC doc first → stray asset; upload the warranty doc after the product → identity resolution wrongly *merges* it into the product asset (same brand/model).
- **Resolution:** introduce an **asset assembly layer** on top of `core/identity`.
  - Every processed doc becomes a `Document` (kept as-is, order-independent).
  - The assembly layer re-scans related docs and **auto-reorders** so the canonical product asset is derived first; warranty/AMC/service-visit docs become **child facets** (`parent_asset_id`) rather than merging fields onto the parent.
  - **Warranty end date surfaces onto the parent asset ONLY when the source doc's extraction is high-confidence**; otherwise it stays on the facet only. (Not auto-surfaced blindly.)
  - User-facing: the parent asset's detail shows its base info + all attached add-ons (warranty status, AMC, service visits) as children.

### B. Contacts entity
- **Resolution (pain 2):** a first-class **Contacts** entity, structured (`name`, `type=dealer/support/receipt`, phone/email, `linkable` to a person/asset/doc).
- **One flow with addons (pain: linkage = "yes, one flow"):** processing an AMC/warranty doc can derive a contact (dealer/customer-care) automatically. Auto-derivation is **confidence-gated** — only promote when extraction is clear (dealer name + contact present); otherwise contacts stay manual. (Hybrid approach.)

### Open sub-decision
- Auto-deriving contact + facet + (maybe) person from one doc is a lot of coupling. Confirm whether low-confidence doc → *no* auto-contact, and whether the review queue surfaces derived contacts for confirmation.

## 6. Open Questions (need decisions)

- **Embedding store:** pgvector vs. external service? Verify Postgres + provider support.
- **Identity doc type:** single `identity` value, or separate types (license/passport/PAN)?
- **Memory de-dupe key:** per-type (ID number for person, name+kind for note)?
- **Search ranking config:** what's the default type priority, and which fields get boost weights?
- **Facet model:** reuse `MoneyMovement.LinkedDocumentID` pattern, or a dedicated generic link table?
- **Download/view:** server-signed URL vs. proxy stream?

---

## 7. Verified-vs-Needs-Checking Log

| Statement | Source | Status |
|-----------|--------|--------|
| Source stores original filename + file verbatim | `entity/source.go`, `config.StorageDir` | ✅ verified |
| Document is 1:1 with Source, single `AssetID` | `entity/document.go`, `entity/asset.go` | ✅ verified |
| `doc_type` has 6 values, no identity | `entity/doc_type.go` | ✅ verified |
| `Extraction.Metadata` = snake_case JSON ≤8KB, untyped | `parse/parse.go` | ✅ verified |
| `UserDirective` = ingest note | `entity/document.go` | ✅ verified |
| Search is ILIKE substring, no fuzzy/vector | `core/search/service.go` | ✅ verified |
| Search joins 5 entity types, priority order | `core/search/service.go:allHits` | ✅ verified |
| No auth layer; `user.UserFrom` context-only | `config`, `commons/user` | ✅ verified |
| ☰ top-left, nav right sheet, no back/home | `app-shell.tsx`, `router.tsx` | ✅ verified |
| per-section `[+]` differs by screen | `add-button.tsx` | ✅ verified |
| Subscriptions/relationships/inventory/tasks absent | `router.tsx` (7 routes) | ✅ verified |
| Chat/LLM note storage | none found | ⚠️ needs check |
