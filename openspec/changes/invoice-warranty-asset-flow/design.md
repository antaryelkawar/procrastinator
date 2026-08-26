# Design: invoice-warranty-asset-flow (v3 — flat layout + generic repository)

## Context

The v2 redesign (clean architecture, multitenancy, metadata JSONB, `procrastinator-*` naming) is implemented and archived (`archive/2026-08-24-invoice-warranty-asset-flow-v2`, commit `e904683`). Two structural adjustments are now applied on top of that committed, working code — this is a **refactor, not a rewrite**:

1. **Flat directory structure with subpackages.** The `procrastinator-` prefix on top-level dirs duplicates the module name (`procrastinator-backend`); `commons-data`/`commons-server` naming is inconsistent with the rest. Top-level dirs become flat: `api/`, `core/`, `infra/`, `commons/`, each with subpackages for grouping.
2. **Generic `Repository[T]` interface.** The three per-entity repository interfaces (`AssetRepository` 7 methods, `SourceRepository`, `DocumentRepository`) collapse into one generic interface with functional options. Entity-specific operations become either `core/` call-site queries (`List` + `Where` options) or concrete methods on the postgres implementation (not on the shared interface).

Behavior, schema, API surface, naming (`PROCRASTINATOR_*`), and testing conventions are unchanged. No migration changes; the dev database is untouched.

## Goals / Non-Goals

**Goals** — flat top-level layout (`api/`, `core/`, `infra/`, `commons/`) with subpackages; a single generic `Repository[T]` in `commons/repo` with `Option`-based query modifiers (explicit tenant at the call site, context as fallback); entity-specific persistence operations confined to concrete postgres types or `core/` consumer-side interfaces; the module stays buildable and green after every task.

**Non-Goals** — no behavior changes (no schema, endpoint, prompt, or merge-semantics changes beyond what the generic update model mechanically implies); no new dependencies; no multi-module split; no pagination feature (the `Limit`/`Offset` options exist but no endpoint exposes them); no auth.

## Decisions

### D1: Architecture — four flat layers, one dependency rule (updated)

| Layer | Package root | Contents | May import |
|---|---|---|---|
| shared kernel | `commons/` | `entity` (Asset, Source, Document, normalize helpers), `repo` (generic `Repository[T]`, options, `Repos`, `TxFactory`, `ErrNotFound`), `service` (`Extractor`, `FileStorage`), `tenant` (ctx helpers, `ErrNoTenant`), `parse` (`ParseExtraction`, `Extraction`, date parsing) | stdlib only (`repo` may import `entity`) |
| domain/application | `core/` | `identity` (resolution + merge), `ingest` (orchestration) | stdlib + `commons/` only |
| infrastructure | `infra/` | `postgres` (pool, migrations, generic repos, factory), `llm` (client, extractor, prompt), `filestorage` | stdlib + `commons/` + pgx/goose (postgres) |
| delivery | `api/` | router/handlers/DTOs, `httpx` (JSON envelope, tenant middleware), `cmd/procrastinator` (main + config — the ONLY env-touching code) | all of the above |

Hard rules (unchanged in spirit, restated for new roots):
- `core/` MUST NOT import `net/http`, `pgx`, or any `infra/` package. Verified mechanically in the final gate via `go list -deps`.
- No `infra/` package imports another `infra/` package.
- `commons/` holds entities, interfaces, and pure helpers only — no concrete implementations, no infra imports.
- `main` (`api/cmd/procrastinator`) is the only composition root and the only env reader: it builds concrete `infra/` implementations and injects them as `commons/` interfaces into `core/` services and the `api/` server. Handlers depend on `core/` services and `commons/` interfaces, never on `infra/` types.

### D2: Directory layout — flat roots with subpackages (updated)

Single Go module (`module procrastinator-backend`, `go.mod` unchanged). New layout:

```
procrastinator-backend/
  go.mod                                  # module procrastinator-backend (unchanged)
  docker-compose.yml                      # unchanged (postgres:18, db/user procrastinator)
  .env / .env.example                     # PROCRASTINATOR_* vars (unchanged)
  migrations/00001_init.sql               # unchanged
  commons/
    entity/asset.go                       # Asset, DocType (+Valid), no more UpdateFields (see D7)
    entity/source.go                      # Source
    entity/document.go                    # Document, DocumentWithSource
    entity/normalize.go                   # NormalizeSerial, NormalizeName, CollapseWhitespace
    repo/options.go                       # Option, Options, Filter, Tenant/Where/Limit/Offset/OrderBy
    repo/repository.go                    # Repository[T], ErrNotFound
    repo/factory.go                       # Repos struct, TxFactory interface
    service/service.go                    # Extractor, FileStorage, StoredFile
    tenant/tenant.go                      # WithTenant, TenantFrom, ErrNoTenant
    parse/extraction.go                   # Extraction (typed core + Metadata + RawPayload)
    parse/parse.go                        # ParseExtraction
    parse/dates.go                        # ParseDate
  core/
    identity/resolve.go                   # Resolve + consumer-side AssetStore interface
    ingest/service.go                     # Process orchestration over interfaces only
  infra/
    postgres/store.go                     # Open/Migrate/TruncateAll, Querier
    postgres/repository.go                # pgRepository[T] generic CRUD + option→SQL builder
    postgres/repos.go                     # AssetRepository/SourceRepository/DocumentRepository wrappers (+guards)
    postgres/factory.go                   # Factory: Repos() + InTransaction (+guard)
    llm/client.go                         # generic OpenAI-compatible chat client (unchanged)
    llm/extractor.go                      # Extractor adapter (guard updated)
    llm/prompt.go                         # document-extraction system prompt (unchanged)
    filestorage/storage.go                # local FS storage (guard updated)
  api/
    server.go                             # chi router, middleware chain, documentLister consumer interface
    handlers.go                           # 4 handlers
    dto.go                                # assetJSON, documentJSON
    httpx/json.go                         # WriteJSON, WriteError
    httpx/tenant.go                       # TenantMiddleware (X-Tenant-ID → ctx)
    cmd/procrastinator/main.go            # composition root (package main)
    cmd/procrastinator/config.go          # Config + Load — sole env access point
```

Placement decisions where v2 packages have no flat counterpart:
- `commons-server/config` → `api/cmd/procrastinator/config.go` (package `main`). Config is read only by the binary; colocating it with `main.go` keeps "the ONLY env-touching code" literally in one package. Its unit tests (`config_test.go`, map-injected, zero-env) move along unchanged.
- `commons-server/httpx` → `api/httpx/` — HTTP utilities belong to the delivery layer.
- `commons-data/normalize.go` → `commons/entity/` — it normalizes entity fields (serial/brand/model); used by `infra/postgres` (norm columns on write) and `core/identity`.
- `commons-data/dates.go` → `commons/parse/` — date parsing exists for extraction parsing.
- `commons-data/tx.go` (`RepoFactory`, `WithRepoFactory`/`RepoFactoryFromContext` context plumbing) → **deleted**. The tx-bound repository set is now passed explicitly as a function parameter (D7) — the context-key carrier added in the verify-2 fix is superseded.

Package-relative test depth is preserved by the move (all packages remain at the same depth under the module root), so existing `migrationsDir` relative paths in integration tests stay correct.

### D3: Naming — unchanged, except binary (mostly unchanged)

- Env vars: all `PROCRASTINATOR_*` names from the v2 D3 table are unchanged. Only `main`/`config.go` reads real env.
- Database/user `procrastinator`, compose healthcheck, test DB `procrastinator_test` — unchanged.
- Module path `procrastinator-backend` — unchanged. Import paths lose the stutter: `procrastinator-backend/commons/entity`, `procrastinator-backend/core/ingest`, etc.
- **Binary renamed `procrastinator-api` → `procrastinator`**, matching its new source dir `api/cmd/procrastinator` (go tool names the binary after the package dir). This is the one user-visible naming delta.
- Final gate greps for `commons-data`, `commons-server`, `procrastinator-core`, `procrastinator-infra`, `procrastinator-api` path references — zero matches outside archived openspec artifacts.

### D4: Asset model — unchanged

Schema (`migrations/00001_init.sql`), structured core + `metadata` JSONB, `doc_type` enum incl. `amc`, tenant-scoped serial uniqueness — all untouched by this refactor. One mechanical consequence of D7: the `metadata || $n::jsonb` SQL overlay disappears with `UpdateFields`; the identical shallow per-key overlay semantics (absent keys retained, present keys overwritten, nulls never stored) are now implemented in Go inside `core/identity` before a full-entity `Update` (see D7). Merge semantics per the specs are unchanged.

### D5: Multitenancy — explicit Option at the call site, context as transport (updated)

- HTTP transport unchanged: `api/httpx.TenantMiddleware` validates `X-Tenant-ID` (`^[A-Za-z0-9_-]{1,64}$`, missing/invalid → 400) and places it in `context.Context` via `commons/tenant.WithTenant`. Services/handlers still receive the tenant through the context.
- **Data-access call sites are explicit**: every repository call passes `repo.Tenant(tid)` as an `Option`. Handlers/services resolve `tid` once via `tenant.TenantFrom(ctx)` (fail-closed) and pass the option down.
- **Resolution rule inside `infra/postgres`**: `Options.TenantID` if the option was given, else `tenant.TenantFrom(ctx)` fallback; neither → `ErrNoTenant`, no query runs. The option always wins when both are present.
- All SQL remains `WHERE tenant_id = $n`; inserts stamp `tenant_id`. Cross-tenant isolation tests unchanged; new unit/integration tests cover option-vs-context precedence and the fail-closed path.

### D6: LLM prompt — unchanged

System prompt (anti-thinking header, classify-then-extract, metadata contract, worked AMC example), client envelope (`json_object`, temperature 0, base64 `image_url` data-URI), `<thought>` stripping safety net in `commons/parse`, and the 502 failure mapping are untouched. Files move (`infra/llm/`, `commons/parse/`); content does not.

### D7: Generic `Repository[T]` + functional options (updated)

All boundary interfaces live in `commons/`, split by subpackage.

**`commons/repo`** — the single generic data-access interface:

```go
package repo

type Filter struct{ Field, Op string; Value any }   // Op whitelisted per impl: =, !=, <, <=, >, >=, LIKE

type Options struct {
    TenantID string
    Filters  []Filter
    Limit    int
    Offset   int
    OrderBy  string
}

type Option func(*Options)

func Tenant(id string) Option
func Where(field, op string, value any) Option
func Limit(n int) Option
func Offset(n int) Option
func OrderBy(field string) Option

var ErrNotFound = errors.New("repo: not found")

type Repository[T any] interface {
    Get(ctx context.Context, id string, opts ...Option) (T, error)
    List(ctx context.Context, opts ...Option) ([]T, error)
    Create(ctx context.Context, entity T, opts ...Option) (T, error)
    Update(ctx context.Context, entity T, opts ...Option) (T, error)
    Delete(ctx context.Context, id string, opts ...Option) error
}

// Repos bundles the three entity repositories (pool- or tx-bound).
type Repos struct {
    Assets    Repository[entity.Asset]
    Sources   Repository[entity.Source]
    Documents Repository[entity.Document]
}

// TxFactory runs fn in one transaction; the *Repos passed to fn are tx-bound.
type TxFactory interface {
    InTransaction(ctx context.Context, fn func(ctx context.Context, tx *Repos) error) error
}
```

Entity-specific queries are **not** on the interface:
- Lookups become call-site queries in `core/identity`: serial → `List(ctx, repo.Tenant(t), repo.Where("norm_serial", "=", s), repo.Limit(2))`; brand+model → two `Where` options.
- Race-safe serial insert and the document/source join stay as **concrete methods on the postgres types**, consumed through narrow consumer-side interfaces:
  - `core/identity` defines `AssetStore interface { repo.Repository[entity.Asset]; CreateOnConflictSerial(ctx, entity.Asset, ...repo.Option) (entity.Asset, bool, error) }` — `Resolve(ctx, AssetStore, parse.Extraction)` takes it directly (one parameter, faked in tests).
  - `api` defines `documentLister interface { ListByAsset(ctx, assetID string, opts ...repo.Option) ([]entity.DocumentWithSource, error) }` for the ordered document+source listing; `main` wires the concrete postgres document repo.
- `Update` is full-entity (the entity carries its id). `UpdateFields` is deleted; the merge (non-empty overwrite, absent never erases, metadata shallow overlay, doc_type last-write-wins) is computed in Go by `core/identity` against the just-read asset, then persisted via `Update`. Accepted trade-off: concurrent merges to the same asset row can lost-update metadata (the serial-create race remains safe via `CreateOnConflictSerial` + conflict re-read); single-user upload flow makes this theoretical.
- **`commons/service`** (unchanged shapes): `Extractor`, `FileStorage`, `StoredFile`.
- **`commons/entity`**: entities + normalization helpers; `UpdateFields` removed.

**`infra/postgres`** — one generic implementation:

```go
type pgRepository[T any] struct {
    q         Querier                       // *pgxpool.Pool or pgx.Tx
    table     string
    columns   []string                      // stable select/insert list
    scanRow   func(pgx.Row) (T, error)
    fieldCols map[string]string             // filterable field → column (SQL-injection whitelist)
    idOf      func(T) string
}

type AssetRepository    struct{ *pgRepository[entity.Asset] }    // + CreateOnConflictSerial; Create/Update wrap to derive norm columns
type SourceRepository   struct{ *pgRepository[entity.Source] }
type DocumentRepository struct{ *pgRepository[entity.Document] } // + ListByAsset (join, ORDER BY created_at, id)

var _ repo.Repository[entity.Asset] = (*AssetRepository)(nil)    // guards for all three + Factory + llm.Extractor + filestorage.Storage

type Factory struct{ pool *pgxpool.Pool }
func (f *Factory) Repos() *repo.Repos                                        // pool-bound
func (f *Factory) InTransaction(ctx context.Context, fn func(context.Context, *repo.Repos) error) error  // tx-bound Repos; commit on nil, rollback otherwise
```

- Option→SQL translation whitelists fields (`fieldCols`) and operators; unknown field/op → error, never string-interpolated input. `resolveTenant(ctx, *repo.Options)` implements the D5 precedence.
- `ingest.Service` is constructed as `New(repos *repo.Repos, tx repo.TxFactory, ex service.Extractor, fs service.FileStorage)`: Source row is written pre-transaction via `repos.Sources`; identity resolution + document insert run inside `InTransaction` against the **explicit tx-bound `*Repos` parameter** (no context-carried factory).
- Transaction rollback semantics verified by the verify-2 fix are preserved: nothing inside `fn` touches pool-bound repos.

### D8: Testing strategy — conventions unchanged, coverage adjusted

Normative conventions carried over verbatim: `<prod>_test.go` pairing; table-driven; `t.Parallel()` by default; **no env vars in tests** except the single `PROCRASTINATOR_TEST_DATABASE_URL` read per package (TestMain/helper) and the gated real-LLM helper; per-package private PostgreSQL schemas; explicit test tenants (`test-tenant`, `test-tenant-b`).

- **Moved tests**: all existing tests move with their packages in the flat-layout task and must stay green (import/package renames only).
- **New pure unit tests**: `commons/repo/options_test.go` (option builders compose into `Options`); `core/identity` and `core/ingest` tests re-target the generic interfaces with in-memory fakes (no Docker) — same scenarios as v2 plus full-entity merge assertions.
- **New integration tests** (private schema `p_generic`): generic CRUD per entity; `Where`/`Limit`/`Offset`/`OrderBy` behavior; unknown field/op rejected; tenant option beats context, context fallback works, neither → `ErrNoTenant`; `CreateOnConflictSerial` convergence; `ListByAsset` join order; `InTransaction` commit/rollback/isolation (the verify-2 regression scenario, kept).
- **Real LLM**: unchanged gated test on the two `sample-data/` PDFs, moved to `infra/llm/`.

## API surface

Unchanged from v2 — same four endpoints, same status codes, `X-Tenant-ID` required, `{"error": "..."}` envelope, price as JSON string. Asset JSON still carries `doc_type` + `metadata`.

## Risks / Trade-offs

- **Full-entity `Update` lost-update window** (concurrent metadata merges) — accepted; documented in D7. Serial-create race safety is preserved by `CreateOnConflictSerial`.
- **Mid-refactor dual-interface period** (generic repo added additively before legacy interfaces are deleted) — contained by task ordering; legacy code is deleted in one dedicated task before the final gate.
- **Field/operator whitelist drift** (new filterable field needed later) — one-line map entry; mitigated by tests asserting rejection of unknown fields.
- **Consumer-side interfaces (`AssetStore`, `documentLister`) reintroduce narrow interfaces** — deliberate: they are consumer-owned (idiomatic Go), keep `core/` free of infra types, and are the only places upsert/join semantics leak past the generic contract.
- **Binary rename** `procrastinator-api` → `procrastinator` — docs/scripts referencing the old name updated in the same task; no functional impact.

## Testing conventions (normative, applies to all tasks)

Tests live in `<prod_file>_test.go`; table-driven; `t.Parallel()` in every independent test; no env vars in tests except one `PROCRASTINATOR_TEST_DATABASE_URL` read per package (TestMain/helper) and the gated real-LLM helper; integration tests use per-package private schemas and explicit test tenants; config is injected as `Config` literals.
