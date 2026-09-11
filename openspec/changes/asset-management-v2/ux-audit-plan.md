# UX Audit Plan — asset-management-v2

| | |
|---|---|
| **Phase** | APPLY (gate) |
| **Change** | `asset-management-v2` |
| **Task** | 15.1 (update the UX audit matrix for delta 2) |
| **Date** | 2026-09-11 |
| **Design ref** | D8-D11 — UX test plan + delta-2 decisions |
| **Spec ref** | `app-chrome, intuitive-input, landing-page, ux-test-plan` |

## Purpose

This document is a **PLAN** — a checklist *matrix* of audit rows, not an execution and not code.
Every status cell below starts **pending (`—`)**; nothing here has been run yet. It is the **apply
gate** required by design **D8** and the **`ux-test-plan`** spec: the change is not complete until
every row of this matrix has been executed and passes. **Task 11.2** executes every row (automated
rows via vitest / browser checks, manual rows traced with screenshots), **task 11.3** fixes any FAIL
and re-verifies it to green, and **task 12.1** may only sign the change off as *applied* once this
matrix is fully green. This plan is what makes that gate concrete and verifiable.

**Delta 2 supersedes the delta-1 audit matrix for the chrome, landing, and ingest flows.** The delta-1
rows covering **F1 (Landing page)**, **F2 (Hamburger ☰ navigation sheet)**, **F3 (Hamburger ☰ nav
links)**, and **F4 ([+] ingest cards)** are now marked **SUPERSEDED** — they are retained in this file
as historical reference only and are **not** executed against the delta-2 implementation. The delta-2
behavior for those flows is captured by the new **F12 (route surface)**, **F13 (chrome)**, and **F14/F15
(context [+] + unified composer)** rows. All other delta-1 rows (documents, finance, reviews,
duplicate-prompt, dark mode, search, asset detail) were outside the delta-2 scope and are **carried
over unchanged** from delta 1, retaining their delta-1 audit units exactly.

## Delta-2 Supersession

### Superseded by delta 2 (do NOT execute these rows)

| Old flow | Why superseded | Replaced by |
|---|---|---|
| F1 — Landing page | Delta-2 landing has hero wordmark + unified composer (no card grid) | F13 (chrome) + F15 (composer) |
| F2 — Hamburger ☰ navigation sheet | Delta-2: top-left ☰, sheet absorbs profile+Home+theme | F13 (chrome) |
| F3 — Hamburger ☰ nav links | Delta-2: Home link added, nav order changed | F13 (chrome) |
| F4 — [+] ingest cards | Delta-2: three-card grid removed, replaced by unified AddComposer | F15 (composer) |

### Carried over from delta 1 (execute as-is)

| Flow | Note |
|---|---|
| F5 — Search flow | Unchanged by delta 2 |
| F6 — Asset detail / edit | Unchanged by delta 2 |
| F7 — Documents | Carried over; corner [+] composer (F15) is the new entry point but list/filter/reprocess/delete/edit-asset actions are unchanged |
| F8 — Finance | Carried over; corner [+] composer (F14) is the new entry point but accounts/movements/import views are unchanged |
| F9 — Reviews queue | Carried over; corner [+] composer (F14) is the new entry point but accept/discard actions are unchanged |
| F10 — Duplicate-prompt | Carried over; the dialog is reused (not rewritten) by the composer |
| F11 — Dark mode / theme control | Carried over; theme toggle moved INTO the sheet (F13) but the dark-mode-on-every-flow requirement is unchanged |

## Scope & preconditions

**Viewports** (design D8 + spec):
- `360×640` (mobile) — **required**
- `≥1280` (desktop) — **required**
- `768` (mid-size) — **optional**, included as clearly-marked `(opt)` columns below

**Modes:** `light` and `dark`. Dark mode applies to **every** flow — the dark column of each flow
table *is* the "dark mode on every flow" coverage (see F11c).

**Precondition — all 8 feature flows are implemented and auditable:** landing, hamburger nav sheet +
nav links, [+] ingest cards, search, asset detail/edit, documents, finance, reviews, duplicate-prompt,
and dark mode. One sub-unit is **DEFERRED** (F7f — no backend endpoint exists; see state.md
Blockers) and is tracked there rather than counted as a pass.

**Evidence output dir:** `.autopilot/change-asset-management-v2/ux-evidence/<flow>/` — manual
rows save screenshots here; browser-automated rows save Lighthouse/axe/screenshot/overflow output here.

**Method split:** rows that an existing vitest suite already covers are marked `A` (and the covering
test file is cited). Visual / overflow / contrast / focus / theme-shift / a11y rows the orchestrator
drives in a real browser are `B`. Anything needing a human judgment call with a screenshot is `M`.
Automatable rows are covered by vitest / browser (Lighthouse/axe/screenshot) where feasible; the rest
are manual-with-screenshots.

## Matrix legend

**Method tags** (every cell carries exactly one):
- `A` — **automated** by an existing vitest test (the covering test file is cited in the
  per-flow "Automated by" note beneath each table; do not bloat the cells).
- `B` — **browser-automated** (chrome-devtools / playwright): Lighthouse a11y, axe, screenshot,
  `scrollWidth <= 360` overflow check, `:focus-visible` check, theme no-jump check.
- `M` — **manual-with-screenshots**: a human judgment call captured with a screenshot into the
  evidence dir.

**Status symbols:**
- `—` — **pending** (this is the plan; nothing run yet)
- `PASS` — executed and green (set by task 11.2/11.3)
- `FAIL` — executed and red (blocks apply; must be fixed + re-verified by task 11.3)

A cell reads `<method tag>: —` (e.g. `A: —`, `B: —`, `M: —`). **Each audit-unit row is crossed with the
viewport × mode grid**: `360×640 L`, `360×640 D`, `≥1280 L`, `≥1280 D` are the four **required** cells;
`768 L (opt)` / `768 D (opt)` are the two optional cells. The **dark columns of F1–F10** constitute the
"dark mode on every flow" requirement (F11c states this explicitly).

> **F7f (asset-reprocess) is DEFERRED** — there is no backend endpoint. It is present in the matrix
> below but flagged `DEFERRED / no-endpoint` (blocked-pending-backend); it is tracked in state.md
> Blockers and **not counted as a pass**.

---

## The audit matrix

### F1 — Landing page (`/`, `ui/src/features/landing/landing-page.tsx`)

> **SUPERSEDED by delta 2** — see F13 (chrome) + F15 (composer). Do not execute these rows.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F1a | Landing renders: search bar + centered [+] button + labeled Insights placeholder + fixed disabled chat pill + NO sidebar/nav rail | A: — | B: — | A: — | B: — | B: — | B: — |
| F1b | Search submit navigates to `/search?q=<term>` | A: — | A: — | A: — | A: — | B: — | B: — |
| F1c | [+] toggles the ingest-cards reveal (aria-expanded false→true, idempotent) | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/landing/landing-page.test.tsx` — regions + no persistent nav,
labeled Insights placeholder, disabled chat pill, search → `/search?q=`, [+] reveal toggle, 3-up `lg`
grid, axe-clean. `A` cells = render/behavior logic; `B` cells = visual/overflow/contrast/a11y.

### F2 — Hamburger ☰ navigation sheet (`ui/src/components/nav-sheet/`)

> **SUPERSEDED by delta 2** — see F13 (chrome). Do not execute these rows.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F2a | ☰ trigger visible top-right on EVERY view (incl. landing `/`) | A: — | B: — | A: — | B: — | B: — | B: — |
| F2b | ☰ opens a slide-in sheet overlaying content from the right | A: — | B: — | A: — | B: — | B: — | B: — |
| F2c | Dismiss via scrim / ✕ / link tap, focus returns to the ☰ trigger | A: — | B: — | A: — | B: — | B: — | B: — |
| F2d | Profile row (circular avatar + user name) opens the active-user/profile context | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/components/nav-sheet/nav-sheet.test.tsx` — ☰ opens on `/` and `/assets`,
profile row (active user / "Not signed in"), 4 nav links in order, 3 inert TBD items, ✕/Escape dismiss
+ focus restore, profile-row tap opens switcher, axe-clean closed + open. `ui/src/components/layout/app-shell.test.tsx`
— hamburger present on `/`, reach every screen from the sheet, active-user switcher, axe-clean.

### F3 — Hamburger ☰ nav links (each must LAND its view and dismiss the sheet)

> **SUPERSEDED by delta 2** — see F13 (chrome). Do not execute these rows.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F3a | Review Queue link → `/ingest/reviews` | A: — | B: — | A: — | B: — | B: — | B: — |
| F3b | Assets link → `/assets` | A: — | B: — | A: — | B: — | B: — | B: — |
| F3c | Documents link → `/documents` | A: — | B: — | A: — | B: — | B: — | B: — |
| F3d | Accounts link → `/finance/accounts` | A: — | B: — | A: — | B: — | B: — | B: — |
| F3e | Disabled TBD items (Subscriptions / Relationships / Inventory) visible but inert (no navigation) | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/components/nav-sheet/nav-sheet.test.tsx` — Review Queue + Accounts navigate
and dismiss; 3 TBD items present but not links (inert). `ui/src/components/layout/app-shell.test.tsx` —
reaches every primary screen from the nav sheet without a reload; navigates via the sheet and closes it
on navigation. (Assets/Documents landing asserted via the "reaches every primary screen" suite +
`A` render of each target page.)

### F4 — [+] ingest cards (`ui/src/features/landing/cards/`)

> **SUPERSEDED by delta 2** — see F15 (unified composer). Do not execute these rows.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F4a | Camera/Image card reveals (capture/pick) with inline optional note field | A: — | B: — | A: — | B: — | B: — | B: — |
| F4b | Document card reveals (pick PDF/allowed format) with inline optional note field | A: — | B: — | A: — | B: — | B: — | B: — |
| F4c | Text card reveals (own text entry, no file required) | A: — | B: — | A: — | B: — | B: — | B: — |
| F4d | Inline note present → travels as the ingestion `user_directive` | A: — | A: — | A: — | A: — | B: — | B: — |
| F4e | Inline note absent → ingestion proceeds with empty directive | A: — | A: — | A: — | A: — | B: — | B: — |
| F4f | Text-only ingest works (no file required) | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/landing/cards/ingest-cards.test.tsx` — Camera capture input,
Document PDF+image input, Text textarea (no file input), all 3 cards present; note → directive (Camera +
Document), file card with no note omits the field (empty directive); text-only ingest (no files/account),
Add disabled until non-empty. **NOTE (verify inline-only):** no pre-selection modal, no separate form
step — the cards are inline-only (asserted by the "card reveal per mode" suite, which reveals the
inline fields directly with no intermediate modal/step).

### F5 — Search flow (`ui/src/features/search/`)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F5a | Search → `/search?q=` → results page renders (heading + count + hits) | A: — | B: — | A: — | B: — | B: — | B: — |
| F5b | Selecting a result navigates to the asset detail view | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/search/search-results-page.test.tsx` — loading/error/no-results
states, results list, query in heading, type badges, total count + page info, pagination, links to
resources. F5b: result rows render `<Link>` to the asset detail (`search-results-page.tsx:91`); the
landing→search hand-off is covered by `ui/src/features/landing/landing-page.test.tsx` (search →
`/search?q=`). `A` = render/behavior; `B` = visual/overflow/a11y.

### F6 — Asset detail / edit (`ui/src/features/assets/`)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F6a | Asset detail renders (fields, linked documents) | A: — | B: — | A: — | B: — | B: — | B: — |
| F6b | Edit asset fields persists (update path) and reflects in UI | B: — | B: — | B: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/assets/asset-detail-page.test.tsx` — renders asset fields correctly,
renders document list, not-found state. (The asset *update/patch* path is exercised by
`ui/src/features/documents/documents-page.test.tsx` "Edit asset … usePatchAsset" for the
document-side edit affordance.) **F6b is `B` (browser) end-to-end** because the detail-page vitest
suite covers render, not the full persist-and-reflect round-trip; verify the update path + UI reflection
in the browser.

### F7 — Documents (`ui/src/features/documents/`)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F7a | Documents list (filename + upload date + 4-state status badge) | A: — | B: — | A: — | B: — | B: — | B: — |
| F7b | Status filter + filename search | A: — | B: — | A: — | B: — | B: — | B: — |
| F7c | Reprocess with optional comment (409 in-flight → friendly toast) | A: — | B: — | A: — | B: — | B: — | B: — |
| F7d | Delete with confirm (ConfirmDialog) | A: — | B: — | A: — | B: — | B: — | B: — |
| F7e | Edit asset via the asset-update path (processed rows) | A: — | B: — | A: — | B: — | B: — | B: — |
| F7f | Asset-reprocess — **DEFERRED: no backend endpoint exists** (see state.md Blockers) | DEFERRED / no-endpoint | DEFERRED / no-endpoint | DEFERRED / no-endpoint | DEFERRED / no-endpoint | (opt) n/a | (opt) n/a |
| F7g | Asset link on processed rows → asset detail | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/documents/documents-page.test.tsx` — loading/error states, rows
(filename + upload date, axe-clean), 4 status badges, processed asset link → `/assets/{id}`, status
filter → `useDocuments`, filename search → `q`, empty CTA, row-menu affordances (processed: Edit asset +
Open asset, no Reprocess; failed: Reprocess + Delete; in_review: Reprocess disabled + Delete), Delete
confirm dialog → `useDeleteDocument` + toast, Reprocess comment dialog (+empty-note omission),
Reprocess 409 friendly toast, Edit asset → `usePatchAsset`, Open asset → `/assets/{id}`.
`ui/src/features/documents/hooks.test.tsx` — `useDocuments` filter keys, `useDeleteDocument`,
`useReprocessDocument` (comment pass/omit, 409 in-flight). **F7f is DEFERRED** (`DEFERRED /
no-endpoint`), tracked in state.md Blockers — present in the matrix but **not** pass/fail and **not**
counted as a pass.

### F8 — Finance (`ui/src/features/finance/`)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F8a | Accounts view renders | A: — | B: — | A: — | B: — | B: — | B: — |
| F8b | Movements view renders | A: — | B: — | A: — | B: — | B: — | B: — |
| F8c | Import flow (statement import + import history) | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/finance/accounts-page.test.tsx` — account list w/ balance, empty
form rejection, server-400 error. `ui/src/features/finance/movements-page.test.tsx` — movements w/
signed amount per kind, occurred date, description, origin badge, no delete for imported, filters
`account_id/from/to`, axe-clean. `ui/src/features/finance/import-page.test.tsx` — renders upload form.
`ui/src/features/finance/import-history-page.test.tsx` — empty state, list of batches.

### F9 — Reviews queue accept/discard (`ui/src/features/reviews/`)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F9a | Pending reviews listed (default filter) | A: — | B: — | A: — | B: — | B: — | B: — |
| F9b | Accept/approve a review (confirm → success toast) | A: — | B: — | A: — | B: — | B: — | B: — |
| F9c | Discard/reject a review (confirm → toast) | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/reviews/review-queue-page.test.tsx` — loading/error/empty, reviews
list, confidence / best-matched, status badges, approve + reject buttons for pending only, approve →
confirm dialog → mutation, reject → confirm dialog → mutation, cancel does not fire, 409 conflict on
approve, 404 on reject, refetch after approve/reject, aria-labels, `role="alertdialog"` confirm.

### F10 — Duplicate-prompt (`ui/src/features/landing/cards/duplicate-prompt-dialog.tsx`)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F10a | Duplicate upload → 409 `DuplicateReport` → prompt modal appears (reprocess/keep choices, `expires_at`) | A: — | B: — | A: — | B: — | B: — | B: — |
| F10b | Reprocess choice | A: — | B: — | A: — | B: — | B: — | B: — |
| F10c | Keep choice | A: — | B: — | A: — | B: — | B: — | B: — |
| F10d | Timeout (10-min auto-keep → verbatim `timeout_toast` "no response — keeping existing document") | A: — | M: — | A: — | M: — | M: — | M: — |

**Automated by:** `ui/src/features/landing/cards/ingest-cards.test.tsx` — 409 opens the dialog showing
filename + upload date + both buttons (F10a), Reprocess → reprocess hook w/ parsed doc id (F10b), Keep
→ keep hook w/ parsed doc id (F10c), keep-default on timeout: auto-keeps and toasts the **verbatim**
`timeout_toast` when the prompt expires unchosen (F10d), dialog passes axe. F10d also `M` for the
human check that the toast wording reads exactly
"no response — keeping existing document" (screenshot of the toast).

### F11 — Dark mode / theme control (cross-cuts every flow)

> **Carried over from delta 1** — unchanged by delta 2. Execute as-is.

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F11a | Theme toggle present on app chrome AND landing (light/dark/system) | A: — | B: — | A: — | B: — | B: — | B: — |
| F11b | Switching theme → no layout shift / no pre-paint flash (applied before first paint) | B: — | B: — | B: — | B: — | B: — | B: — |
| F11c | Dark mode renders EVERY other flow correctly | B: — | B: — | B: — | B: — | B: — | B: — |

**Automated by:** `ui/src/context/theme-provider.test.tsx` — defaults to system, `setTheme("dark"/"light")`
persists + switches the `html` class, persisted preference on reload, unrecognized → system, dark on
fresh mount when OS dark, system follows OS scheme change live, no follow when fixed, unsubscribes on
unmount (covers F11b logic + persistence). `ui/src/components/ui/theme-toggle.test.tsx` — accessible
name + default label, Dark/Light selection persists + switches class, three mode options (light/dark/system),
44px touch-target minimum, axe-clean (covers F11a). **F11c (stated explicitly):** the **dark columns of
F1–F10 ARE the "dark mode on every flow" coverage** — F11c passes when every `-D` cell of F1–F10 is
`PASS`. F11b is `B` (browser) end-to-end: capture before/after theme switch to assert no layout shift
and no pre-paint flash.

### F12 — Route surface walk (`ui/src/router.tsx`, design D9 / app-chrome spec)

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F12a | Legacy `/add` redirects to `/` (not a stale page, not a blank shell) | A: — | B: — | A: — | B: — | B: — | B: — |
| F12b | Legacy `/finance/movements` redirects to `/finance/accounts` | A: — | B: — | A: — | B: — | B: — | B: — |
| F12c | Legacy `/finance/import` redirects to `/finance/accounts` | A: — | B: — | A: — | B: — | B: — | B: — |
| F12d | Legacy `/finance/import/history` redirects to `/finance/accounts` | A: — | B: — | A: — | B: — | B: — | B: — |
| F12e | Legacy `/finance/import/:batchId` redirects to `/finance/accounts` | A: — | B: — | A: — | B: — | B: — | B: — |
| F12f | Unknown path renders NotFoundPage (not a blank shell, not a stale page) | A: — | B: — | A: — | B: — | B: — | B: — |
| F12g | No dead links: walk every link in app shell, nav sheet, and all reachable pages — every link resolves to a reachable route | M: — | M: — | M: — | M: — | M: — | M: — |
| F12h | `/assets/:id` deep-link renders the asset detail view | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/router.test.tsx` — 10/10 (route-table: 7 views render + 5 legacy redirect + not-found + `/assets/:id` deep-link). F12g (dead-link walk) is `M` — requires a human or orchestrator-driven browser walk of every link in the app.

### F13 — Chrome: no top bar + top-left ☰ (app-shell, nav-sheet, design D10 / app-chrome spec)

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F13a | No view renders a persistent horizontal top bar (the old bar is gone) | A: — | B: — | A: — | B: — | B: — | B: — |
| F13b | Top-left ☰ present on EVERY view (incl. landing `/`), ≥44px, aria-labeled | A: — | B: — | A: — | B: — | B: — | B: — |
| F13c | Sheet content order (top→bottom): profile row → Home → Review Queue / Assets / Documents / Accounts → theme toggle → 3 disabled TBD items | A: — | B: — | A: — | B: — | B: — | B: — |
| F13d | Home link is the FIRST nav item in the sheet, navigates to `/` | A: — | B: — | A: — | B: — | B: — | B: — |
| F13e | Profile row (avatar + name) is the first item in the sheet; opens active-user/profile context | A: — | B: — | A: — | B: — | B: — | B: — |
| F13f | Theme toggle (light/dark/system) is inside the sheet; NO theme toggle exists outside the sheet | A: — | B: — | A: — | B: — | B: — | B: — |
| F13g | PROCRASTINATOR hero wordmark on landing: centered above search bar, tap → `/`, ≥44px, not a bar element | A: — | B: — | A: — | B: — | B: — | B: — |
| F13h | Sheet dismisses via scrim tap / ✕ / link tap; focus returns to ☰ trigger | A: — | B: — | A: — | B: — | B: — | B: — |
| F13i | Sheet navigation (tap a link) lands on the target view AND dismisses the sheet | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/components/layout/app-shell.test.tsx` — no-header on `/` + `/assets`, ☰ fixed top-left. `ui/src/components/nav-sheet/nav-sheet.test.tsx` — 5 links with Home first, Home tap→`/`+dismiss, theme combobox in sheet. `ui/src/features/landing/landing-page.test.tsx` — five-regions-order (wordmark→search→[+]→Insights→pill) + wordmark-as-link-to-/.

### F14 — Context [+] per section (app-chrome D10 / landing-page spec)

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F14a | `/ingest/reviews` has top-right [+] → opens review creation composer (≥44px, aria-labeled "Add review") | A: — | B: — | A: — | B: — | B: — | B: — |
| F14b | `/assets` has top-right [+] → opens asset creation composer (≥44px, aria-labeled "Add asset") | A: — | B: — | A: — | B: — | B: — | B: — |
| F14c | `/documents` has top-right [+] → opens document upload composer (≥44px, aria-labeled "Add document") | A: — | B: — | A: — | B: — | B: — | B: — |
| F14d | `/finance/accounts` has top-right [+] → opens account creation composer (≥44px, aria-labeled "Add account") | A: — | B: — | A: — | B: — | B: — | B: — |
| F14e | Landing page `/` has NO top-right [+] (only the big centered [+] exists) | A: — | B: — | A: — | B: — | B: — | B: — |
| F14f | Each [+] opens the section's add flow using the unified composer pattern (NOT old-style field-by-field form) | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/docs/add-button.test.tsx` — it.each 4 labels (SR name), ≥44px class-assertion, onAdd fires once, landing exclusion (real LandingPage: centered "Add something" present + NO `button[class*="right-4"]`). F14f is `A` where the composer component tests assert the directive-first pattern; `B` for the visual "no old-style form" check.

### F15 — Unified Add composer (intuitive-input D11 / landing-page spec, `features/docs/composer/`)

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F15a | Landing [+] opens the unified AddComposer (bottom sheet at 360px / inline dialog ≥sm) | A: — | B: — | A: — | B: — | B: — | B: — |
| F15b | Composer first interaction: ONE primary describe-input (placeholder "Describe it…") — NOT a multi-field form | A: — | B: — | A: — | B: — | B: — | B: — |
| F15c | Attach strip: 📷 camera / 📁 file picker / ⌨ paste-text — any combination usable in one session | A: — | B: — | A: — | B: — | B: — | B: — |
| F15d | Optional directive note field ("Anything the scanner should know?") is always available in the composer | A: — | A: — | A: — | A: — | B: — | B: — |
| F15e | Attached fragments render as removable chips in-session; removing a chip updates the composition before submit | A: — | B: — | A: — | B: — | B: — | B: — |
| F15f | Submit with file → POST to documents endpoint (multipart); pipeline runs (classification/extraction) | A: — | A: — | A: — | A: — | B: — | B: — |
| F15g | Submit with paste-text only (no file) → text ingest path; no file required | A: — | A: — | A: — | A: — | B: — | B: — |
| F15h | Submit with camera capture → image ingest path (same upload/ingestion path as file) | A: — | B: — | A: — | B: — | B: — | B: — |
| F15i | Upload + note: both, either, or neither — all combinations work (file+note, file-only, note-only) | A: — | A: — | A: — | A: — | B: — | B: — |
| F15j | No 360px horizontal overflow in the composer (bottom sheet mode) | B: — | B: — | B: — | B: — | B: — | B: — |
| F15k | Section [+] composers (assets, documents, reviews, finance) all use the same unified composer pattern (not per-section forms) | A: — | B: — | A: — | B: — | B: — | B: — |
| F15l | 409 DuplicateReport from composer → existing DuplicatePromptDialog (reprocess/keep) — not a new dialog | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/docs/composer/add-composer.test.tsx` — upload surface, camera action, note field, removable chips, file-only submit, text-only submit, 409→DuplicatePromptDialog. `ui/src/features/landing/landing-page.test.tsx` — [+] opens AddComposer. F15j (overflow) is `B` (browser scrollWidth check). F15k (section composers) is `A` where composer tests assert the pattern; `B` for visual "no old-style form" check.

### F16 — Review chips pattern (intuitive-input D11, `features/docs/composer/review-chips.tsx`)

| Unit | Audit unit / what to verify | 360×640 L | 360×640 D | ≥1280 L | ≥1280 D | 768 L (opt) | 768 D (opt) |
|---|---|---|---|---|---|---|---|
| F16a | After submit + extraction, review step renders extracted fields as confirmable chips (brand, model, serial, price…) — not a blank multi-field form | A: — | B: — | A: — | B: — | B: — | B: — |
| F16b | Tap a chip → reveals an input to correct the value; Done confirms the correction | A: — | B: — | A: — | B: — | B: — | B: — |
| F16c | Unextracted optional fields are collapsed behind a single "Add details" disclosure — NOT pre-rendered as empty boxes | A: — | B: — | A: — | B: — | B: — | B: — |
| F16d | User can save unedited (tap Save without editing any chip) → entity created with extracted values | A: — | B: — | A: — | B: — | B: — | B: — |
| F16e | No add/creation flow in the app opens with a multi-field form as its first interaction | M: — | M: — | M: — | M: — | M: — | M: — |
| F16f | Review chips pattern works for asset, account, review, and document creation flows | A: — | B: — | A: — | B: — | B: — | B: — |

**Automated by:** `ui/src/features/docs/composer/review-chips.test.tsx` — 21 tests (chips/conf, tap-reveal, correct, unedited-save, add-details collapse/expand + empty-omit, cancel, busy, title, toReviewChips, axe collapsed+expanded). F16e (no old-style form anywhere) is `M` — requires a human/orchestrator walk of every add flow to confirm the first interaction is a directive input, not a field grid.

> **Note:** F12–F16 automated-by notes cite the vitest suites that cover each audit unit where known.
> Rows not yet mapped to a concrete vitest file are `B` (browser) or `M` (manual) and remain **TBD — to
> be covered by vitest/browser checks (task 11.2)**.

---

## Quality bar matrix

The 8 quality-bar criteria (spec wording, numbered QB-1 … QB-8). Every required flow must satisfy the
criteria that gate it.

| ID | Criterion (spec wording) | Gates which flows | How verified |
|---|---|---|---|
| QB-1 | No horizontal overflow at 360px | all flows | `B: —` overflow check `scrollWidth <= 360` at `360×640` (light + dark) |
| QB-2 | ≥40px touch targets | all flows (esp. F1, F2, F4, F10) | `B: —` measure interactive hit-targets ≥40px at `360×640`; `A` where a test asserts it (theme-toggle 44px test) |
| QB-3 | Readable contrast in light AND dark (no unreadable contrast in either theme) | all flows — both `-L` and `-D` columns | `M: —` + `B: —` (axe/contrast) on **both** `-L` and `-D` cells of every flow; human contrast-readability judgment |
| QB-4 | No layout shift on theme/system change (no theme-shift layout jump) | F11 + all `-D` rows | `B: —` before/after theme-switch capture; `A` theme-provider (class applied pre-paint) + live system-follow |
| QB-5 | Loading skeletons for async data | F1, F5, F6, F7, F8, F9 (any async list/detail) | `A: —` render tests assert a loading state; `B: —` screenshot of the skeleton while data in flight |
| QB-6 | Visible keyboard focus on desktop | desktop-only F1–F16 (`≥1280` L + D) | `B: —` `:focus-visible` check at `≥1280`; Tab through each view and confirm a visible focus ring |
| QB-7 | Toasts confirm destructive-adjacent actions | F7c, F7d, F9b, F9c, F10 (reprocess/delete/accept/discard/dup-choices) | `A: —` tests assert the toast fires on the action; `M: —` screenshot of the confirm toast wording |
| QB-8 | No view renders a persistent top bar (delta-2 chrome) | F13, all flows | `B: —` assert 0 `<header>` elements on every route at 360px and ≥1280px; `A` where app-shell tests assert no-header |

## Gate (apply signoff wiring)

**This matrix is the apply gate.** It is wired across tasks 11.1 → 11.2 → 11.3 → 12.1:

- **Task 11.2 — Execute:** run every row. Automated rows (`A`) via vitest, browser rows (`B`) via
  chrome-devtools/playwright (Lighthouse a11y, axe, screenshot, overflow check), manual rows (`M`)
  traced by a human at **360px + desktop in light/dark** with screenshots captured into
  `.autopilot/change-asset-management-v2/ux-evidence/<flow>/`. Record `PASS`/`FAIL` next to each row.
- **Task 11.3 — Fix:** any `FAIL` row is fixed and **re-verified to `PASS`** (re-execute the row; the
  quality bar must hold: no 360px horizontal overflow, ≥40px touch targets, readable contrast, no
  theme-shift layout jump, loading skeletons, visible keyboard focus, toasts on destructive-adjacent
  actions).
- **Task 12.1 — Signoff:** may only be marked **applied** when **every required row is `PASS`**. The
  **DEFERRED F7f** row is tracked in state.md Blockers and is **not counted as a pass** (it is excluded
  from the pass tally, not a `FAIL`). Any single `FAIL` blocks apply — the change is not applied until
  that row is fixed and re-verified.

**Signoff checklist (12.1):**
- [ ] all required rows PASS: carried-over F5a–F11c (excluding DEFERRED F7f) + new delta-2 F12a–F16f; superseded F1a–F4f are excluded from the pass tally (marked SUPERSEDED)
- [ ] quality bar QB-1 … QB-8 all hold
- [ ] screenshots present for all `M` rows (in `.autopilot/change-asset-management-v2/ux-evidence/`)
- [ ] no un-fixed `FAIL` row

**Delta-2 spec satisfaction:** The DELTA-2 scenario spec (`ux-test-plan` 'Delta-2 chrome rows are audited' scenario) is satisfied when F12, F13, F14, F15, and F16 are all present and executed.

## Non-goals

This is a **plan only**:
- **No code, no execution, no pass/fail yet** — every status cell is `—` (pending); execution is 11.2.
- **No browser e2e framework required** — there is no Playwright/Cypress suite to write; the `B` rows
  are orchestrator-driven manual/orchestrator checks (chrome-devtools/playwright: Lighthouse, axe,
  screenshots, overflow/focus/theme checks) and the `M` rows are human-judgment screenshots.
- **No performance / load testing** — out of scope for this UX gate (a11y, layout, contrast, focus,
  skeletons, toasts, theme behavior only).
- **The SUPERSEDED delta-1 rows (F1–F4) are retained for historical reference only and are excluded
  from the apply gate.** They are not executed and not counted in the pass tally.

(End of file - total 434 lines)
