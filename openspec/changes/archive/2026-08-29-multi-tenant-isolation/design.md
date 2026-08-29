# Design: multi-tenant-isolation

## Context

The spec deltas (proposal + 3 capability deltas) define the target: fail-closed tenant resolution in the data layer, a `tenants` registry, the 400/404 header contract, RLS defense-in-depth, tenant-scoped file storage, and whitelisted dynamic queries. This design grounds each requirement in the **actual working tree** (v3 flat layout + generic `Repository[T]`, committed at `591eaaa`/`c2f5a64`), which is **weaker than the v3 design promised**:

| # | Gap confirmed in code | Grounding |
|---|---|---|
| G1 | `pgRepository` runs **unscoped** when `Options.TenantID == ""` — no ctx fallback, no `ErrNoTenant`, no fail-closed | `infra/postgres/repository.go` (`if o.TenantID != ""` guards) |
| G2 | Filter `Field`/`Op` and `OrderBy` are **string-interpolated** into SQL (`fmt.Sprintf("%s %s $%d", f.Field, f.Op, …)`) | `infra/postgres/repository.go:45,68,79-81` |
| G3 | No `tenants` table; any well-formed header is a de-facto tenant | `migrations/00001_init.sql` |
| G4 | `httpx.TenantMiddleware` validates **non-empty only**; malformed headers pass; a second `server.tenantContext` bridge responds **401** (spec says 400) and uses a *second* context key | `api/httpx/tenant.go`, `api/server.go:92-108` |
| G5 | `filestorage.Storage.Put` ignores tenant; flat key space `dir/{uuid}.ext` | `infra/filestorage/storage.go:37-62` |
| G6 | **The compose app role is a superuser** (`POSTGRES_USER: procrastinator` in the postgres image) — RLS would be a no-op for it, violating the spec's "SHALL NOT be a superuser" clause | `docker-compose.yml:5` |

The v3 contract this builds on (unchanged by this design): explicit `repo.Tenant(id)` option with `context.Context` fallback, option wins; `repo.Repository[T]` + `Options`; `repo.Factory`/`repo.Repos` + `InTx` with explicit tx-bound repos; per-package private test schemas + explicit test tenants.

## Goals / Non-Goals

**Goals** — every repository operation resolves a tenant (option → ctx) and fails closed with `tenant.ErrNoTenant` issuing **zero SQL**; a `tenants` registry (migration + FKs + backfill + explicit seed) is the source of truth; header contract missing/malformed → `400`, unregistered → `404`, validated before any handler code; RLS enabled + forced on all tenant-owned tables with a **transaction-scoped** tenant binding that cannot leak across pooled connections; stored files live under `{tenantID}/{fileID}`; dynamic query names validated against per-entity whitelists; the app DB role is non-superuser without `BYPASSRLS`.

**Non-Goals (explicit out-of-scope)** —
- No authentication/authorization (no sessions, no authn layer); the `404` vs `403` choice for unknown tenants is revisited when auth lands.
- No tenant-management API surface (no CRUD endpoints for tenants); provisioning is migration + admin seeding only.
- No registry cache or invalidation mechanism (per-request PK lookup is sufficient at this scale).
- No migration/move of pre-existing flat files in `storage/`; no file-download endpoint exists, so old `storage_path` values are inert until a read path is built.
- No changes to `repo.Options`/`Option` shapes, entity types, LLM prompt, merge semantics, pagination, or the API endpoint set.
- No `BYPASSRLS` service role, no per-tenant storage quotas, no multi-DB sharding.
- No UI — that is separate change B.

## Decisions

### D1: Fail-closed tenant resolution — `resolveTenant` in `infra/postgres`

One resolution rule for all five `pgRepository[T]` methods, executed **before any SQL is built or sent**:

```go
func resolveTenant(ctx context.Context, o *repo.Options) (string, error) {
    if o.TenantID != "" {          // explicit option wins
        return o.TenantID, nil
    }
    return tenant.TenantFrom(ctx)  // context fallback; ErrNoTenant when absent
}
```

- Sentinel: the **single existing** `tenant.ErrNoTenant` (`commons/tenant`) — returned unwrapped, so `errors.Is(err, tenant.ErrNoTenant)` works from every layer. No new sentinel in `commons/repo`.
- Every method's first statement is resolution; on `ErrNoTenant` the method returns immediately — no `scope.run`, no SQL (spec scenario "Tenantless call issues no SQL" is testable with a panic-on-query fake, D8).
- `Create`/`Update` stamp `tenant_id` from the **resolved** tenant (always present at that point); `Get`/`List`/`Delete` keep `WHERE tenant_id = $n` as the primary scoping clause.
- Call sites (`core/ingest`, `core/identity`, `api`) already resolve `tid` once and pass `repo.Tenant(tid)` — **no changes** in `core/` (layering rule preserved: `core/` imports no `infra/`).

### D2: Tenant registry — migration 00002 + `TenantRegistry` boundary

**`migrations/00002_tenant_registry.sql`** (single goose migration, order matters):

```sql
-- +goose Up
CREATE TABLE tenants (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);
-- Backfill: every distinct pre-existing tenant_id becomes a registry row,
-- before FK enforcement.
INSERT INTO tenants (id)
    (SELECT DISTINCT tenant_id FROM sources
     UNION
     SELECT DISTINCT tenant_id FROM assets
     UNION
     SELECT DISTINCT tenant_id FROM documents)
    ON CONFLICT (id) DO NOTHING;
-- Explicit provisioning of dev/test tenants (never implicit from a header).
INSERT INTO tenants (id) VALUES ('test-tenant'), ('test-tenant-b')
    ON CONFLICT (id) DO NOTHING;
-- Tenant-owned rows must reference a registered tenant (default ON DELETE RESTRICT).
ALTER TABLE sources   ADD CONSTRAINT sources_tenant_id_fkey   FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE assets    ADD CONSTRAINT assets_tenant_id_fkey    FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE documents ADD CONSTRAINT documents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants (id);

-- +goose Down  (drop the three FKs, then DROP TABLE IF EXISTS tenants)
```

- The `CHECK` on `tenants.id` mirrors the header format — the registry itself rejects malformed IDs (defense-in-depth).
- Default FK `ON DELETE RESTRICT` gives "deleting a registry row that still owns data is rejected" for free (pg error 23503).

**Boundary + implementation:**

```go
// commons/repo/registry.go (new)
type TenantRegistry interface {
    Has(ctx context.Context, id string) (bool, error)
}

// infra/postgres/tenant_registry.go (new)
func NewTenantRegistry(q Querier) repo.TenantRegistry   // SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)
```

- `repo.Factory` gains one **additive** field `Tenants repo.TenantRegistry`; `postgres.NewFactory` populates it from the pool. No constructor signatures change (`api.New`, `ingest.New`, `main.go` untouched) — this is the only modification to a v3-committed file in `commons/`, and it is purely additive.
- `Has` runs on the `tenants` table, which has **no RLS** (it is the registry, not tenant-owned data) — it is callable before any tenant binding exists.
- **Lookup, not cache** (spec-report open point resolved): one indexed PK probe on a tiny table per request; a cache would add invalidation machinery for no measurable gain at this scale.

### D3: URL-based transport and User-centric model

The `X-Tenant-ID` header is dropped. The active user (tenant) is identified by the `{userId}` parameter in the URL path.

**Routes Example:**
- `POST /api/users/{userId}/documents`
- `GET /api/users/{userId}/assets`
- `GET /api/users/{userId}/assets/{assetId}`
- `GET /api/users/{userId}/assets/{assetId}/documents`

**Middleware Changes:**
- `httpx.TenantMiddleware` reads `chi.URLParam(r, "userId")`.
- Validation: missing/malformed → 400, unregistered → 404, registry DB error → 500.

### D2: User registry (Table `tenants`)

- `tenants.id` holds the user id.
- The table name `tenants` is kept for stability.
- No org layer.
- Migration 00002 handles table creation and registry PK lookup.

### D9: Households and Scope (New)

**Schema Changes:**
- `households` (id, tenant_id, display_name, created_at)
- `household_members` (household_id, user_id/tenant_id, unique per pair)
- Tenant-owned data rows (assets, documents, sources) add:
  - `scope_type` (enum: 'personal', 'household')
  - `owner_household_id` (uuid, nullable)

**Access Logic (Application Level):**
- Membership check: a user may read/write rows scoped to their `personal` scope or to any `household` they belong to.
- NO authn — access is still by the userId in the URL.
- Enforcement: membership check is a simple query, not a security boundary.

**Migration `00003_row_level_security.sql`** (idempotent-shaped; goose applies once per schema):

```sql
-- +goose Up  (per table: sources, assets, documents)
ALTER TABLE <t> ENABLE ROW LEVEL SECURITY;
ALTER TABLE <t> FORCE ROW LEVEL SECURITY;      -- policy binds the table owner too
DROP POLICY IF EXISTS <t>_tenant_isolation ON <t>;
CREATE POLICY <t>_tenant_isolation ON <t>
    USING  (tenant_id = current_setting('app.tenant_id', true))
    WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
-- +goose Down  (drop policy; NO FORCE; DISABLE — per table)
```

- `current_setting('app.tenant_id', true)` returns **NULL** when unset → `tenant_id = NULL` is not true → an unbound transaction sees **zero rows** and cannot write (spec scenario "Unbound transaction sees nothing").
- `FORCE` is required because the app role **owns** the tables (it creates them via goose); without it the owner would bypass the policy.
- Application-level `WHERE tenant_id = $n` stays the primary mechanism (unchanged) — RLS is the backstop.

**Binding — never session-scoped, never stale on pooled connections.** The binding statement is `SELECT set_config('app.tenant_id', $1, true)`; the third argument (`is_local = true`) makes it **transaction-scoped** — it reverts on commit/rollback, so a pooled connection can never carry a stale tenant.

Where binding happens is the spec-report's second open point; the constraint is that binding and the protected statement must share one transaction, and `pgxpool` may serve consecutive calls on **different** connections. Hence a `txScope` abstraction (new file `infra/postgres/scope.go`):

```go
type txScope interface {
    // run executes fn inside a tenant-bound transaction scope:
    // app.tenant_id is set (transaction-locally) to tid before fn runs.
    run(ctx context.Context, tid string, fn func(q Querier) error) error
}

type poolScope struct{ pool *pgxpool.Pool } // per-op: Begin → set_config → fn(tx) → Commit (Rollback on error)
type txScopeImpl struct{ tx pgx.Tx }        // ambient tx (InTransaction): re-apply set_config, fn(tx)
```

- `pgRepository` replaces its `q Querier` field with `scope txScope`; every method is `resolveTenant` → `scope.run(ctx, tid, func(q) { <built SQL> })`. Rows are fully drained inside `fn` before the pool scope commits (pgx streams rows on the tx connection).
- **Pool-bound ops open a short transaction each** (BEGIN/COMMIT per statement). This is the only spec-compliant option (session-scoped binding is forbidden; pool connections are not stable across calls). Cost: one extra round-trip per op — accepted at this app's scale (documented trade-off).
- **Tx-bound ops (inside `Factory.InTx`) re-apply `set_config` per op on the ambient transaction** — idempotent within one tx, and it makes an explicit `repo.Tenant` option that differs from the ctx tenant bind correctly (option-wins holds under RLS too). Commit/rollback ownership stays with `InTransaction` (verify-2 rollback semantics unchanged).
- Constructors: `NewAssetRepository(pool *pgxpool.Pool)` etc. (parameter narrows from `Querier`; existing call sites compile unchanged); `InTransaction` builds repos via unexported `newTx…(tx pgx.Tx)` constructors.

**G6 fix — non-superuser app role (spec SHALL).** `POSTGRES_USER` in the postgres image is a **superuser**; superusers bypass RLS unconditionally (FORCE included), so G6 makes the whole RLS requirement vacuous. Fix in the dev container topology:

```yaml
# docker-compose.yml
environment:
  POSTGRES_USER: pgadmin          # bootstrap superuser only; the app never uses it
  POSTGRES_PASSWORD: pgadmin
  POSTGRES_DB: procrastinator
volumes:
  - ./initdb:/docker-entrypoint-initdb.d:ro
  - pgdata:/var/lib/postgresql
```

```sql
-- initdb/01-app-role.sql — runs as pgadmin on FIRST init of an empty volume only
CREATE ROLE procrastinator LOGIN PASSWORD 'procrastinator' NOBYPASSRLS;
ALTER DATABASE procrastinator OWNER TO procrastinator;
CREATE DATABASE procrastinator_test OWNER procrastinator;
```

- DSNs in `.env`/`.env.example` are **unchanged** (`postgres://procrastinator:procrastinator@…`) — the role keeps its name/credentials; it simply stops being a superuser.
- Side benefit: `procrastinator_test` is now **created automatically** (the old "create test DB once" manual step disappears); the app role owns both databases → can create test schemas and owns every table goose creates → `FORCE RLS` binds it everywhere.
- One-time dev migration: existing `pgdata` volumes predate the init script (which never re-runs) → `docker compose down -v && docker compose up -d` resets dev data (dev data is disposable). Documented in the task verify step.
- Fresh CI/local containers are unaffected (init runs on first boot).

**Deployment-ordering risk (accepted):** migrations auto-apply at boot (`main.go` → `store.Migrate`), so RLS (00003) and the binding code ship in the **same binary** — there is no window where the new DB runs under the old code, except a crash between migration and serve followed by an old-binary restart (old code = unbound = sees nothing). Mitigation: deploy atomically; no toggle GUC (YAGNI).

### D5: Whitelisted dynamic queries — per-entity `fieldCols`/`orderCols`, fixed operator set

New per-entity configuration in `infra/postgres` (colocated with the `toMap` functions, `repos.go` or a new `whitelist.go`):

```go
var assetFieldCols = map[string]string{ // filterable field → column
    "brand": "brand", "model": "model", "serial_number": "serial_number",
    "norm_serial": "norm_serial", "norm_brand": "norm_brand", "norm_model": "norm_model",
    "purchase_date": "purchase_date", "warranty_end": "warranty_end",
    "price": "price", "currency": "currency", "doc_type": "doc_type",
    "created_at": "created_at", "updated_at": "updated_at",
}
var assetOrderCols = map[string]string{
    "id": "id", "created_at": "created_at", "updated_at": "updated_at",
    "brand": "brand", "model": "model", "doc_type": "doc_type",
    "purchase_date": "purchase_date", "warranty_end": "warranty_end",
}
// sources: fieldCols = filename, content_type, byte_size, sha256, uploaded_at
//          orderCols = id, uploaded_at, byte_size
// documents: fieldCols = asset_id, source_id, doc_type, created_at
//            orderCols = id, created_at, doc_type
```

- **Operator set (fixed, matches spec):** `=`, `!=`, `<`, `<=`, `>`, `>=`, `LIKE`, `IN`.
  - `IN`: the filter value must be a slice (validated via reflection; non-slice → error); it expands to one bind placeholder per element (`field IN ($n, $n+1, …)`). Values are always bind parameters.
- **`OrderBy` stays a comma-separated string** (v3 `Option` shape preserved — no `commons/repo` change): each comma-separated token is trimmed and must be a key in the entity's `orderCols`. This keeps the existing `repo.OrderBy("created_at, id")` call site in `api/server.go` working unchanged.
- Validation runs in the SQL builder **before any string is assembled**: unknown field / unknown op / unknown order column / non-slice `IN` value → descriptive error (`postgres: unknown filter field %q for table %q`), zero SQL issued. Call-site names used today (`norm_serial`, `norm_brand`, `norm_model`, `doc_type`, `asset_id`, `created_at, id`) are all in the whitelists.
- Whitelist drift is a one-line map entry per new filterable field (v3 risk note stands); rejection of unknown names is unit- + integration-tested.

### D6: Tenant-scoped file storage

`infra/filestorage.Storage.Put` (interface `repo.FileStorage` **unchanged** — tenant arrives via ctx, the spec'd choice over an explicit parameter):

```go
func (s *Storage) Put(ctx context.Context, payload []byte) (entity.Source, error) {
    tid, err := tenant.TenantFrom(ctx)   // fail closed BEFORE sniffing or any I/O
    if err != nil { return entity.Source{}, err }   // tenant.ErrNoTenant, nothing stored
    …
    key  := tid + "/" + uuid + ext        // {tenantID}/{fileID}
    path := filepath.Join(s.dir, key)
    os.MkdirAll(filepath.Dir(path), 0o755)
    os.WriteFile(path, payload, 0o644)
    return entity.Source{ ID: uuid, Filename: uuid, …, Path: key, … }, nil
}
```

- **`Source.Path` stores the tenant-relative key** (`acme/3f2c….pdf`), not an absolute filesystem path — portable across hosts and visibly tenant-prefixed. No read path exists yet (no download endpoint), so old rows with legacy paths are inert; **moving existing flat files is a non-goal** (D: Non-Goals).
- Client-supplied filenames still never reach the key (key = `{tenantID}/{uuid}{ext}`); `Filename` remains the uuid as today.
- `core/ingest.Process` already resolves the tenant from ctx before calling `Put` — no core changes.

### D7: API wiring — registry via `Factory`, bridge removed

- `Routes()`: `r.Use(httpx.TenantMiddleware(s.factory.Tenants))` — the registry travels with the existing `*repo.Factory`; **`api.New`, `ingest.New`, and `main.go` signatures are unchanged** (no composition-root edits in this change).
- `api/server.go`: delete `tenantContext` + its `httpx.TenantFrom` import; `tenantFromCtx`'s defensive branch maps to `500` (unreachable).
- Handler status-code behavior after the change: missing → `400`, malformed → `400` (was `401`), unregistered → `404` (new), registered → as before. `tenant.ErrNoTenant` leaking into a handler is a programming error → default `500` (no dedicated mapping).

### D8: Test strategy

Conventions unchanged (v3 normative): `<prod>_test.go`, table-driven, `t.Parallel()`, one `PROCRASTINATOR_TEST_DATABASE_URL` read per package in `TestMain`/helper, per-package private schemas, explicit test tenants.

**Pure unit (no Docker, no network):**
- `api/httpx/tenant_test.go` — rewritten around an in-memory fake `repo.TenantRegistry` (set + injectable error): missing → 400; malformed table (empty, 65 chars, `a b`, `a/b`, leading/trailing space) → 400; unregistered → 404; registry error → 500; registered → 200 with `tenant.TenantFrom(ctx)` == id. Handler-must-not-run assertion on every rejection.
- `commons/tenant/tenant_test.go` — `Valid` table (valid IDs, >64, bad chars, empty).
- `infra/postgres/repository_test.go` (new, **internal** `package postgres`) — `pgRepository` constructed with a fake `txScope` + a **panic-on-query** fake `Querier`: (a) no option + no ctx tenant → `tenant.ErrNoTenant` and the fake scope is never invoked (zero SQL — the spec's "query-recording or panic-on-query executor" scenario); (b) option beats ctx; (c) unknown field / op / order column / injection-shaped name (`"1; DROP TABLE assets--"`) → error before scope.run; (d) `IN` with non-slice → error; (e) `IN` with slice → correct placeholder count and args.
- `infra/filestorage/storage_test.go` — added: ctx tenant `acme` → file exists at `acme/{uuid}.ext`, `Source.Path == "acme/{uuid}.ext"`; no ctx tenant → `tenant.ErrNoTenant`, **no file on disk**; two tenants → disjoint prefixes.

**Integration (real PG, private schemas):**
- `infra/postgres/generic_test.go` — extended: ctx-fallback (no option, `tenant.WithTenant` ctx) stamps and scopes; neither → `ErrNoTenant`; `IN` operator round-trip; existing option-based tests stay green unchanged.
- `infra/postgres/tenant_registry_test.go` (new) — `Has` true/false; FK enforcement: insert asset with unregistered `tenant_id` → pg error 23503 (spec: "Registry enforcement is itself tested"); delete tenant that owns data → 23503; **backfill**: two-step goose run in a scratch schema (`UpTo` 00001 → seed legacy rows with unknown tenant IDs → `UpTo` 00002 → assert registry rows exist and FKs now enforce).
- `infra/postgres/rls_test.go` (new) — the four spec RLS scenarios: (1) tx bound to `acme`, raw `SELECT * FROM assets` (no `WHERE`) → only `acme` rows; (2) bound to `acme`, `INSERT … tenant_id='globex'` → fails, no `globex` row; (3) unbound pool query → zero rows, unbound insert fails; (4) pool reuse: bound-`acme` tx commits, same pool serves a bound-`globex` tx → only `globex` rows (no residue).
- `infra/postgres/testutil_test.go` — new `ensureTenants(ctx, pool, ids …string)` helper (`INSERT … ON CONFLICT (id) DO NOTHING`): the **single reusable registration step** required by the `test-infrastructure` delta; every new multi-tenant test (rls, registry) registers its tenants through it. (Migration 00002 already seeds `test-tenant`/`test-tenant-b` per schema; the helper covers additional tenants like `acme`/`globex` and makes the convention explicit.)
- `api/handlers_test.go` — `MissingTenant` expectation flips **401 → 400**; new `MalformedTenant` subtest (65-char, bad char) → 400; new `UnregisteredTenant` subtest (well-formed `unknown-tenant`) → 404 on all four endpoints; `newEnv` needs no wiring change (registry comes from `postgres.NewFactory`).
- `core/ingest/service_test.go`, `core/identity/resolve_test.go` — **unchanged** (fakes already resolve the tenant; the `repo.Tenant` option contract is untouched).

**Scenario → test mapping** is verified exhaustively in the final gate (every WHEN/THEN in the three deltas maps to ≥1 test).

## API surface

Endpoints, request shapes, response shapes, and the `{"error": "…"}` envelope are **unchanged**. Status-code deltas for tenant handling only:

| Request | Before | After |
|---|---|---|
| no `X-Tenant-ID` | 400 | 400 |
| malformed `X-Tenant-ID` | 401 (impl drift) | **400** |
| well-formed, unregistered | 200/404/… (header silently accepted) | **404** |
| registered | 200/201/… | unchanged |

## Risks / Trade-offs

- **Per-op transactions for pool-bound repo calls** (BEGIN/COMMIT per statement, D4) — one extra round-trip per op; correctness (spec-forbids session-scoped binding; pool connections are unstable) wins at this scale.
- **Compose topology change requires a one-time `docker compose down -v`** for existing dev volumes (init scripts don't re-run) — dev data loss, documented in the task.
- **RLS migration + binding code ship together** (goose auto-applies at boot): a crash between migration and serve + old-binary restart leaves the DB "dark" (unbound → nothing visible) until the new binary is restored — accepted; documented.
- **`repo.Factory.Tenants` additive field** touches a v3-committed file — purely additive, no signature change; `ingest`/`api` ignore the field.
- **`Source.Path` semantics change** (absolute → tenant-relative key) — no reader exists yet; old rows inert; a future download endpoint must join `StorageDir` + key (documented in the non-goals).
- **Whitelist drift** — one-line map entry per new filterable field; rejection of unknown names is tested.
- **`IN` operator is new surface** (no current call site) — implemented + tested now because the spec names it in the supported set.

## Coordination with `invoice-warranty-asset-flow` (v3)

v3 is **complete and committed** in the working tree at design time (all 7 task groups checked in its `tasks.md`; tree clean at `c2f5a64`; legacy per-entity files and `factory_new.go` gone; `main.go` in final form). This design therefore treats the v3 files as a stable base, not a moving target:

- No task in this change deletes or rewrites v3 interfaces; the only `commons/` edit is the additive `Factory.Tenants` field (D2) and the additive `tenant.Valid` export (D3).
- Files this change modifies that v3 also shaped: `infra/postgres/{repository,repos,factory,scope}.go`, `api/{server,handlers_test,httpx/tenant*}.go`, `infra/filestorage/storage.go`. If a late v3 commit lands before apply, re-verify these at apply start (state.md baseline note already requires base-sha re-verification at COMMIT).
- Spec-delta non-conflict (architect's decisions) is preserved: this design adds no requirements and modifies none beyond the delta text.

## Spec review major — disposition

`review-spec-1.md` is **missing from disk** (recorded in state.md Log; the Reviews table references it but the file was never written / was lost). Against the review criteria (`CRIT-S-*`) and the spec report's open points, the design explicitly disposes of the plausible majors:

1. **CRIT-S-06 — explicit out-of-scope boundaries:** the proposal has no non-goals section; this design carries the authoritative **Non-Goals** list (auth, tenant-admin API, registry cache, file migration, pagination, UI, `BYPASSRLS` role, quotas).
2. **RLS "non-superuser role" SHALL vs. compose reality (G6):** the spec's RLS requirement is unimplementable as-is (superuser app role); D4 fixes the container topology so the spec is actually enforceable.
3. **Status-code drift (401 vs spec'd 400):** D3/D7 close the drift with the 400/404/500 table.
4. **Spec-report open points for the designer**, all decided: registry **lookup** (not cache) owned by the middleware via `repo.TenantRegistry`; RLS binding **placement** = per-op `txScope` (pool: per-op tx; `InTx`: re-bind ambient tx); **404** (not 403) for unregistered tenants; `FileStorage` tenant via **ctx** (interface unchanged).
