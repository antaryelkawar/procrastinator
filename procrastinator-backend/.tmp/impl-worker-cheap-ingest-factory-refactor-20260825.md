# Chunk: Refactor ingest service to *repo.Factory

Status: done

## Files created/modified

1. `core/ingest/service.go` — rewritten
2. `core/ingest/service_test.go` — rewritten
3. `api/server.go` — Server now takes `*repo.Factory`; added `tenantFromCtx` helper and `listDocumentsWithSource` method
4. `api/handlers.go` — handlers use `s.factory.Assets`/`s.factory.Documents`/`s.factory.Sources` with `repo.Tenant(tid)` options
5. `api/handlers_test.go` — test env uses `postgres.NewFactory(pool)` and `New(svc, factory, maxBytes)`; compile-time guard updated
6. `api/cmd/procrastinator/main.go` — uses `postgres.NewFactory(store.Pool())` and `api.New(svc, factory, cfg.MaxUploadBytes)`

## Changes in service.go

- Removed `legacyAssetRepo` adapter and `matchLegacyFilter` entirely.
- `Service.factory` changed from `repo.RepoFactory` to `*repo.Factory`.
- `New` signature changed to `New(factory *repo.Factory, ...)`.
- `Process` extracts tenant via `tenant.TenantFrom(ctx)` at the top, passes `repo.Tenant(tid)` to all repo calls.
- `s.factory.Sources.Create(ctx, source, repo.Tenant(tid))` replaces `s.factory.SourceRepo().Create(ctx, source)`.
- `s.factory.InTx(ctx, func(ctx, repos *repo.Repos) error {...})` replaces `s.factory.InTransaction(ctx, func(ctx) error {...})` + `repo.RepoFactoryFromContext`.
- Inside InTx, uses `repos.Assets` directly and `repos.Documents.Create(ctx, doc, repo.Tenant(tid))`.
- `extractionFields` helper unchanged.
- Added `"procrastinator-backend/commons/tenant"` import.

## Changes in service_test.go

- Replaced all legacy fakes with generic `repo.Repository[T]` fakes:
  - `fakeAssetRepo` — implements `repo.Repository[entity.Asset]` with `Get`/`List`/`Create`/`Update`/`Delete`, `snapshot()`/`rollback()`, `count()`/`all()`/`get()`. `List` parses options via `repo.ApplyOptions`, filters on TenantID, `norm_serial`/`norm_brand`/`norm_model` Where filters, and Limit.
  - `fakeSourceRepo` — implements `repo.Repository[entity.Source]` with same CRUD pattern.
  - `fakeDocumentRepo` — implements `repo.Repository[entity.Document]` with `createErr` support, `snapshot()`/`rollback()`.
- `fakeFactory` wraps a concrete `*repo.Factory` with `InTx` that snapshots/rolls back asset and doc repos.
- Compile-time guards for all three generic repos.
- `fakeExtractor` and `fakeStorage` unchanged.
- Harness passes `ff.factory` (`*repo.Factory`) to `ingest.New`.
- All 8 test scenarios preserved with `t.Parallel()`, seeded assets get `TenantID: testTenant`.

## Tests

- `go test ./core/... -count=1` — all pass:
  - `core/identity`: 7 test functions, all pass
  - `core/ingest`: TestProcess with 8 subtests, all pass
- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./api/... -count=1` — ok (skips without DB env var, compiles cleanly)

## Decisions

- The api layer (`api/server.go`, `api/handlers.go`) had to be migrated alongside the ingest service because `api.New` previously took legacy `repo.AssetRepository`/`repo.DocumentRepository` interfaces that are incompatible with `repo.Repository[T]` (different method signatures). The api server now takes `*repo.Factory` and uses generic `Get`/`List` with tenant options.
- `ListByAsset` (legacy joined query returning `DocumentWithSource`) is replaced by `listDocumentsWithSource` which uses `Documents.List` with an `asset_id` filter + `Sources.Get` per document. The legacy SQL did a JOIN; the new approach is N+1 but matches the generic repo interface.
