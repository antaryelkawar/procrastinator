# Asset Management Rework

## Why

Cycle 1 delivered the backend rework (parallel extraction + consensus, dedupe, taxonomy,
lifecycle, unified add, enhanced search) and proved it against the registry data — but the
application still presents like a document tool with a bolted-on list: `/` redirects into a
list view, the add flow and search bar live behind a permanent sidebar, the app looks like
"an html page" rather than a modern mobile-native-feeling app. On top of that, a real
defect surfaced in use: uploads that **succeed at upload but fail at processing** leave
invisible, stuck sources — no document row, no asset, nowhere in the UI to see or retry
them. The registry also can't be trusted when a byte-duplicate re-upload is silently
marked "duplicate" without asking the user whether they actually wanted it reprocessed.
The dependency audit (cycle 1, task 7.1) deliberately held six newer framework majors
(Vite 8, TS 7, vitest 5, plugin-react 6, jsdom 30, jest-dom 7) as "justified gaps —
follow-up change"; that follow-up is now *this* cycle. Finally, the backend `api/` package
is a flat bag of handler files and the UI organizes `pages/*` separately from their
hooks/utils — both need bounded module structure to keep the rework maintainable.

## What Changes

Cycle 2 revises this change on top of the cycle-1 delta. Everything cycle 1 specified
stays in place; cycle 2 adds and amends the following:

- **Landing page redesign.** The app root (`/`) is a single landing page — never a list
  view. Top-to-bottom: (a) a large, prominent **search bar**;, (b) a big centered **[+]**
  button that, when clicked/touched, reveals **📷 Camera / 📎 Upload / ✎ Text** options —
  the user picks one and provides input *without pre-selecting document type or account*;
  (c) a labeled **Insights** placeholder section below (visually present, intentionally
  empty this cycle). A **hamburger menu icon** on every screen opens the navigation sheet
  (no permanent sidebar anywhere). Specific views (asset detail, search results, review
  queue, documents, finance) hang off that navigation.
- **Framework pass.** Re-audit the stack against upstream *at implementation time*
  (verified current facts as of this spec: Vite 8.2.2, Tailwind 4.3, shadcn CLI 4.8.x,
  TanStack Table v8 docs line, react-router 7 — which *is* upstream Remix's successor,
  confirmed at remix.run; Next.js 15 verified; SvelteKit version line unproven this
  session). The decision recorded in cycle 1 (Vite + React + react-router, SPA vs Go API)
  stands — SSR brings no benefit to an authenticated personal app, SvelteKit would be a
  full rewrite — and the framework audit now *also* **adopts the newer stable majors**
  (Vite 8, TypeScript 7, vitest 5, plugin-react, jsdom, jest-dom) unless a gap is
  re-justified. The framework/major audit is re-executed with two outcomes: swap (with
  rationale) or keep + upgrade majors (majority of work is UI quality, not framework). UI
  quality bar raised to: mobile-first polish that reads as a native-feeling app (touch
  targets ≥44px, cards at small widths, tokens/transitions, dark mode), desktop perfectly
  usable — not "an html page".
- **Modularization.** Backend: flat `api/` splits into domain subpackages
  (`api/assets`, `api/add`, `api/search`, `api/lifecycle`, …); `cmd`, `gen`, contract
  files, and composition stay at root. `core/` subpackages (identity, processing,
  lifecycle, search, review, statement) remain bounded — verified, not re-split. UI:
  `pages/*` reorganizes into feature folders (`features/add`, `features/search`,
  `features/assets`, `features/reviews`, `features/finance` …) with co-located hooks and
  components; `components/` holds shared primitives only.
- **Documents section (stuck-document fix).** A new top-level view lists *every ingested
  source* (file or text) with its processing status — `processed` (linked to an asset),
  `in review` (held), `failed`/`asset-less` (no asset link) — plus the linked asset name
  when present, and per-row actions: **reprocess** (with optional comment) and **delete**
  (soft-delete, consistent with asset lifecycle). This is where stuck/failed uploads
  become visible and recoverable.
- **Reprocess / edit / delete actions.** Assets: reprocess (re-runs extraction over its
  linked documents), edit data (extended PATCH across structured fields, sticky on
  user-set fields — same user-set category rule generalized). Documents: reprocess (on the
  retained source; accompanying text may be upserted onto that source instead of a new
  one — design detail deferred) and soft-delete. These are the direct remedy for the
  stuck-doc bug.
- **Duplicate-upload reprocess prompt.** When a duplicate (same content hash) is uploaded,
  the outcome is no longer a silent `duplicate` badge: the add flow presents an explicit
  choice — **Reprocess** (re-run extraction on the existing source) vs **Keep existing
  result** — with the existing document/asset refs shown.
- **UX test plan gate.** The implementation plan (subspec) **must** include a UX-oriented
  test plan, and implementation is **not signed off until it passes**. It covers: mobile
  (360px) and desktop; all landing flows (camera / upload / text, with accompanying-text
  variations); search-as-nav from the landing root; dark mode on/off; the Documents
  section listing/status/actions; reprocess/delete/edit flows; the duplicate reprocess
  prompt; and the general "snappy, modern, native-feeling" quality bar (no desktop tables
  on phones, ≥44px touch targets, polished tokens).
- **Dark mode toggle.** A theme toggle (light / dark / system) is added to the app shell
  (hamburger area), class-strategy tokens per current Tailwind v4 + shadcn practice
  (`@custom-variant dark`), persisted, applied flash-free.
- **Companion text.** An added item may be file(s) **with accompanying text**, or text
  alone — all flow through the same pipeline; text is optional additional input, never a
  requirement to describe the document first.

## Capabilities

### New Capabilities

- `landing-experience`: root landing page; search-bar-top / [+]-center / Insights-bottom
  layout; add picker (camera/upload/text) with no pre-selection and optional companion
  text; hamburger-only navigation; deferred account selection for statements; Insights
  placeholder.
- `document-management`: all-sources documents section with status joins; reprocess
  document / reprocess asset; soft-delete document; duplicate reprocess prompt semantics;
  optional reprocess comment.
- `code-organization`: backend `api/` domain subpackages; `core/` boundary verification;
  UI feature-folder layout with co-located hooks/components.

### Modified Capabilities

- `web-platform`: framework-swap evaluation embedded in the audit (keep swap vs adopt
  majors decision recorded); dark-mode toggle; mobile-first quality bar; UX test-plan gate.
- `unified-add`: no pre-selection of document type/account; accompanying text accepted
  with files and alone; duplicate outcome carries the prompt contract.
- `search-experience`: the primary search bar moves from shell-persistent to the landing
  root route; shell keeps a hamburger-only nav.
- `asset-registry`: extended asset edit (data fields across the structured core, sticky
  user-set semantics generalized).
- `asset-lifecycle`: document soft-delete + retention restore; reprocess document and
  reprocess asset lifecycle actions.
- `document-ingestion`: duplicate-upload prompt contract (reprocess-vs-keep); companion
  text items.
- `api-contract`: documents list/status endpoint; document delete + reprocess endpoints;
  asset reprocess endpoint; asset-edit payload extension.

## Out of Scope

- **Mobile/native apps** — the UI is still responsive mobile-first web only; "native" here
  means native-feeling UX, not a packaged app.
- **Chat assistant interface** — the user has expressed interest in a bottom-of-landing
  chat with the LLM; deferred to a later iteration and recorded here for visibility.
- **Historical data backfill** of name/category for legacy assets (unchanged).
- **Purge of source bytes on document deletion** — document delete soft-deletes the
  document row; source blobs are retained by policy (retention restores within window).
- **Ledger behavior changes** (unchanged).
- **Email watcher / automatic ingestion sources** (unchanged).
- **Hard-delete / un-merge / per-field confidence / pg_trgm** (unchanged).
- **Insights functionality** — the landing section is a labeled placeholder this cycle.

## Impact

- **Backend**: `api/` restructured to subpackages (same operations, same generated
  surface); three new operation families (documents list/status, document delete +
  reprocess, asset reprocess); PATCH asset widened to the structured core; migration(s)
  for document soft-delete (`documents.deleted_at`) and any search/index needs for the
  documents-section join; the existing backend domain logic (extraction, consensus,
  dedupe, identity, search) is **kept** — only package structure and read paths change.
- **UI**: routes reparented (`/` landing; `/documents` new; `/add` may alias to landing
  but root is canonical); feature-folder reorg; framework majors upgraded per audit;
  hamburger-only shell; dark-mode tokens + next-themes (or equivalent verified default);
  new landing, documents, and prompt components; `DEPS-AUDIT.md` rewritten with
  framework-swap rationale.
- **Specs**: proposals + delta specs gain the three new capabilities and the seven
  modified ones above; `asset-ui` designer subspec picks up the UX test-plan gate.
- **Non-breaking**: API surface remains additive; generated lockstep preserved. Route
  moves (`/` → landing) follow versioned client behavior (old deep links `/assets` still
  resolve).
