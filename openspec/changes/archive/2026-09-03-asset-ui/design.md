# Design: asset-ui — Web UI for the Procrastinator personal finance app

## Context

The Go backend (`procrastinator-backend/`, port `:8080`) is complete and verified: document ingestion, asset registry, financial ledger, statement import, multitenancy. It is reachable only via raw HTTP. This change adds the single-page web frontend (`ui/`) so a non-technical user can use every user-facing capability. Stack is fixed by prior research (`.tmp/websearcher-ui-stack-research-20260825.md`): **Vite (react-ts) + Tailwind CSS v4 + shadcn/ui + React Router + TanStack Query + react-dropzone + XHR upload progress**. One responsive build serves desktop and phone browsers.

Backend wire contract was verified against the code (`.tmp/explore-api-json-contracts-20260901.md`); key facts the design depends on:

- Error envelope everywhere: `{"error":"<message>"}`.
- Money is transmitted as exact-decimal **strings** (`"39999.99"`, pattern `^[0-9]+(\.[0-9]+)?$`, unsigned; movement sign is implicit in `kind`, import-line sign in `direction`).
- `occurred_on` is a plain ISO date (`"2026-08-20"`); all other date/time fields are RFC3339Nano UTC (date-only fields arrive as UTC midnight, e.g. `"2026-08-20T00:00:00Z"`).
- Nullable fields are **key-omitted** when null (Go `omitempty`), never `"key": null`; exceptions: `link_conflicting` (always present) and `lines` (`null` in batch list responses).
- No CORS middleware — the Vite dev server **must** proxy `/api` → `http://localhost:8080`.
- Movement list filter params are exactly `account_id`, `from`, `to` (inclusive occurred-on bounds).
- Movement–document link is three flat fields on the movement (`linked_document_id`, `link_creator`, `link_conflicting`); no nested link object.
- Commit summary is exactly `{"created":N,"skipped":M}`.

## Goals / Non-Goals

**Goals:**
- Responsive SPA with client-side routes for the six screens (upload, asset list, asset detail, accounts, movements, statement import incl. history), deep-linkable, usable at 360px width with no page-level horizontal scroll.
- Explicit active-user context; every request scoped to the active user; switching users replaces all displayed data.
- Centralized data layer (TanStack Query): caching, dedup, loading/error/empty states, invalidation after mutations.
- Exact money/date rendering (no binary floats, no timezone drift).
- Destructive actions (movement delete, batch commit/discard) behind explicit confirmation.
- Every screen axe-clean (WCAG 2.1 AA ruleset) in component tests.

**Non-Goals** (per proposal):
- No backend changes (code or API surface); no auth; no document download/preview; no OCR; no household-scope UI; no import rollback.
- No SSR/SEO, no offline/PWA, no native apps.
- No E2E test framework dependency (Playwright/Cypress) in the repo — browser verification is done by the ui-impl-worker via its browser tooling against the dev server + live backend.

## Decisions

### D1 — Tenancy transport is encapsulated in the API client (spec deviation, flagged)

The delta spec says every request is scoped via the `/api/users/{userId}/…` URL path. The **backend code** (verified at `api/server.go:94-132`) is transitional: documents/assets are mounted under `/api/users/{userId}` (path tenancy), but **all finance routes are mounted at `/api/finance` and resolve the tenant from the `X-Tenant-ID` header** (fallback in `api/httpx/tenant.go`). The proposal forbids backend changes.

Decision: the API client is the only module that knows the transport. It exposes resource-level functions; screens never build URLs.

- documents/assets → `GET/POST /api/users/{userId}/…`
- finance → `/api/finance/…` with header `X-Tenant-ID: {userId}`

This preserves the spec's intent and every testable behavior (all requests scoped to the active user; user switch re-fetches; no cross-user data; the spec's concrete scenario targets `/api/users/alice/assets`, which holds). If the backend later moves finance routes under `/api/users/{userId}/finance`, only `client.ts` changes. **Flagged for reviewer:** recommend relaxing the spec wording to "scoped to the active user" or scheduling the backend route migration as a follow-up change.

### D2 — Amounts are strings end-to-end; money formatting is string-only

Monetary values are never parsed to `number`. Display formatting operates on the decimal string: group the integer part with a regex, keep fraction digits verbatim, resolve the currency symbol from a small static map (`INR ₹`, `USD $`, `EUR €`, `GBP £`) with fallback to the ISO code. `Intl.NumberFormat` is **not** used (it requires a float). Sign display is derived, not stored: `expense`/import `out` → `−`, `income`/import `in` → `+`, `transfer` → no sign (accounts shown instead). Client-side amount validation (movement form) uses the backend pattern `^[0-9]+(\.[0-9]+)?$` plus a string-level non-zero check — never `parseFloat`.

### D3 — Dates are rendered by string manipulation, never via `new Date()`

`occurred_on` renders verbatim. RFC3339 date-only fields (`purchase_date`, `warranty_end`) render as `value.slice(0, 10)`. Timestamps (`created_at`, `source_uploaded_at`, …) render as `YYYY-MM-DD HH:mm UTC` by string surgery (replace `T`, strip fraction/`Z`). Warranty-expired check compares date strings lexicographically: `warrantyEnd.slice(0,10) < todayLocalISO()` where `todayLocalISO()` is built from local `Date` parts (not `toISOString`, which is UTC). This satisfies the timezone-stability requirement by construction.

### D4 — Active user: context + localStorage; query keys carry `userId`; hard cache clear on switch

`ActiveUserProvider` holds the current user id, persists it to `localStorage`, and validates format client-side (`^[A-Za-z0-9_-]{1,64}$` per multitenancy). Every TanStack Query key includes `userId` (e.g. `['assets', userId]`, `['movements', userId, filters]`). On user switch the provider calls `queryClient.clear()` — belt-and-braces so no stale record from the previous user can render. An unregistered user surfaces the backend's `404 {"error":"unknown user"}` through the standard error state on every screen (addresses review warning 2 without a spec change).

### D5 — Data layer: TanStack Query; mutations invalidate by key prefix

Query keys: `['assets',uid]`, `['asset',uid,id]`, `['asset-docs',uid,id]`, `['accounts',uid]`, `['movements',uid,{accountId,from,to}]`, `['batches',uid]`, `['batch',uid,id]`. Invalidation map: document upload → `['assets',uid]`; account create → `['accounts',uid]`; movement create/patch/delete/link/unlink → `['movements',uid]` + `['accounts',uid]`; batch upload/commit/discard → `['batches',uid]`, `['batch',uid,id]`, and on commit also `['movements',uid]` + `['accounts',uid]`. Screens render four states (loading skeleton / error with retry / empty guidance / data); failed loads keep previous data via `placeholderData: keepPreviousData` where lists re-query (movement filters).

### D6 — Uploads use XHR, not fetch (upload progress)

`fetch` exposes no upload progress. A single helper wraps `XMLHttpRequest` with `upload.onprogress`, used by both document upload (progress required by spec) and statement upload (reuse, progress shown for free). Multi-file document uploads run with concurrency 3; each file is an independent state machine `queued → uploading → success(asset) | error(status,message)`; a failed file never blocks others; `502` and network failures offer per-file retry.

### D7 — No form library; controlled components + pure validators

Three forms (account create, movement create, description edit) have few, spec-explicit rules. Hand-rolled controlled components with exported pure validator functions keep dependencies minimal and make every validation rule directly unit-testable (incl. review warning 3: cross-currency transfer rejection). Server rejections map onto form-level error text without clearing entered values.

### D8 — shadcn/ui component set, installed once up front

`button, card, table, input, select, label, textarea, dialog, alert-dialog, badge, progress, sheet, skeleton, separator, dropdown-menu, sonner`. `alert-dialog` backs the single shared `ConfirmDialog` used by all destructive actions. shadcn/Radix primitives give keyboard-operable, labelled controls; every screen test asserts `vitest-axe` has no violations (measurable a11y bar — addresses review warning 1).

### D9 — Responsive shell: sidebar ≥ `md`, hamburger + Sheet below; card lists on small screens

App shell renders a permanent sidebar at `md+` and a top bar with hamburger opening a `Sheet` below `md`. Data listings render as tables at `md+` and stacked cards below `md` (same data components, two layouts) so nothing requires horizontal page scroll at 360px.

### D10 — Testing: Vitest + React Testing Library + vitest-axe; no msw, no Playwright

`client.ts` is tested against a stubbed `fetch`/fake XHR. Hook and screen tests `vi.mock` the client module boundary and drive RTL. Every screen test file includes an axe assertion. Verification commands: `npm run test` (vitest), `npm run build` (tsc + vite build), `npm run dev` (proxy to `:8080`). Declared here per review suggestion 3.

## Module layout and interfaces

New workspace `ui/` at repo root (no changes under `procrastinator-backend/`).

```
ui/
  index.html  package.json  vite.config.ts  tsconfig.json  components.json
  src/
    main.tsx                      # providers: QueryClient, ActiveUser, Router
    index.css                     # @import "tailwindcss"; theme tokens
    router.tsx                    # route table; "/" -> "/assets"
    lib/
      api/client.ts               # fetch wrapper: userPath/financePath, X-Tenant-ID, {"error"} parsing, ApiError{status,message}
      api/types.ts                # DTOs mirroring backend JSON (optional keys = omitted-when-null)
      api/errors.ts               # status -> human-readable copy (400/404/409/413/415/422/502/network)
      api/hooks.ts                # useAssets/useAsset/useAssetDocuments/useAccounts/useMovements/useBatches/useBatch + mutations
      api/upload.ts               # XHR multipart upload with onProgress (documents + statements)
      format/money.ts             # formatMoney(amount: string, currency: string): string; signed variant
      format/date.ts              # formatDate, formatTimestamp, isWarrantyExpired, todayLocalISO
      utils.ts                    # cn()
    context/active-user.tsx       # ActiveUserProvider, useActiveUser()
    components/
      ui/*                        # shadcn-generated (D8)
      layout/app-shell.tsx        # sidebar / sheet nav, active-user switcher, <Outlet/>
      feedback/{loading,error-state,empty-state}.tsx
      confirm-dialog.tsx
    pages/
      upload/upload-page.tsx
      assets/{asset-list-page,asset-detail-page}.tsx
      finance/{accounts-page,movements-page,movement-create-form,movement-actions,movement-link,import-page,import-history-page}.tsx
```

Routes: `/` → `/assets`; `/upload`; `/assets`; `/assets/:assetId`; `/finance/accounts`; `/finance/movements`; `/finance/import` (upload + preview + commit/discard); `/finance/import/history`; `/finance/import/:batchId` (detail; commit/discard when `preview`).

Key interfaces (signatures, not implementation):

- `client.ts`: `apiFetch<T>(path: string, init?, user: string): Promise<T>`; `userPath(user, p)`; `financePath(p)` + `X-Tenant-ID` injection; throws `ApiError { status: number | 0, message: string }` (`0` = network failure).
- `hooks.ts`: one `useQuery` per key above; mutations: `useCreateAccount`, `useCreateMovement`, `usePatchDescription`, `useDeleteMovement`, `useLinkMovement`, `useUnlinkMovement`, `useCommitBatch`, `useDiscardBatch` — each with the D5 invalidation map.
- `upload.ts`: `uploadDocument(user, file, onProgress): Promise<Asset>`; `uploadStatement(user, accountId, file, onProgress): Promise<ImportBatch>` — both XHR-based, reject with `ApiError`.
- `active-user.tsx`: `useActiveUser(): { userId: string; setUserId(id: string): void }` — `setUserId` validates format and clears the query cache.

Dev wiring: `vite.config.ts` sets `server.proxy = { '/api': 'http://localhost:8080' }` (no CORS on backend). Production: `npm run build` emits static `dist/`; the host must route `/api` to the backend (same-origin assumption, no env config).

## Risks / Trade-offs

- [Spec says URL-path tenancy for all requests; backend finance routes are header-tenancy] → D1 encapsulates transport in `client.ts`; deviation flagged to reviewer; recommend spec rewording or follow-up backend migration. Screens are unaffected either way.
- [Backend mounts no CORS headers, so the SPA cannot call the API cross-origin] → dev proxy in Vite (D10); production deploy must same-origin-proxy `/api`. Documented in module layout; no code mitigation possible from the SPA alone.
- [String-only money formatting ignores locale grouping conventions for some currencies] → accepted: grouping via regex, symbol map with ISO-code fallback; exactness is the hard requirement, locale polish is not.
- [Warranty-expired compares UTC-midnight date to viewer's local "today"] → spec scenario is phrased on "today's date"; viewer-local today is the only sensible reading; date-string comparison keeps it drift-free.
- [Movement filters re-query on every change] → `placeholderData: keepPreviousData` prevents list flicker; keys include the filter tuple so cache stays correct.
- [Concurrent uploads (3) could still saturate a slow LLM-backed endpoint] → per-file isolation and retry bound the blast radius; concurrency limit is a one-line constant.
- [Axe assertions in jsdom catch only automatable a11y issues] → accepted; manual/keyboard checks happen in the ui-impl-worker's browser pass.

## Migration Plan

Purely additive: new `ui/` directory; no backend, schema, or infra changes. Dev: `cd ui && npm install && npm run dev` with the backend on `:8080`. Production: serve `ui/dist/` statically with `/api` routed to the backend. Rollback = stop serving `dist/`.

## Open Questions

1. Spec rewording for D1 (relax "via the `/api/users/{userId}/…` URL path" to "scoped to the active user") vs. follow-up backend change moving finance routes under `/api/users/{userId}/finance` — reviewer to adjudicate; design supports both without screen changes.
2. Initial active user on first load (no last-used user in localStorage): default to a sensible placeholder (`alice` is the seeded dev user) and let the user switch — acceptable unless the reviewer objects.
