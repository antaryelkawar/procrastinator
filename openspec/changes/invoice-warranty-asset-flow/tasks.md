# Tasks: invoice-warranty-asset-flow (v3 — flat layout + generic repository)

This is a **refactor of committed, working code** (v2, commit `e904683`), not a rewrite. Existing tests move with their packages and must stay green; genuinely new behavior (option builders, generic CRUD, tenant option/context precedence, Go-side metadata merge) is written **test-first**. The module must build and test green after every task — tasks 2 and 3 are deliberately **additive** so the cut-over never breaks the build; all legacy code is deleted in task 6.

**Normative test conventions (apply to every task):** tests in `<prod_file>_test.go`; table-driven (slice of case structs + `t.Run`); `t.Parallel()` in every independent test; **no env vars in tests** — the only exceptions are reading `PROCRASTINATOR_TEST_DATABASE_URL` (and the real-LLM gate) **once** per package in `TestMain`/a single helper. Integration tests use **per-package private PostgreSQL schemas** (search_path DSN param) and explicit test tenants (`test-tenant`, `test-tenant-b`). All paths below are relative to the repo root; the Go module root is `procrastinator-backend/`.

**Shared file:** `api/cmd/procrastinator/main.go` is the composition root — any task that changes a wired constructor signature (tasks 4, 5) applies the minimal rewiring to keep `go build ./...` green; task 6 owns its final form.

## 1. Flat directory restructure (mechanical move, no interface changes)

Move all packages to the D2 layout and rename packages/imports. No type, function, or behavior changes — old per-entity interfaces stay exactly as-is for now.

- [ ] 1.1 `commons-data/` → `commons/` subpackages: `asset.go`/`source.go`/`document.go`/`normalize.go` → `commons/entity/`; `repository.go`/`services.go`/`tx.go` → interim files in `commons/repo/` and `commons/service/` (per D2; interfaces unchanged); `tenant.go` → `commons/tenant/`; `parse.go`/`extraction.go`/`dates.go` → `commons/parse/`. Update package clauses and all importers. Move all `*_test.go` alongside.
- [ ] 1.2 `procrastinator-core/{identity,ingest}` → `core/{identity,ingest}`; `procrastinator-infra/{postgres,llm,filestorage}` → `infra/{postgres,llm,filestorage}`; update imports.
- [ ] 1.3 `procrastinator-api/` → `api/`: `server.go`/`handlers.go`/`handlers_test.go` to `api/`; `commons-server/httpx/` → `api/httpx/`; `commons-server/config/config.go(+_test)` → `api/cmd/procrastinator/config.go(+_test)` (package `main`); `procrastinator-api/cmd/procrastinator-api/main.go` → `api/cmd/procrastinator/main.go`. Binary is now `procrastinator`. Delete all emptied dirs.
- **Files:** everything under `procrastinator-backend/commons-data/`, `commons-server/`, `procrastinator-core/`, `procrastinator-infra/`, `procrastinator-api/` → new `commons/`, `core/`, `infra/`, `api/` trees (moves only)
- **Depends on:** —
- **Verify:** from `procrastinator-backend/`: `go build ./...` (produces binary `procrastinator` from `api/cmd/procrastinator`), `go vet ./...`, `gofmt -l` clean, `go test ./... -count=1` green with compose PG up and explicit skips without; `grep -ri "commons-data|commons-server|procrastinator-core|procrastinator-infra|procrastinator-api" --include="*.go" procrastinator-backend/` finds nothing.

## 2. commons/repo: generic Repository[T] + options (additive)

- [ ] 2.1 Write failing tests in `commons/repo/options_test.go` (table-driven, `t.Parallel()`, pure): each option builder sets its field; options compose in order; multiple `Where` accumulate; zero-value `Options` has no tenant/filters/limits.
- [ ] 2.2 Implement `commons/repo/options.go` (`Filter`, `Options`, `Option`, `Tenant`/`Where`/`Limit`/`Offset`/`OrderBy` per D7), `commons/repo/generic.go` (`Repository[T]` interface, `ErrNotFound`), `commons/repo/factory.go` (`Repos` struct, `TxFactory` interface with explicit `*Repos` tx parameter). Legacy interfaces (`AssetRepository`/`SourceRepository`/`DocumentRepository`, old `RepoFactory`, `WithRepoFactory`/`RepoFactoryFromContext`) remain in place, marked `Deprecated:` — they are deleted in task 6.
- **Files:** `procrastinator-backend/commons/repo/options.go`, `options_test.go`, `generic.go`, `factory.go` (plus `Deprecated:` comment edits in the existing `commons/repo` files)
- **Depends on:** 1
- **Verify:** `go test ./commons/...` green without Docker/network; `go build ./...` green (new code is unreferenced by consumers).

## 3. infra/postgres: generic pgRepository[T] + new Factory (additive alongside legacy)

- [x] 3.1 Write failing integration tests in `infra/postgres/generic_test.go` (private schema `p_generic`, single env read in `TestMain`, explicit tenants): generic `Create`/`Get`/`List`/`Update`/`Delete` round-trip per entity (full field set incl. `metadata`, price exact round-trip); `Get`/`Delete` miss → `ErrNotFound`; `List` stable order + non-nil `[]`; `Where` filters (incl. two-filter brand+model), `Limit`/`Offset`/`OrderBy`; unknown field or op → error; **tenant precedence** — `Tenant` option beats context tenant, context fallback when no option, neither → `ErrNoTenant`, foreign-tenant rows invisible everywhere; `CreateOnConflictSerial` converges concurrent inserts; `ListByAsset` join ordered by `created_at, id` with source filename/upload timestamp; `Factory.InTransaction` commits on success, rolls back on error, tx-bound `Repos` see uncommitted writes and write nothing through the pool (verify-2 regression scenario).
- [x] 3.2 Implement `infra/postgres/repository.go` (`pgRepository[T]` per D7: option→SQL builder with field/operator whitelists, `resolveTenant` per D5, `RETURNING`-based write-then-scan), `infra/postgres/repos.go` (`AssetRepository`/`SourceRepository`/`DocumentRepository` wrappers — asset `Create`/`Update` derive norm columns via `commons/entity` normalization, plus `CreateOnConflictSerial` and `ListByAsset`; compile-time guards `var _ repo.Repository[...] = ...`), `infra/postgres/factory_new.go` (`Factory` with `Repos()` + `InTransaction`, `var _ repo.TxFactory` guard). Legacy `pgAssetRepo`/`pgSourceRepo`/`pgDocumentRepo`/`pgRepoFactory` and their tests stay untouched (deleted in task 6).
- **Files:** `procrastinator-backend/infra/postgres/repository.go`, `repos.go`, `factory_new.go`, `generic_test.go`
- **Depends on:** 2
- **Verify:** `go test ./infra/postgres/ -count=1` green against compose PG (legacy + new suites); default-parallel `-count=5` stable; skips explicitly without env; `go build ./...` green.

## 4. core: identity + ingest on generic interfaces

- [x] 4.1 Rewrite `core/identity/resolve_test.go` against the new contract (fake `AssetStore` = generic fake repo + conflict-injection on `CreateOnConflictSerial`; mutex-protected, call recording, `t.Parallel()`): serial lookup via `List`+`Where("norm_serial")` precedes brand+model; both-present rule for brand+model; no identity → `ErrNoIdentity`; create on miss; conflict path re-reads winner and merges; **merge builds a full updated entity** — non-empty overwrites, absent never erases, metadata shallow overlay accumulated, doc_type last-write-wins — persisted via `Update`.
- [x] 4.2 Rewrite `core/identity/resolve.go`: consumer-side `AssetStore` interface (embeds `repo.Repository[entity.Asset]`, adds `CreateOnConflictSerial`) and `Resolve(ctx, AssetStore, parse.Extraction)` per D7. Imports stdlib + `commons/` only.
- [x] 4.3 Rewrite `core/ingest/service_test.go` (pure fakes of `*repo.Repos`/`repo.TxFactory`/`service.Extractor`/`service.FileStorage`; fake `InTransaction` must invoke `fn` with the **tx-bound** `Repos` and assert the service uses them — not the pool-bound set): same scenarios as v2 (oversize → `ErrTooLarge` first; Source pre-tx survives `ErrExtraction`; `ErrNoIdentity` chain; happy-path document insert with metadata + verbatim raw extraction; rollback leaves no asset; tenant flows into repo calls as the `Tenant` option).
- [x] 4.4 Rewrite `core/ingest/service.go`: `New(repos *repo.Repos, tx repo.TxFactory, ex service.Extractor, fs service.FileStorage)`; `Process` resolves tenant once from ctx and passes `repo.Tenant(tid)` on every repo call; document insert + `identity.Resolve` run inside `InTransaction` against the explicit tx `*Repos`. Delete usage of `RepoFactoryFromContext`. Apply the minimal `main.go` rewiring (`postgres.NewFactory`, `factory.Repos()`).
- **Files:** `procrastinator-backend/core/identity/resolve.go`, `resolve_test.go`, `procrastinator-backend/core/ingest/service.go`, `service_test.go`, `procrastinator-backend/api/cmd/procrastinator/main.go` (minimal wiring only)
- **Depends on:** 3
- **Verify:** `go test ./core/...` green without Docker/network, `-count=5` stable; `go build ./...` green; `go list -deps ./core/...` contains no `pgx`, no `net/http`, no `infra`.

## 5. api: handlers + server on generic interfaces

- [x] 5.1 Update `api/handlers_test.go` to the generic wiring (real `*repo.Repos` + real PG in per-test private schemas, real tenant middleware, fake-LLM httptest — same scenarios as v2, unchanged expectations: 201 incl. `doc_type`+`metadata`, 400 missing/invalid tenant, 400 missing file, 413 oversize, 415 bad type, 502 outage, 422 no identity, list/get/documents shapes, cross-tenant 404 and same-serial-two-tenants).
- [x] 5.2 Update `api/server.go` (consumer-side `documentLister` interface per D7; `Server` takes `*ingest.Service`, `repo.Repository[entity.Asset]`, `documentLister`) and `api/handlers.go` (resolve tenant once from ctx, pass `repo.Tenant(tid)` on all repo calls; `Get`/`List` via generic interface; documents via `documentLister`). Split DTOs (`assetJSON`, `documentJSON`, converters) into `api/dto.go`. Apply the minimal `main.go` rewiring if the `Server` constructor changed.
- **Files:** `procrastinator-backend/api/server.go`, `handlers.go`, `dto.go`, `handlers_test.go`, `procrastinator-backend/api/cmd/procrastinator/main.go` (minimal wiring only)
- **Depends on:** 4
- **Verify:** `go test ./api/ -count=1` green against compose PG; default-parallel `-count=5` stable; `go build ./...` green.

## 6. Legacy deletion + final wiring

- [x] 6.1 Delete the legacy per-entity interfaces and helpers: old `AssetRepository`/`SourceRepository`/`DocumentRepository` declarations, old `RepoFactory`, `WithRepoFactory`/`RepoFactoryFromContext` (from `commons/repo`), and `entity.UpdateFields` with all remaining references.
- [x] 6.2 Delete the legacy postgres implementations and their tests: `pgAssetRepo`/`pgSourceRepo`/`pgDocumentRepo`/`pgRepoFactory` code (old `assets.go`/`sources.go`/`documents.go`/`factory.go` contents) and the now-superseded legacy test files (`assets_test.go`/`sources_test.go`/`documents_test.go`/`factory_test.go`) — coverage is carried by `generic_test.go` (task 3) plus the moved-and-updated consumer suites. Rename `factory_new.go` → `factory.go`.
- [x] 6.3 Final `main.go` form: wires `postgres.NewFactory` → `factory.Repos()` + `llm.NewExtractor` + `filestorage.Storage` into `ingest.New` and `api.New`; boots, migrates, serves with graceful shutdown (unchanged behavior).
- **Files:** deletions in `procrastinator-backend/commons/repo/` (legacy interface files), `procrastinator-backend/commons/entity/asset.go` (`UpdateFields`), `procrastinator-backend/infra/postgres/{assets,sources,documents,factory}.go` + legacy `*_test.go`, `factory_new.go`→`factory.go`, `procrastinator-backend/api/cmd/procrastinator/main.go`
- **Depends on:** 4, 5
- **Verify:** `go build ./...`, `go vet ./...`, full `go test ./... -count=1` green (DB + no-DB); `grep -rn "UpdateFields|FindBySerial|FindByBrandModel|RepoFactory|WithRepoFactory" --include="*.go" procrastinator-backend/` finds nothing; against compose PG + real `.env`: binary boots and `POST /api/documents` with `X-Tenant-ID` → 201.

## 7. Final gate

- [x] 7.1 From `procrastinator-backend/`: `go vet ./...`, `gofmt -l` (empty), `go build ./...`, full `go test ./... -count=1` green with compose PG up (integration) and without (explicit skips); default-parallel `-count=3` stable; env-grep confirms no env access in `*_test.go` outside sanctioned helpers; **naming greps**: zero `commons-data`/`commons-server`/`procrastinator-core`/`procrastinator-infra`/`procrastinator-api` references in the backend tree, zero `life-manager`/`lifemanager`/`LM_` (outside documented `PROCRASTINATOR_LLM_*` false positives), `PROCRASTINATOR_*` env table intact; **layering**: `go list -deps ./core/...` free of `pgx`/`net/http`/`infra`, no cross-imports between `infra/` packages, `commons/` imports stdlib (+`entity` from `repo`) only; interface guards compile (by construction); every spec scenario still mapped to a test; `openspec validate invoice-warranty-asset-flow --strict` passes.
- **Depends on:** all
