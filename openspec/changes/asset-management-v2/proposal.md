## Why

The app works but feels like a prototype: a column-per-field relational model that fights the open-ended LLM-extracted data, a generic "assets" redirect as the home screen, a flat `api/` package and page soup in the UI, no documents overview, and a stack drifting behind the toolchain. This change reworks the data layer to schema-flexible JSONB, redesigns the landing page into an ingest-first hub, and modernizes/reorganizes both codebases so future work lands in obvious places.

## What Changes

- **BREAKING — JSONB entity storage**: all entities (assets, documents, sources, ingest_reviews, accounts, movements, import batches, users, households) migrate to a uniform physical shape — identity/FK/RLS key columns + audit timestamps + one `jsonb` payload column. Migrations start **from scratch** (throw away 00001–00006 chain; database is WIP, no data to preserve). Normalized lookup fields (e.g. `norm_name`, `norm_brand`, `norm_model`, `norm_serial`) move **into** the payload; identity resolution queries them via GIN/jsonb expression indexes, not plain columns.
- **BREAKING — API mirrors entity structure**: responses expose the nested entity with its payload fields, not flattened column lists. OpenAPI re-modeled; codegen and existing consumers re-synced.
- **Landing page redesign**: `/` becomes the app's home — centered PROCRASTINATOR brand wordmark (hero, tap→home), big search bar, centered [+] button opening a **unified Add composer** (delta 2: single describe-input + camera/file/paste attachment strip + optional ingestion directive note; replaces the earlier three-card layout), a labeled Insights placeholder, and a static chat-bar placeholder pill. Mobile-first.
- **Chrome redesign (delta 2, user corrections)**: the previous top bar is removed entirely. Every view carries only: a hamburger (☰) floating **top-left** opening the sole slide-in navigation sheet — which now also contains the profile (avatar + name) and the light/dark/system theme toggle (moved out of the old top bar) — and, on every non-landing view, a context [+] icon **top-right** that adds that section's thing (reviews / asset / document / account). The sheet gains a **Home** entry (first nav link) and the wordmark is a tap-affordance so `/` is always reachable. The route surface collapses to `/` (landing), `/search`, `/ingest/reviews`, `/assets`, `/assets/:id`, `/documents`, `/finance/accounts`; legacy routes are removed (redirect/404 on deep link) — no dead links.
- **Intuitive input, no old-style forms (delta 2)**: all add/create flows (assets, documents, accounts, reviews, and the unified ingest) adopt a research-informed **directive-first unified composer with progressive disclosure** (single describe-text input + attach strip, optional directive note, then an AI-prepared review of confirmable "chips" instead of a blank field-by-field form). Data shapes, API, and jsonb storage are unchanged — this delta is about chrome, routes, and input UX (+ any backend support the composer needs), while keeping already-implemented behavior consistent.
- **Framework pass** (versions verified 2026-09-08 via npm registry / GitHub releases, not training data):
  - Vite 8.2 (Rolldown), React 19.2.x + React Compiler 1.0 (via `@vitejs/plugin-react` v6 `reactCompilerPreset`), react-router **v8** (stay on react-router; TanStack Router rejected), TanStack Query 5.x (v6 dropped — RC only), Tailwind 4.3 (CSS-first `@theme`), lucide-react 1.x, shadcn CLI 4.x, vitest 5.
  - **Next.js / Remix / SvelteKit evaluated and rejected for this app**: private, SEO-less, client-rendered CRUD on a Go API — SSR frameworks add a server runtime and migration cost with no material UX gain.
  - **Mobile-native path = PWA** (`vite-plugin-pwa`): installable, offline app-shell, camera via getUserMedia. React Native is explicitly out of scope (separate UI runtime, cannot reuse shadcn/Tailwind components); Capacitor wrap deferred.
- **Backend modularization**: flat `api/` (16 files) split into subpackages (`api/assets`, `api/search`, `api/add`, `api/documents`, `api/finance`, `api/reviews`, `api/households`, plus shared `api/httpx`, `api/gen`). Domain logic in `core/` stays as-is.
- **UI modularization**: `pages/*` reorganized into feature folders (`features/add`, `features/search`, `features/assets`, `features/reviews`, `features/finance`, `features/documents`, `features/docs` shared) with co-located hooks/components.
- **Documents section**: new documented view listing all documents with processing status (`processed` / `in_review` / `failed` / `asset_less`), their linked asset, and actions: reprocess unprocessed documents with optional comment, delete documents, edit asset data, reprocess assets.
- **Duplicate reprocess prompt**: uploading a content-hash duplicate now asks the user — reprocess (fresh extraction) or keep existing — instead of silently marking the document duplicate.
- **Dark mode**: light / dark / system toggle, persisted preference, applied before first paint (no flash).
- **UX test plan as a plan gate**: the implementation plan must contain a UX-oriented test plan covering mobile 360px, desktop, dark mode, and all primary flows, with a defined quality bar.

## Capabilities

### New Capabilities
- `jsonb-entity-storage`: uniform entity physical shape (keys + audit + jsonb payload), from-scratch migrations, payload-side normalized lookups with jsonb indexes, API mirroring entity structure.
- `landing-page`: the `/` experience — brand wordmark, search bar, [+] unified Add composer (delta 2 replaces the three-card ingest hub), Insights placeholder, chat-bar placeholder; hamburger (top-left) + slide-in navigation sheet as the only navigation mechanism.
- `framework-currency`: framework upgrade requirements (verified-stable stack decisions), PWA installability/mobile readiness, and the metaframework evaluation verdict.
- `modular-structure`: backend subpackage split + UI feature-folder organization requirements.
- `documents-section`: documents overview view — statuses, asset linkage, reprocess/delete/edit actions.
- `duplicate-reprocess-flow`: duplicate-upload prompt behavior (reprocess vs keep existing) replacing the silent duplicate mark.
- `dark-mode`: light/dark/system theme toggle with persistence and flash-free hydration.
- `ux-test-plan`: the mandatory UX test plan artifact and its coverage/quality-bar requirements (a plan gate, not a runtime feature).
- `app-chrome` (delta 2): exact reachable route surface (legacy routes unreachable), Home always reachable, no top bar (top-left hamburger + centered wordmark; profile + theme toggle inside the sheet), context top-right [+] on every non-landing view.
- `intuitive-input` (delta 2): research-informed directive-first unified composer with progressive-disclosure review replaces old-style field-by-field forms in all add/create flows; research citations in the spec.

### Modified Capabilities
- `document-ingestion`: duplicate detection outcome changes from silently marking duplicate to offering reprocess; no doc-type/account pre-selection at ingest entry; endpoint contract unchanged, consumed by the unified composer (delta 2).
- `asset-registry`: canonical asset model changes from structured columns + `metadata` jsonb to key columns + full jsonb payload; identity-resolution fields move into the payload (indexed as jsonb expressions); asset creation input becomes the directive-first composer (delta 2).
- `landing-page` (delta 2): three-card [+] layout replaced by the unified Add composer; hamburger moves top-left; no top bar; wordmark + sheet Home entry.
- `documents-section` (delta 2): adds the corner [+] composer entry; base list/contract unchanged.
- `duplicate-reprocess-flow` (delta 2): ingest entry wording follows the unified composer; reprocess/keep contract unchanged.
- `ux-test-plan` (delta 2): matrix updated for chrome, routes, context [+], and composer flows; carried-over rows preserved.

## Out of scope

- React Native / Expo, and the Capacitor store wrap (mobile path this change = PWA only; Capacitor deferred to a future change).
- Rewrites of backend domain logic: `core/*` (processing, identity, lifecycle, search, statement, ledger, review) and `infra/*` stay behaviorally unchanged — only the storage mapping/repo layer and package locations move.
- Real Insights widgets and a working chat assistant — both are static placeholders on the landing page.
- OCR of handwritten labels / non-PDF-PNG-JPEG formats beyond today's accepted types.
- Data migration of existing rows — the schema starts from scratch (WIP database, no data preserved).
- Authentication changes (passkeys/OTP beyond current user plumbing) and multi-tenant theming.
- (delta 2) Reimplementation of the extraction pipeline or change to document-enum/classification semantics — the composer only changes the input interaction; behavior under it stays as already implemented.

## Impact

- **Backend**: `migrations/` (obsolete chain replaced), `infra/postgres` (repository, scans, filters, RLS mapping), `commons/entity` + `commons/repo` (serialization to/from payload), flat `api/*.go` (moved, not rewritten), `api/openapi.yaml` (re-modeled to entity-shaped schemas) + regen of `api/gen` and `ui/src/lib/api/generated`.
- **UI**: `src/router.tsx` (route surface trimmed to the new IA), new landing route, `pages/` → `features/` moves, shadcn components (theme tokens, Tailwind v4), new `dark-mode` provider + landing page + documents section + duplicate-prompt flow; delta 2 additionally reworks the app shell (`components/nav-sheet` absorb profile/theme; top bar removed), adds unified ingest composer + review-chip flow components, and re-points each section's add flow to the composer.
- **Tooling**: `ui/package.json` (Vite 8, React 19.2.8, tailwindcss 4.3, lucide-react 1.x, vitest 5, vite-plugin-pwa), Node ≥22 floor, `@testing-library/dom` added.
- **Specs archived**: `openspec/specs/*` carry requirements in their old terminology (column-based, silent dedupe); this change ships delta specs for `document-ingestion` and `asset-registry` and MUST be applied alongside the implementation.
- **Risk**: identity-resolution correctness under jsonb expression indexes (mitigated: matching regression tests per lookup stage); Tailwind v4 token migration across all components (mitigated: mechanical, compiler-verified).
