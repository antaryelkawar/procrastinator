## Context

The backend works but stores every field as a relational column (`assets` carries brand/model/serial/norm_*/category/lifecycle columns, `documents` carries `doc_type`/`extracted_fields`/`raw_extraction` columns, etc.). The UI is a prototype-grade SPA on a Vite/React stack drifting behind the toolchain. Domain logic in `core/*` and `infra/*` behavior (extraction, consensus, identity resolution, lifecycle, search, statement, ledger, review) is sound and stays behaviorally unchanged — this change rewires the storage mapping under it, modernizes the UI toolchain, and adds four user-facing features (landing page, documents section, duplicate-prompt flow, dark mode).

Relevant code sites surveyed (Sept 2026):

- `procrastinator-backend/migrations/00001–00006` — the column-based chain to be replaced from scratch.
- `infra/postgres/repos.go` — `filterConfig` whitelists (`assetFieldCols`, `documentFieldCols`, `sourceFieldCols`) map caller-influenced filter/order names to SQL columns; with jsonb storage these map to validated payload-path expressions instead, preserving the injection-safety contract.
- `infra/postgres/repository.go` — generic repo machinery assembles `SELECT * … WHERE` from key-column metadata; this is the single choke point for the payload mapper.
- Query patterns found: ILIKE list search over `name/brand/model/serial_number` (assets), `name/account_type/institution` (accounts), `description/external_reference` (movements), `filename` (batches, sources); FK joins `documents.asset_id`/`documents.source_id`; RLS predicates on `owner_id`/`owner_household_id`; soft-delete `deleted_at IS NULL`; norm-serial partial unique index; content-hash dedupe (`sources.sha256` per owner scope); three-stage identity resolution on `norm_*`.

## Goals / Non-Goals

**Goals:**

- Uniform physical entity shape: keys + RLS scope + lifecycle-with-constraints + audit + one `payload jsonb`.
- Payload-side normalized lookups with jsonb indexes; identity resolution behaviorally identical.
- API mirrors entity structure (`{ id, …, data: {…}, createdAt, updatedAt }`) end-to-end (OpenAPI → Go gen → UI gen).
- Framework/toolchain pass (verified versions, React Compiler, PWA), modular backend + feature-folder UI.
- landing-page, documents-section, duplicate-reprocess-flow, dark-mode.
- UX test plan as an apply gate (tasks group before signoff).

**Non-Goals:** React Native/Expo, Capacitor, real Insights/chat, data migration of existing rows, `core/*`/`infra/*` behavioral rewrites, auth changes, metaframeworks (evaluated: rejected — see Decision 6).

## Decisions

### D1 — Column split per entity (question-gated 2026-09-08)

Per-entity shape, keyed by **actual query patterns** (verified against `repos.go` filter configs, `repository.go` scoping, RLS tests, dupe lookup):

| Table | Key / FK / lifecycle / RLS columns | payload holds |
|---|---|---|
| `assets` | `id`, `owner_id`, `owner_household_id`, `deleted_at`, `created_at`, `updated_at` | `data`: name, brand, model, serial, purchase date, warranty end, price, currency, `asset_category` + confidence + `category_user_set`, `norm_name/brand/model/serial`, `merged_into`, `merged_at`, extraction metadata |
| `documents` | `id`, `owner_id`, `owner_household_id`, `asset_id` (FK ON DELETE SET NULL), `source_id` (FK), `deleted_at`, audit | `data`: `doc_type`, `status`, `extracted_fields`, `raw_extraction`, `confidence`, `user_comment` (new for reprocess hint) |
| `sources` | `id`, `owner_id`, `owner_household_id`, `deleted_at`, audit | `data`: filename, content_type, byte_size, `sha256`, storage_path, uploaded_at |
| `ingest_reviews` | `id`, `owner_id`, `source_id` (FK), `owner_household_id`, `deleted_at`, audit | `data`: doc_type, extraction payload, candidate asset ids, `best_matched_asset_id`, decision state |
| `financial_accounts` | `id`, `owner_id`, `owner_household_id`, `deleted_at`, audit | `data`: name, account_type, institution, balances, all account payload |
| `money_movements` | `id`, `owner_id`, `owner_household_id`, `account_id` (FK), `import_batch_id` (FK, nullable), `deleted_at`, audit | `data`: description, external_reference, amount, booking date, category, status |
| `import_batches` | `id`, `owner_id`, `owner_household_id`, `account_id` (FK), `source_id` (FK), `deleted_at`, audit | `data`: state, filename, format, line counts, all batch payload |
| `import_lines` | `id`, `owner_id`, `owner_household_id`, `import_batch_id` (FK), `deleted_at`, audit | `data`: line fields incl. status |
| `users` | `id`, `deleted_at`, audit | `data`: identity fields (no RLS scope beyond PK) |
| `households` | `id`, `deleted_at`, audit | `data`: name, members/metadata payload |
| `household_members` | join table stays relational (real FK pair `user_id`/`household_id` used by the at-least-one-membership predicate in `scope_access.go`) — exempt from payload shape as a pure association table |

Rationale for the splits:
- **`assets.deleted_at` stays a real column** (gated): it backs the soft-delete `IS NULL` filters *and* hosts the `WHERE` predicate of the partial unique norm-serial index; keeping it as a column keeps RLS/tenancy tests uniform. `merged_into`/`merged_at` move into payload (`data.merged_into` as asset id + `data.merged_at`); merge/uniqueness semantics enforced by domain queries over payload paths.
- **FK columns stay columns** where Postgres referential integrity uses them (`documents.asset_id` SET NULL, `documents.source_id`, `money_movements.account_id`, `money_movements.import_batch_id`, `import_batches.account_id`/`source_id`, `ingest_reviews.source_id`).
- **`doc_type` leaves column space** (moves to `documents.payload.data.doc_type`; the DB CHECK constraint is retired — vocabulary validation lives in Go, unchanged). `status` likewise payload-side (per spec scenario), backed by an expression index for the documents-section filter.
- **Content hash** stays in `sources.payload.data.sha256` (dedupe is per-scope lookup, no global uniqueness) with a btree expression index on `(owner_id, (payload #>> '{data,sha256}'))` (gated option).
- **`sources.filename`** stays payload but gets a trigram expression index — it is used both for search joins and documents-section search-by-filename.

### D2 — Indexing strategy (question-gated)

Two-tier jsonb indexing, all defined **per entity** in the fresh migration:

1. **Equality lookups → single-expression btree indexes.** Identity-resolution paths (`payload #>> '{data,norm_serial}'`, `norm_brand`, `norm_model`, `norm_name`) plus `(owner_id, payload #>> '{data,norm_serial}')` composite; dupe-hash `(owner_id, payload #>> '{data,sha256}')`; documents-status `((owner_id), (payload #>> '{data,status}'))`; import-batch state. These back the three-stage resolve and dedicated lookups, are cheap to write, and let EXPLAIN-verified stages prove index use in tests (spec scenario).
2. **ILIKE/fuzzy search → `pg_trgm` GIN expression indexes** on the ILIKE'd payload text paths (assets name/brand/model/serial; accounts name/type/institution; movements description/reference; sources filename). Justification: the landing search and list filters are the dominant interactive path; a btree expression cannot serve ILIKE leading-wildcard. Cost: heavier writes on payload updates — acceptable at this app's write volume (single-user uploads, not bulk ingest).
3. **No GIN jsonb_path_ops dump index** — no containment (`@>`) queries exist in current patterns; expression indexes cover the actual predicates. Revisit if later features introduce containment.

Migration extension note: enable `pg_trgm` extension in the new migration chain (allowed — same-DB, no data movement).

### D3 — Repository mapping layer

`infra/postgres` gains a generic **codec** in one file per entity kind-registry (e.g. `infra/postgres/payload_codec.go`): each kind declares `{table, columnsOfInterest (keys/audit), payloadPath root "data", fieldMap}`. Marshalling: `entity.Data → payload via fieldMap`, un-scoped fields excluded; unmarshalling inverse. The existing registry-driven `repository.go` selectors (`SELECT *`, filter/order assembly) are rewritten to consult the codec's column/payload-path mapping instead of string column names; `filterConfig` whitelists switch values from `"deleted_at"` → `"(payload #>> '{data,deleted_at}')"`. Repo layer is the only layer aware of payload; `core/*` untouched. RLS/soft-delete metadata (scoping/`deleted_at IS NULL`) still comes from per-entity key-column metadata in the registry, unchanged conceptually.

**Named `commons/*` changes** (per the spec's company/repo serialization requirement, scoped to exactly three points — everything else in `commons/*` is unchanged):
- `commons/entity/document.go`: `entity.Document` gains `UserDirective string` (Ingest/reprocess user note), mapped to `documents.payload.data.user_directive` by the codec field map.
- `commons/repo/payload_codec.go`: `PayloadCodec` interface (`Marshal(kind, entity) ([]byte, error)` / `Unmarshal(kind, []byte, any) error`) — the contract the infra codec implements and the generic repository consumes; no default SQL assembly in `commons/repo` changes otherwise.
- `commons/entity/import_batch.go` et al.: no other struct changes — all other payload fields stay inside the existing `Data` maps already present on each entity.

### D4 — API shape and codegen

- `api/openapi.yaml` re-modeled: every entity resource schema = `{ id, <scoped keys as needed>, data: <per-entity DataSchema>, createdAt, updatedAt }`. Error/duplicate-report schemas (`documents-section`, `duplicate-reprocess-flow`) added as object schemas, not ad-hoc maps.
- `api/gen` and `ui/src/lib/api/generated` regenerated; `wire_parity_test.go` and codegen lockstep tests updated to the entity-shaped wires.
the codegen lockstep tests updated to the entity-shaped wires.
- Handler DTOs (`dto.go`, `finance_dto.go`) remap from flat column maps to `data`-nested maps. Handlers still keep their existing behavior; only serialization changes.
- **Duplicate-outcome schema (named for implementers)**: `DuplicateReport` object schema in the re-modeled OpenAPI, shape: `{ code: "duplicate", existing_document_id, existing_source_filename, existing_source_uploaded_at, existing_asset_id?, prompt: { reprocess_uri, keep_uri, expires_at, timeout_toast: "no response — keeping existing document" } }` (HTTP status 409). The `prompt` object is what renders the reprocess/keep modal; `timeout_toast` is the verbatim UI toast text for the 10-min fallback. Referenced by name in tasks 3.3/3.4.

### D5 — Backend modularization

Flat `api/` (16 files) splits into `api/assets`, `api/add`, `api/search`, `api/documents`, `api/finance`, `api/reviews`, `api/households`, `api/tenancy` and keeps shared plumbing (`api/httpx` for request/response helpers, `api/gen`, `api/docs`, `api/dto` for shared contract concerns). Composition lives in the root (server wiring only). This is a move+import-update refactor, not a rewrite — behavior verified by unchanged test suites, `wire_parity_test.go`, and lockstep tests.

### D6 — UI stack and modularization

- **Stack**: Vite 8.2 (Rolldown), React 19.2.8 + React Compiler via `@vitejs/plugin-react` v6 `reactCompilerPreset` (`babel-plugin-react-compiler@1.x`), **react-router v8** (stack verdict: Next/Remix/SvelteKit **rejected**; TanStack Router also rejected; TanStack Query stays on v5.x — v6 is RC-only). Rationale logged once: this is a private, SEO-less, client-rendered CRUD tool on a Go API; SSR machinery adds runtime + migration cost with no material UX gain. Tailwind 4.3 CSS-first `@theme` token migration, lucide-react 1.x, shadcn CLI 4.x, vitest 5 (Node ≥22.12), `vite-plugin-pwa` 1.3 for manifest + app-shell service worker. All versions re-verified against npm dist-tags at implementation time and recorded in the task note.
- **Feature folders**: `pages/*` moves under `src/features/{add,search,assets,reviews,finance,documents,docs}` (docs = shared feature primitives; `components/`, `lib/`, `context/` shared). Cross-feature imports limited to `features/docs/` + `components/` + `lib/`; enforced by ESLint `no-restricted-imports` rule scoped per feature dir.
- **Routing**: `src/router.tsx` composes; `/` renders the landing feature. No persistent sidebar on any route: hamburger (☰, top-right) opens the slide-in side sheet on every view (per landing-page spec: profile avatar + name → Review Queue / Assets / Documents / Accounts; Subscriptions/Relationships/Inventory disabled TBD).

### D7 — New UI features wiring

- **Landing page** (`features/landing`): search bar → navigates to `/search?q=`; `[+]` reveals three ingest cards — Camera/Image (capture/pick + inline optional note), Document (pick PDF + inline optional note), Text (input area) — all pass-through with the note as ingestion directive, no pre-selection, no separate steps (per spec); Insights placeholder; fixed chat pill (disabled). Reuses `getUserMedia` for the camera card.
- **Documents section** (`features/documents`): `GET /api/users/{userId}/documents` list + status filter + filename search, `POST /documents/{id}/reprocess` (comment opt), `DELETE /documents/{id}`, "open asset"/edit links, asset reprocess action.
- **Duplicate prompt** (`features/add` + backend `api/add`): upload returns an explicit duplicate outcome (`409`-semantics structured body incl. existing document ref). UI shows modal prompt (reprocess/keep). Backend holds the document in "pending-choice" state for up to 10 min, recording keep-existing on timeout with a toast notification; no silent mark. Ingest entry point removes doc-type/account pre-selection.
- **Ingest note directive** (backend `api/add` + shared directive plumbing): the upload endpoint accepts an optional free-text note alongside the file, persisted as `documents.payload.data.user_directive` (exposed on the entity as `Document.UserDirective`, see D3), and passed into extraction as the prompt parameter `extractor.Extract(ctx, source, directive string)`. The verifier asserts `assert extraction prompt contains the note` in a stub-extractor test, making directive attachment machine-checkable. Iterate: same field the documents-section reprocess comment writes to — one directive channel, `data.user_directive`, for both ingest-time hints and reprocess-time hints. Cards share one inline-note pattern (plain inline field, never a separate step).
- **Dark mode**: `context/theme-provider` (system default, localStorage, `prefers-color-scheme` live listener), `class="dark"` toggling on `<html>`, apply-before-first-paint bootstrapping; Tailwind v4 `@theme` tokens supply light/dark palettes.

### D7a — Duplicate pending-choice state machine (gates task 3.3 vs 3.4 split)

The duplicate upload flow is a small explicit state machine; the mechanism is named here so `api/add` (creation) and `api/documents` (choice endpoints + sweeper) do not diverge:

- **Storage**: stars in the document's own payload — `documents.payload.data.pending_choice = { state: "pending" | "resolved", outcome: "" | "reprocess" | "keep_existing", created_at, expires_at }`. The document row itself stays soft-pending (not enqueued for extraction) while `pending_choice.state = "pending"`; existing-document referencing fields for the modal come from the target document (`existing_source_filename`, `existing_source_uploaded_at`, `existing_asset_id` of `DuplicateReport`, D4).
- **Timeout clock — two clocks, one deterministic outcome** (explicit choice: lazy evaluation on next request + a periodic sweeper):
  - *Sweeper* (primary): `api/documents` owns a background job (same tick loop the pipeline already uses; started from the composition root) that every 60 s queries payload paths where `state = "pending" AND expires_at < now()`, resolves each to `keep_existing` and emits the toast event (`timeout_toast` from `DuplicateReport`) to the UI's notification channel.
  - *Lazy guard* (safety net): any read/write touching such a document (list, get, reprocess, keep) first applies the rule `pending AND expires_at < now() → keep_existing` inline, so a temporarily stalled sweeper can never surface stale pending state. This makes the outcome deterministic regardless of process timing.
- **Restart recovery**: no in-memory state carries the clock — `created_at`/`expires_at` are in the payload, so a process restart recovers in-flight pending choices for free (ST-011); the sweeper resumes from the payload on boot and resolves any already-expired rows.
- **Ownership split** (disjoint): task 3.3 (`api/add`) only *creates* `pending_choice` rows on duplicate detection (and never resolves them); task 3.4 (`api/documents`) owns the *mutators and reads* — reprocess (`POST /api/users/{userId}/documents/{id}/reprocess`), keep (`POST …/{id}/keep`), the sweeper, and the lazy guard. `core/pipeline` is unchanged (reprocess reuses the existing re-entry path already added by the documents-section work).

### D8 — UX test plan (apply gate)

A dedicated task group with per-row checklist items (flow × viewport × mode). Matrix rows enumerated as tasks — mobile 360×640 + desktop ≥1280 (768 optional check) × flows (landing, hamburger ☰ sheet open/close + each nav link lands its view incl. disabled TBD items, `+` ingest cards all 3 modes with + without inline note, search flow, asset detail/edit, documents list/filter/reprocess/delete/edit-asset/asset-reprocess, duplicate-prompt both choices + timeout, finance, reviews) × light/dark. Quality bar enforced: no 360px horizontal overflow, ≥40px touch targets, readable contrast both themes, no layout shift on theme change, loading skeletons, visible keyboard focus on desktop, toasts for destructive-adjacent actions. Automated where feasible in vitest; the rest manually traced w/ screenshots. Any failing row blocks apply-pass until fixed and re-verified.

## Delta 2 Decisions (subspec-delta-2, 2026-09-09 — spec review PASS cycle 1)

### D9 — Route surface & legacy removal (app-chrome)

Reachable surface is exactly: `/` (landing), `/search`, `/ingest/reviews`, `/assets`, `/assets/:id`, `/documents`, `/finance/accounts`, plus `*` → NotFoundPage. The legacy routes currently registered — `/add`, `/finance/movements`, `/finance/import`, `/finance/import/history`, `/finance/import/:batchId` — are **deleted from the router**, each redirecting to its closest surviving view (`/add` → `/`; finance sub-views → `/finance/accounts`). Route-tree-exclusive page components are deleted with the routes (not just unlinked): `features/add/add-page*`, `features/finance/movements-page*`, `features/finance/import-page*`, `features/finance/import-history-page*`, `features/finance/import-lines-table*`, plus finance flow components with no surviving entry (`movement-create-form*`, `movement-link*`, `movement-actions*` — verified no surviving consumer). A vitest route-table test asserts the exact surface: each reachable route renders its view, each legacy path redirects, an unknown path renders not-found, and `/assets/:assetId` still deep-links. (Movements/import *backend* entities stay — only their standalone views are removed from the surface.)

### D10 — Chrome layout: no top bar (app-chrome / landing-page rewrite of the 7.1 contract)

Delta 2 supersedes the old top-bar chrome of 7.1:

- `app-shell.tsx` loses its `<header>` entirely; the sole remaining chrome element is the ☰ hamburger as a **fixed top-left** floating icon button (≥44px, on every view incl. landing). The top-bar ThemeToggle and any brand-bar element are removed from the shell.
- `nav-sheet.tsx` absorbs what the bar dropped: its content order becomes profile row (avatar + name, existing switcher) → **Home** link (first nav item, navigates `/`) → Review Queue / Assets / Documents / Accounts → **theme toggle row** (reuses `theme-toggle`) → the 3 disabled TBD rows unchanged. Focus-restore to trigger stays.
- Landing wordmark: `landing-page.tsx` renders a stylized "PROCRASTINATOR" hero mark (text, tracked/letter-spaced, `tracking-[0.3em]`-scale, not an image — no new asset needed) centered above search, as a `<Link to="/">`.

### D11 — Unified Add composer + progressive-disclosure review chips (intuitive-input / landing-page / documents-section / asset-registry)

- **Shared `AddComposer`** lives in `ui/src/features/docs/composer/` (the cross-feature hub allowed by the ESLint boundary) so every consumer feature (`landing`, `documents`, `assets`, `finance`, `reviews`) may import it. Surface: one primary describe-input ("Describe it — brand, model, anything you remember" class placeholder), an attach strip (📷 camera capture via `getUserMedia`/capture input; 📁 file picker; ⌨ paste-text) with attached fragments rendered as removable chips in-session, and the always-available optional directive note ("Anything the scanner should know?") carried as `data.user_directive` per document-ingestion. Bottom sheet at narrow widths, inline dialog ≥ `sm` (same component, responsive variant).
- **Submit routing (no new endpoints)**: file/attach → `POST /api/users/{uid}/documents` multipart (`file` + optional `note`) — the task-3.3 contract byte-identically, incl. `409 DuplicateReport` → the existing landing `duplicate-prompt-dialog` **reused, not rewritten**; paste-text-only → the existing text ingest path; camera capture → same file path. Asset/account/review creation from a section's context [+] uses the same composer; extracted values render via the review-chips step and are POSTed through the **existing** asset/account/review create endpoints — payload shape unchanged (asset-registry "composer-derived asset round-trips like an extracted one", duplicate-serial rule applies unchanged).
- **Review chips component** in `features/docs/composer/review-chips.tsx`: extraction output rendered as confirmable chips (brand/model/serial/price…), tap-to-reveal/correct, unextracted optional fields behind a collapsed "add details" affordance; user can save unedited. No layout renders old-style field-grids; legacy form edit affordances (documents-row edit, asset-detail edit fields) are edit-time, not creation-time, and stay.
- **Legacy demotion**: the three-card ingest grid (`features/landing/cards/ingest-cards.tsx`) and the old multi-field composition paths are removed from the creation entry; card feature files that the composer replaces are deleted; `duplicate-prompt-dialog` + `cards/hooks.ts` (ingest upload plumbing) are kept and consumed by the composer.
- **Backend impact**: none new. Task 14.7 is a [backend] verification pass that the composer's callsides hit the existing endpoints exactly (note→`data.user_directive`, 409 contract, no pre-selection params) — lockstep/wire-parity tests are the machine check.

- [Identity-resolution regressions under expression indexes] → Resolve tests (`resolve_test.go` cases) run against the jsonb schema; per-stage EXPLAIN assertions (index used, correct plan) catch index drift.
- [Payload mapping loses type fidelity for scalar mixes (e.g. `price` numeric precision)] → Money fields stored as JSON **numeric literals** (not strings) and a unit test asserts the exact round-trip (`39999.99`/`INR`).
- [Heavy write amplification on trigram indexes during bulk import-line writes] → Indexes only cover entity kinds actually ILIKE-searched; if import-line bulk ingest regresses badly, drop trigram on import_lines payload paths and re-evaluate (document follow-up).
- [Tailwind v4 token migration is mechanical but broad] → Follow the CSS-first `@theme` migration path; compiler catches breaking refs; one deliberate pass per feature folder.
- [PWA caching can serve stale assets in dev] → Service worker registered only on production build; dev builds skip SW.
- [Metaframework rejection locked without recheck] → Not a risk; decision documented here with rationale (D6) per spec requirement.

## Migration Plan

The existing migration chain `00001–00006` is replaced by a **fresh chain from scratch** (database is WIP, no data preserved):

1. Delete `migrations/00001…00006`; author a new `00001_schema.sql` defining every entity table in the uniform shape (incl. D1/D2 indexes, trigram extension, RLS policies mapping the old scoping rules).
2. `migrate_test.go` rewritten to a fresh-shape schema test (no legacy asserts); RLS test suite re-targeted to the new table definitions.
3. Repo codec layer (D3) deployed as a single commit alongside the schema shift so no commit exists that pairs the old repos with the new tables.
4. Rollback strategy: if the new chain must be rolled back after implementation begins, the previous archived change's chain (`openspec` archive of asset-management-rework) still exists in git history; no live data is at risk.

## Open Questions

(none — gated via question tool 2026-09-08: lifecycle split D1, dupe-hash D1/D2, ILIKE/trigram D2.)
