# UI Dependency Audit — asset-management-rework (Task 7.1)

Audit of every **direct** UI dependency in `ui/package.json`. For each: the **pinned**
version (resolved in `ui/package-lock.json` after `npm install`), the **latest stable**
upstream version (`npm view <pkg> version` = the `latest` dist-tag, never a pre-release
or canary), and a **justification** for any gap.

## Stack policy

Spec `web-platform` (*current latest-stable stack, no pre-release/canary, no package lags
its latest stable line without a recorded reason*; design **D12**): the five framework
majors are held on their **current stable line** — React 19, Vite 6, Tailwind 4,
TanStack Query 5, react-router 7 — because D12 explicitly rejects upgrading to newer
majors for this change ("upgrading to canary majors … violates the stable-only rule").
The newer absolute-latest majors are therefore recorded below as **justified gaps**, not
bumped: a cross-major framework migration is a separate, follow-up change. No dependency
is pinned to a pre-release, canary, beta, or experimental tag.

## Runtime dependencies (`dependencies`)

| Package | Pinned (lock) | Latest stable | Status / justification |
|---|---|---|---|
| `@fontsource-variable/geist` | 5.3.0 | 5.3.0 | current — at latest stable |
| `@tanstack/react-query` | 5.102.8 | 5.102.8 | current — TanStack Query 5, latest stable in major 5 |
| `@tanstack/react-table` | 9.2.4 | 9.2.4 | **newly added** — latest stable; the shared feature-rich DataTable base |
| `class-variance-authority` | 0.7.1 | 0.7.1 | current — at latest stable |
| `clsx` | 2.1.1 | 2.1.1 | current — at latest stable |
| `lucide-react` | 1.41.0 | 1.41.0 | **bumped 1.39.0 → 1.41.0** — latest stable in major 1 |
| `msw` | 2.15.0 | 2.15.0 | current — at latest stable (mock service worker) |
| `next-themes` | 0.4.6 | 0.4.6 | current — at latest stable (framework-agnostic theme util) |
| `radix-ui` | 1.6.7 | 1.6.7 | current — at latest stable (headless primitives) |
| `react` | 19.2.8 | 19.2.8 | current — React 19, latest stable in major 19 |
| `react-dom` | 19.2.8 | 19.2.8 | current — matches React 19.2.8 |
| `react-dropzone` | 20.1.1 | 20.1.1 | current — at latest stable (upload/dropzone primitive) |
| `react-router-dom` | 7.18.3 | 7.18.3 | current — react-router 7, latest stable in major 7 |
| `shadcn` | 4.21.0 | 4.21.0 | **bumped 4.19.1 → 4.21.0** — latest stable in major 4 (CLI, not imported at runtime) |
| `sonner` | 2.0.8 | 2.0.8 | current — at latest stable (toasts) |
| `tailwind-merge` | 3.6.0 | 3.6.0 | current — at latest stable (tailwind class merge) |
| `tw-animate-css` | 1.4.0 | 1.4.0 | current — at latest stable (Tailwind 4 animations) |

## Dev dependencies (`devDependencies`)

| Package | Pinned (lock) | Latest stable | Status / justification |
|---|---|---|---|
| `@tailwindcss/vite` | 4.3.3 | 4.3.3 | current — Tailwind 4 Vite plugin, latest stable in major 4 |
| `@testing-library/jest-dom` | 6.9.1 | 7.0.1 | **gap** — held on major 6 (resolved 6.9.1). Major 7 is a newer stable line; staying on the current major to match the existing `vitest` test setup. No pre-release used. |
| `@testing-library/react` | 16.3.3 | 16.3.3 | current — at latest stable in major 16 |
| `@types/node` | 26.4.1 | 26.4.1 | current — at latest stable in major 26 |
| `@types/react` | 19.2.18 | 19.2.18 | current — matches React 19 |
| `@types/react-dom` | 19.2.7 | 19.2.7 | current — matches React 19 |
| `@vitejs/plugin-react` | 4.7.0 | 6.1.1 | **gap** — held on major 4 (resolved 4.7.0). Major 6 targets Vite 8 and changes the plugin architecture; staying on the stable major 4 that pairs with the Vite 6 build. No pre-release used. |
| `jsdom` | 26.1.0 | 30.0.1 | **gap** — held on major 26 (latest stable in major 26 = 26.1.0). Newer majors are newer stable lines; staying on the current major used by the vitest 3 env. No pre-release used. |
| `openapi-typescript` | 7.13.0 | 7.13.0 | current — at latest stable in major 7 (codegen) |
| `orval` | 8.29.0 | 8.29.0 | **bumped 8.28.1 → 8.29.0** — latest stable in major 8 (codegen) |
| `redoc-cli` | 0.13.21 | 0.13.21 | current — at latest stable (OpenAPI docs viewer) |
| `tailwindcss` | 4.3.3 | 4.3.3 | current — Tailwind 4, latest stable in major 4 |
| `typescript` | 5.8.3 | 7.0.2 | **gap** — held on major 5.x (`~5.8.3`). TypeScript 7 is a newer major; the whole toolchain (Vite 6, vitest 3, plugin-react 4, shadcn/orval) is validated against TS 5.x. Upgrading TS majors is out of scope for this audit. No pre-release used. |
| `vite` | 6.4.3 | 8.2.2 | **gap** — held on major 6 (resolved 6.4.3 = latest stable in major 6). Vite 8 is a newer major; the task pins the UI to the current stable Vite 6 line and a Vite 6 → 8 upgrade is a separate follow-up. No pre-release used. |
| `vitest` | 3.2.7 | 5.0.0 | **gap** — held on major 3 (resolved 3.2.7 = latest stable in major 3). vitest 5 is a newer major; staying on the stable major 3 that pairs with the Vite 6 build. No pre-release used. |
| `vitest-axe` | 0.1.0 | 0.1.0 | current — at latest stable (axe integration for vitest) |

## Summary

- **Five framework majors confirmed on current stable lines and lockfile-pinned:**
  React `19.2.8`, Vite `6.4.3`, Tailwind `4.3.3`, TanStack Query `5.102.8`,
  react-router `7.18.3`.
- **New dependency added at latest stable:** `@tanstack/react-table` `9.2.4`
  (lockfile-pinned) — the shared feature-rich DataTable base for tasks 7.3/8.x.
- **Bumped to latest stable within their current major:** `lucide-react` → `1.41.0`,
  `shadcn` → `4.21.0`, `orval` → `8.29.0`.
- **Recorded justified gaps** (newer stable majors intentionally not adopted for this
  change, per D12): `vite` (8.2.2), `typescript` (7.0.2), `vitest` (5.0.0),
  `@vitejs/plugin-react` (6.1.1), `jsdom` (30.0.1), `@testing-library/jest-dom` (7.0.1).
- **No dependency** is pinned to a pre-release, canary, beta, or experimental tag.

## Build note (for the orchestrator)

`npm install` succeeds and `@tanstack/react-table` 9.2.4 is installed. `npm run build`
(`tsc -b && vite build`) reports two **pre-existing, out-of-scope** errors that exist at
the committed baseline (HEAD) independent of this change:
- `src/pages/reviews/review-queue-page.test.tsx:42,56` — excess property `raw_extraction`
  (the field is not part of the review schema type; owned by the review-queue rework task).
- `src/pages/search/search-results-page.test.tsx:3` — unused import `waitFor`
  (owned by the search-results task).

These are tracked files owned by later UI tasks (7.3/7.4/8.x) and are intentionally left
untouched by 7.1. The 7.1-owned deliverables (dependency manifest + lockfile + this audit,
plus the newly-added `@tanstack/react-table`) are all in place and install cleanly.
