# Proposal: invoice-warranty-asset-flow (v3 — flat layout + generic repository)

## Why

The v2 redesign is implemented, verified, and archived (commit `e904683`). Two structural issues remain in the committed layout: top-level directories repeat the module name (`procrastinator-backend/procrastinator-api`, `…/procrastinator-core`, `…/procrastinator-infra`) while `commons-data`/`commons-server` follow a different convention, and the data-access layer carries three per-entity repository interfaces (`AssetRepository` with 7 entity-specific methods, `SourceRepository`, `DocumentRepository`) that a single generic `Repository[T]` with functional options can replace — removing interface churn per new entity and making query modifiers (tenant, filters, limits) explicit at the call site. This change applies both adjustments as a pure refactor: no schema, endpoint, prompt, or behavior changes.

## What Changes

- **BREAKING (internal)** — Restructure `procrastinator-backend/` to flat top-level roots with subpackages: `commons/` (`entity`, `repo`, `service`, `tenant`, `parse`), `core/` (`identity`, `ingest`), `infra/` (`postgres`, `llm`, `filestorage`), `api/` (handlers/server/DTOs, `httpx/`, `cmd/procrastinator/`). The `commons-data`, `commons-server`, `procrastinator-core`, `procrastinator-infra`, `procrastinator-api` roots are deleted; the binary is renamed `procrastinator-api` → `procrastinator` (source: `api/cmd/procrastinator`). Env vars (`PROCRASTINATOR_*`), module path (`procrastinator-backend`), database, and config struct are unchanged.
- **BREAKING (internal)** — Replace the per-entity repository interfaces with one generic `repo.Repository[T]` (`Get`/`List`/`Create`/`Update`/`Delete`) plus functional options (`Tenant`, `Where`, `Limit`, `Offset`, `OrderBy`). Entity-specific operations move to call-site queries (`List` + `Where`) in `core/` or concrete postgres methods consumed via narrow consumer-side interfaces (`core/identity.AssetStore` for race-safe serial insert, `api.documentLister` for the document+source join). `RepoFactory`/`InTransaction` becomes `repo.TxFactory` passing an explicit tx-bound `*repo.Repos` (the context-carried factory is deleted); `UpdateFields` is deleted in favor of full-entity `Update` with the merge computed in Go by `core/identity` (identical merge semantics).
- Tenant scoping becomes explicit at the data-access call site: every repository call passes `repo.Tenant(tid)`; the context tenant is the fallback; neither → fail-closed `ErrNoTenant`. HTTP middleware and header contract unchanged.
- Behavior, API surface, schema, LLM prompt, and test conventions (table-driven, `t.Parallel`, no env in tests, `<prod>_test.go`) are unchanged.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `backend-platform`: package roots renamed to the flat `api`/`core`/`infra`/`commons` layout with subpackages; binary renamed `procrastinator`; boundary interfaces become the generic `repo.Repository[T]` + options in `commons/repo` (with `commons/service` for `Extractor`/`FileStorage`) and a `repo.TxFactory` transaction abstraction.
- `multitenancy`: repositories take the tenant via an explicit `repo.Tenant(id)` option at the call site, with the context tenant as fallback (still fail-closed).
- `test-infrastructure`: interface-based unit testing now fakes the generic `commons/repo`/`commons/service` interfaces (was `commons-data`).

## Impact

- **Code**: every package under `procrastinator-backend/` moves; `infra/postgres` gains a generic `pgRepository[T]` implementation; `core/identity`, `core/ingest`, `api/`, and `main.go` are re-wired to the generic interfaces; legacy per-entity interfaces and postgres implementations are deleted after the cut-over.
- **APIs**: unchanged (same four endpoints, same contracts).
- **Dependencies**: unchanged (Go 1.27, chi v5.3.2, pgx v5.10.0, goose v3.27.3, PostgreSQL 18; generics are stdlib since Go 1.18).
- **Infrastructure**: unchanged (`docker-compose.yml`, `.env`/`.env.example`, migrations untouched). Binary artifact name changes to `procrastinator`.
