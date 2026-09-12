# modular-structure — Delta

## ADDED Requirements

### Requirement: Backend api package is split by feature

The flat `procrastinator-backend/api` package (16 mixed files: add, search, lifecycle, reviews, finance_*, households, handlers, server, dto, errors, tools) SHALL be split into feature subpackages — `api/assets`, `api/add`, `api/search`, `api/documents`, `api/finance`, `api/reviews`, `api/households`, `api/tenancy` (households/users) — each owning its handlers + tests. Shared plumbing (`api/httpx`, `api/gen`, `api/docs`, DTO/contract concerns) stays in dedicated packages; composition lives in the composition root. Domain logic in `core/` and infra in `infra/` are NOT part of this restructure (already modular).

#### Scenario: No flat mixing of features in api root

- **WHEN** a developer looks for the finance movement handlers
- **THEN** they find them in `api/finance/` with their tests co-located, and the `api` root contains only the composition root / server wiring

#### Scenario: Behavior unchanged after the split

- **WHEN** the split is complete and the backend test suite runs
- **THEN** all handler tests pass with import-path updates only — no behavioral diffs — and the wire-parity test still holds

### Requirement: UI pages are organized as feature folders

The UI `src/pages/*` tree SHALL be reorganized into `src/features/<feature>/` folders with co-located page components, hooks, and small feature-local components: `features/add`, `features/search`, `features/assets`, `features/reviews`, `features/finance`, `features/documents`, plus a `features/docs` group for shared primitives (data-table, upload, quick-search stay in `components/`). The only cross-feature imports permitted are from `features/docs/` (and `components/`, `lib/`); no file under `features/X/` may import from `features/Y/**` for any X≠Y — a lint rule enforces this.

#### Scenario: Documents feature is self-contained

- **WHEN** a developer changes the documents list UI
- **THEN** the page, its data hooks, and its row components live together under `features/documents/`, importing only shared UI primitives and the generated API client

#### Scenario: UI tests move with features

- **WHEN** the reorganization completes and `vitest` runs
- **THEN** all moved tests pass with import updates only; no test coverage is lost
