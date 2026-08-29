# Tasks: multi-tenant-isolation

**Backend-only change** (UI is separate change B). Every task is tagged `[backend]`.

**Base state:** the v3 refactor (`invoice-warranty-asset-flow`) is complete and committed (flat layout, generic `Repository[T]`, `repo.Factory`/`Repos` + `InTx`, all v3 tasks checked, tree clean). Tasks below modify that base; at apply start, re-verify the base sha from `.autopilot/change-multi-tenant-isolation/state.md` (the in-flight v3 work was the recorded coordination risk — if any late v3 commit landed, re-check tasks 7–9 and 11–12 for drift before starting).

**Normative conventions (apply to every task)** — carried over verbatim from v3: tests in `<prod_file>_test.go`; table-driven (case structs + `t.Run`); `t.Parallel()` in every independent test; **no env vars in tests** except one `PROCRASTINATOR_TEST_DATABASE_URL` read per package (TestMain/single helper); integration tests use **per-package private PostgreSQL schemas** (search_path DSN) and explicit test tenants (`test-tenant`, `test-tenant-b`, or additionally registered `acme`/`globex`); the module must build and test green after **every** task. All paths relative to repo root; Go module root is `procrastinator-backend/`.

**Spec mapping** — every task names the delta requirement(s) it satisfies so the final gate can verify scenario → test coverage exhaustively.

## 1. [backend] Migration 00002 — tenant registry table, backfill, explicit seed, FKs [x]

Implements: `multitenancy` — **Tenant registry** (all four scenarios).

Create `procrastinator-backend/migrations/00002_tenant_registry.sql` (design D2): `tenants` table (`id text PRIMARY KEY` + format `CHECK`, `created_at`); backfill `INSERT … ON CONFLICT DO NOTHING` from the distinct `tenant_id` values of `sources`/`assets`/`documents` **before** FK enforcement; explicit seed of `test-tenant`/`test-tenant-b`; three `ADD CONSTRAINT … FOREIGN KEY (tenant_id) REFERENCES tenants(id)` (default `ON DELETE RESTRICT`); full `-- +goose Down` (drop FKs, drop table).

- **Files:** `procrastinator-backend/migrations/00002_tenant_registry.sql` (new)
- **Depends on:** —
- **Verify:** scratch DB (compose PG): apply goose, then confirm by hand — (a) a DB pre-seeded with rows under an unknown tenant ID gets registry rows after migration; (b) inserting an `assets` row with an unregistered `tenant_id` fails with FK violation 23503; (c) deleting a `tenants` row that owns data fails with 23503; (d) `go build ./...` green (no Go changes).

## 2. [backend] Migration 00003 — RLS policies + FORCE on tenant-owned tables [x]

Implements: `multitenancy` — **Row-level security defense-in-depth** (policy side of the requirement).

Create `procrastinator-backend/migrations/00003_row_level_security.sql` (design D4): for each of `sources`, `assets`, `documents` — `ENABLE ROW LEVEL SECURITY`, `FORCE ROW LEVEL SECURITY`, `DROP POLICY IF EXISTS <t>_tenant_isolation`, `CREATE POLICY <t>_tenant_isolation … USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (same)`; full `-- +goose Down` (drop policy, `NO FORCE`, `DISABLE` per table).

- **Files:** `procrastinator-backend/migrations/00003_row_level_security.sql` (new)
- **Depends on:** 1
- **Verify:** scratch DB: after goose, `SELECT relname, relrowsecurity, relforcerowsecurity FROM pg_class WHERE relname IN ('sources','assets','documents')` shows both flags on for all three tables; as a non-superuser role (hand-created if task 3's topology is not yet in place) an unbound raw `SELECT` returns zero rows, and after `SELECT set_config('app.tenant_id','test-tenant',true)` in a transaction the same `SELECT` returns only that tenant's rows; `go build ./...` green. (Full four-scenario RLS coverage lands in task 14.)

## 3. [backend] Non-superuser app role — compose topology + init script [x]

Implements: `multitenancy` — **RLS** ("The application database role SHALL NOT be a superuser and SHALL NOT hold `BYPASSRLS`").

Per design D4: change `docker-compose.yml` to `POSTGRES_USER: pgadmin` / `POSTGRES_PASSWORD: pgadmin` / `POSTGRES_DB: procrastinator`, mount `./initdb:/docker-entrypoint-initdb.d:ro`; add `procrastinator-backend/initdb/01-app-role.sql` that (running as pgadmin on first init only) creates `procrastinator` with `LOGIN PASSWORD 'procrastinator' NOBYPASSRLS`, sets `ALTER DATABASE procrastinator OWNER TO procrastinator`, and creates `procrastinator_test OWNER procrastinator`. DSNs in `.env.example` stay unchanged. Document the one-time dev reset (`docker compose down -v && docker compose up -d`) in the file header comment.

- **Files:** `procrastinator-backend/docker-compose.yml`, `procrastinator-backend/initdb/01-app-role.sql` (new)
- **Depends on:** 2
- **Verify:** `docker compose down -v && docker compose up -d` from `procrastinator-backend/`; via psql as pgadmin: `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname='procrastinator'` → both false; both databases exist and are owned by `procrastinator`; the binary boots (`go run ./api/cmd/procrastinator` with `.env`) and migrates both DBs cleanly; `go build ./...` green.

## 4. [backend] commons/repo — `TenantRegistry` boundary + additive `Factory.Tenants` [x]

Implements: boundary for `multitenancy` — **Tenant registry** (lookup side).

Add `procrastinator-backend/commons/repo/registry.go` with `type TenantRegistry interface { Has(ctx context.Context, id string) (bool, error) }` (design D2). Add one field `Tenants TenantRegistry` to `repo.Factory` in `commons/repo/factory.go` (purely additive — no signature changes anywhere). `commons/` stays stdlib-only (layering rule).

- **Files:** `procrastinator-backend/commons/repo/registry.go` (new), `procrastinator-backend/commons/repo/factory.go`
- **Depends on:** —
- **Verify:** `go build ./...` green; `go list -deps ./commons/...` still stdlib + `commons/entity` only.

## 5. [backend] commons/tenant — exported format validator `tenant.Valid` [x]

Implements: shared format source for `multitenancy` — **Tenant identification on every request** (malformed → 400).

Export `Valid(id string) bool` in `commons/tenant/tenant.go` backed by the existing `^[A-Za-z0-9_-]{1,64}$` pattern; use it inside `WithTenant` too (single source of truth). Extend `commons/tenant/tenant_test.go` with a table of valid IDs and invalid ones (empty, 65 chars, space, `/`, `;`, leading `-`).

- **Files:** `procrastinator-backend/commons/tenant/tenant.go`, `procrastinator-backend/commons/tenant/tenant_test.go`
- **Depends on:** —
- **Verify:** `go test ./commons/...` green without Docker/network.

## 6. [backend] infra/postgres — `TenantRegistry` implementation + `NewFactory` wiring [x]

Implements: `multitenancy` — **Tenant registry** (lookup implementation).

Add `procrastinator-backend/infra/postgres/tenant_registry.go`: `NewTenantRegistry(q Querier) repo.TenantRegistry` — `Has` via `SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1)` (the `tenants` table has no RLS, so no tenant binding is needed or performed). Update `NewFactory` in `factory.go` to populate `Factory.Tenants` from the pool. Add integration tests (private schema `p_generic`, reusing `openTestPool`): `Has` true for seeded `test-tenant`, false for `unknown-tenant`, true after inserting a registry row.

- **Files:** `procrastinator-backend/infra/postgres/tenant_registry.go` (new), `procrastinator-backend/infra/postgres/tenant_registry_test.go` (new), `procrastinator-backend/infra/postgres/factory.go`
- **Depends on:** 1, 4
- **Verify:** `go test ./infra/postgres/ -run 'TestTenantRegistry' -count=1` green against compose PG (skips explicitly without env); `go build ./...` green.

## 7. [backend] infra/postgres — fail-closed tenant resolution in `pgRepository` [x]

Implements: `multitenancy` — **Tenant-scoped persistence** (all four scenarios, incl. "Tenantless call issues no SQL").

Restructure all five `pgRepository[T]` methods (design D1): first statement resolves the tenant — `Options.TenantID` if set (option wins), else `tenant.TenantFrom(ctx)`, else return `tenant.ErrNoTenant` **before any SQL is built or sent**; `Create`/`Update` stamp `tenant_id` from the resolved tenant. (The `txScope` binding lands in task 9; until then the repo keeps its `q Querier` field.) Add internal unit tests `procrastinator-backend/infra/postgres/repository_test.go` (package `postgres`) constructing `pgRepository` with a panic-on-query fake `Querier`: no-option + no-ctx-tenant → `ErrNoTenant` with zero queries for each of Get/List/Create/Update/Delete (the spec's query-recording/panic-on-query scenario); explicit option + different ctx tenant → option wins (a recording fake asserts the bound `tenant_id` argument equals the option value). Extend `generic_test.go` (integration): ctx-fallback (no option, `tenant.WithTenant` ctx) stamps and scopes rows; neither → `ErrNoTenant`; foreign-tenant invisibility unchanged.

- **Files:** `procrastinator-backend/infra/postgres/repository.go`, `procrastinator-backend/infra/postgres/repository_test.go` (new), `procrastinator-backend/infra/postgres/generic_test.go`
- **Depends on:** —
- **Verify:** `go test ./infra/postgres/ -count=1` green (unit part without Docker; integration with compose PG, explicit skip without); `go build ./...` green.

## 8. [backend] infra/postgres — whitelisted filter fields, operators, and order columns [x]

Implements: `backend-platform` — **Safe dynamic query construction** (all four scenarios).

Per design D5: add per-entity `fieldCols` (filterable field → column) and `orderCols` (orderable column → column) maps for assets/sources/documents (covering every field the existing call sites use: `norm_serial`, `norm_brand`, `norm_model`, `doc_type`, `asset_id`, `created_at`, `id`); fixed operator set `= != < <= > >= LIKE IN` with `IN` accepting only slice values and expanding to per-element bind placeholders; validate every filter field, operator, and each comma-separated `OrderBy` token **before** SQL assembly — unknown name/operator → descriptive error, zero SQL issued. Extend the internal unit tests: unknown field/op/order rejected with no query; injection-shaped name (`1; DROP TABLE assets--`) rejected; `IN` non-slice rejected; `IN` slice → correct placeholder count + args. Extend `generic_test.go`: `IN` operator integration round-trip; two-filter and order-by behaviors still pass.

- **Files:** `procrastinator-backend/infra/postgres/repository.go`, `procrastinator-backend/infra/postgres/repos.go` (whitelist maps), `procrastinator-backend/infra/postgres/repository_test.go`, `procrastinator-backend/infra/postgres/generic_test.go`
- **Depends on:** 7
- **Verify:** `go test ./infra/postgres/ -count=1` green (unit + integration); `go build ./...` green; `grep -n 'Sprintf("%s %s' procrastinator-backend/infra/postgres/` finds no raw field/op interpolation.

## 9. [backend] infra/postgres — transaction-scoped tenant binding (`txScope`) [x]

Implements: `multitenancy` — **RLS** (binding side: "bind … at the start of every transaction that touches tenant-owned tables", transaction-scoped, no stale tenant on pooled connections).

Per design D4: add `procrastinator-backend/infra/postgres/scope.go` with the `txScope` abstraction — pool-backed scope opens a transaction per operation (`Begin` → `SELECT set_config('app.tenant_id', $1, true)` → run → `Commit`, rollback on error; rows drained before commit); ambient-tx scope re-applies the same `set_config` on the surrounding transaction before running (idempotent; commit/rollback stay with `InTransaction`). `pgRepository`'s `q Querier` field becomes `scope txScope`; `NewAssetRepository`/`NewSourceRepository`/`NewDocumentRepository` take `*pgxpool.Pool` (existing call sites unchanged); `Factory.InTx` builds repos via new unexported tx-bound constructors. Unit tests (internal): fake scope records that binding precedes the statement for both pool and ambient paths, and that an ambient scope re-binds per operation (option-wins under RLS). Integration: existing `TestGenericFactoryInTx` commit/rollback scenarios stay green; a pool-bound repo call and a tx-bound repo call both see only their bound tenant's rows.

- **Files:** `procrastinator-backend/infra/postgres/scope.go` (new), `procrastinator-backend/infra/postgres/repository.go`, `procrastinator-backend/infra/postgres/repos.go`, `procrastinator-backend/infra/postgres/factory.go`, `procrastinator-backend/infra/postgres/repository_test.go`, `procrastinator-backend/infra/postgres/generic_test.go`
- **Depends on:** 8
- **Verify:** `go test ./infra/postgres/ -count=1` green (unit + integration); `go build ./...` green; `core/` still imports no `infra/` (`go list -deps ./core/...` clean).

## 10. [backend] infra/filestorage — tenant-scoped keys, fail-closed `Put` [x] [x]

Implements: `multitenancy` — **Tenant-scoped file storage** (all three scenarios).

Per design D6: `Storage.Put` resolves the tenant from ctx via `tenant.TenantFrom` **before** sniffing or any I/O (no tenant → `tenant.ErrNoTenant`, nothing written); files are written under `{tenantID}/{uuid}{ext}` (create the tenant directory); `Source.Path` records the tenant-relative key `{tenantID}/{fileID}`; client-supplied names still never reach the key. Extend `storage_test.go` (pure, `t.TempDir`): upload under ctx tenant `acme` → file at `acme/{uuid}.ext` + `Path == "acme/{uuid}.ext"`; no-tenant ctx → `ErrNoTenant` + no file on disk; two tenants → disjoint key prefixes.

- **Files:** `procrastinator-backend/infra/filestorage/storage.go`, `procrastinator-backend/infra/filestorage/storage_test.go`
- **Depends on:** —
- **Verify:** `go test ./infra/filestorage/ -count=1` green without Docker/network; `go build ./...` green.

## 11. [backend] api/httpx — middleware: format + registry validation (400/404/500) [x]

Implements: `multitenancy` — **Tenant identification on every request** (all four scenarios).

Per design D3/D7: `TenantMiddleware` becomes a factory taking a `repo.TenantRegistry`; decision table — missing/empty → 400; malformed (`!tenant.Valid`) → 400; registry lookup error → 500; well-formed unregistered → 404; registered → `tenant.WithTenant` into ctx and delegate. **Delete** `httpx.TenantFrom` and the `httpx`-local `tenantKey` (the `commons/tenant` key is the only context key). Keep the module buildable in the same task: `Routes()` applies `httpx.TenantMiddleware(s.factory.Tenants)` and nothing else; delete `Server.tenantContext`; `tenantFromCtx`'s defensive branch now maps to 500 (unreachable invariant — the middleware guarantees the ctx tenant). Rewrite `tenant_test.go` around an in-memory fake registry: table over missing/empty/65-char/bad-char/`a b` headers → 400 with handler-not-called; unregistered → 404; registry error → 500; registered → 200 and `tenant.TenantFrom(r.Context())` returns the id; JSON error envelope on every rejection.

- **Files:** `procrastinator-backend/api/httpx/tenant.go`, `procrastinator-backend/api/httpx/tenant_test.go`, `procrastinator-backend/api/server.go`
- **Depends on:** 4, 5, 6
- **Verify:** `go test ./api/httpx/ -count=1` green without Docker/network; `go build ./...` green; `go test ./api/ -count=1` still green against compose PG with the **old** 401/400 expectations untouched (contract tests update in task 12 — if the old `MissingTenant` 401 expectation breaks under the new middleware, update that one assertion to 400 in this task and note it in the report).

## 12. [backend] api — header contract tests: 400 malformed, 404 unregistered [x]

Implements: `multitenancy` — **Tenant identification on every request** (handler-visible contract) + closes the 401 drift.

Update `api/handlers_test.go` (design D7): `MissingTenant` expectation is 400 (flip the leftover 401 if task 11 did not); add `MalformedTenant` subtest (65-char header and bad-character header → **400** on all four endpoints); add `UnregisteredTenant` subtest (well-formed `unknown-tenant` → **404** on all four endpoints, no data read or written). `newEnv` needs no wiring change (the registry arrives via `postgres.NewFactory`).

- **Files:** `procrastinator-backend/api/handlers_test.go`
- **Depends on:** 11
- **Verify:** `go test ./api/ -count=1` green against compose PG (skips explicitly without env); `go build ./...` green; `grep -rn "tenantContext\|httpx.TenantFrom" procrastinator-backend/api/` finds nothing.

## 13. [backend] infra/postgres — test-tenant registration helper + registry enforcement tests [x]

Implements: `test-infrastructure` — **Test tenant registration** (all three scenarios) + `multitenancy` — **Tenant registry** (registry-enforcement scenario).

Add `ensureTenants(ctx, pool, ids …string)` to `procrastinator-backend/infra/postgres/testutil_test.go` (idempotent `INSERT … ON CONFLICT (id) DO NOTHING`) — the single reusable registration step; new multi-tenant tests register their tenants (e.g., `acme`, `globex`) through it before seeding. In `tenant_registry_test.go` add: inserting a tenant-owned row for an **unregistered** tenant fails with FK violation 23503 (registry enforcement is itself tested); cross-tenant cases register both explicit tenants via the helper and seed/read under the two distinct ids; backfill — scratch schema with a two-step goose run (`UpTo` 00001 → seed rows under a legacy tenant id → `UpTo` 00002) asserting the legacy id gained a registry row before FK enforcement.

- **Files:** `procrastinator-backend/infra/postgres/testutil_test.go`, `procrastinator-backend/infra/postgres/tenant_registry_test.go`
- **Depends on:** 1, 6
- **Verify:** `go test ./infra/postgres/ -run 'TestTenantRegistry|TestBackfill' -count=1` green against compose PG; `go vet ./...` green.

## 14. [backend] infra/postgres — RLS integration tests (four spec scenarios) [x]

Implements: `multitenancy` — **Row-level security defense-in-depth** (all four scenarios).

New `procrastinator-backend/infra/postgres/rls_test.go` (private schema via `openTestPool`, tenants registered via `ensureTenants`): (1) transaction bound to `acme` — raw `SELECT * FROM assets` with no `WHERE tenant_id` returns only `acme` rows; (2) bound to `acme` — `INSERT`/`UPDATE` with `tenant_id = 'globex'` fails and no `globex` row is created/modified; (3) unbound transaction (no `set_config`) — zero rows returned and inserts fail; (4) pool reuse — a bound-`acme` tx commits, the pool connection is then used for a bound-`globex` tx, which observes only `globex` rows (no residue).

- **Files:** `procrastinator-backend/infra/postgres/rls_test.go` (new)
- **Depends on:** 2, 3, 9, 13
- **Verify:** `go test ./infra/postgres/ -run 'TestRLS' -count=1` green against compose PG with the task-3 topology (proves the policies bind the non-superuser app role); default-parallel `-count=5` stable.

## 15. [backend] Final gate [x]

Per v3 gate conventions, from `procrastinator-backend/`: `go vet ./...`; `gofmt -l` empty; `go build ./...`; full `go test ./... -count=1` green with compose PG up (integration) and without (explicit skips); default-parallel `-count=3` stable; env-grep confirms no env access in `*_test.go` outside sanctioned helpers; **layering** — `go list -deps ./core/...` free of `pgx`/`net/http`/`infra`, `commons/` stdlib-only, no `infra/`-to-`infra/` imports; **interface guards** compile (by construction); `grep -rn "tenantContext\|httpx.TenantFrom\|Sprintf(\"%s %s" procrastinator-backend/` finds nothing outside archived openspec artifacts; **scenario → test mapping table** covering every WHEN/THEN in the three delta files (multitenancy ×2 MOD + ×3 ADD, backend-platform ×1 ADD, test-infrastructure ×1 ADD) with the test name for each; `openspec validate multi-tenant-isolation --strict` passes.

- **Files:** (verification only — no production edits expected; fix-forward any gap found)
- **Depends on:** all

## 16. [backend] Migration 00005 — household/scope tables + scope columns [x]

`migrations/00005_household_scope.sql`: `households` (id, tenant_id, display_name) + `household_members` (household_id, user_id, unique per pair); `scope_type` (personal|household) + `owner_household_id` on sources/assets/documents with mutual-exclusion CHECKs; matching-tenant invariant as a BEFORE INSERT OR UPDATE trigger; 5 access-pattern indexes; RLS unchanged (household scope is app-level per spec). Verified: goose 00001→00005 clean + Down reverses, constraint/trigger behavior, backfill default personal.

## 17. [backend] Entities + household repo + scope query helper [x]

`ScopeType`/`OwnerHouseholdID` on Asset/Source/Document; `entity.Household`/`HouseholdMember` + scope constants; `scope_type`/`owner_household_id` in per-entity whitelists; `ListScoped` helper (personal OR member-household rows); `HouseholdRepository` (CRUD + AddMember/ListMembers/HouseholdsForUser) + `Factory.Households` wiring. 3 integration tests (household CRUD, membership, scope visibility matrix) pass vs real PG.

## 18. [backend] API: /api/users/{userId} routes + scope-aware handlers [x]

Routes: `POST /api/users/{userId}/documents`, `GET /api/users/{userId}/assets`, `GET /api/users/{userId}/assets/{assetId}`, `GET /api/users/{userId}/assets/{assetId}/documents` (old flat routes removed). Middleware reads `chi.URLParam(r,"userId")` (X-Tenant-ID header dropped for these routes; transitional fallback kept only for the other change's unmigrated finance routes). Handlers use ListScoped; invisible asset → 404. Contract tests retargeted to path param + scope visibility cases.

## 19. [backend] Final gate (revised spec: user-tenancy + household/scope) [x]

Full gate over tasks 1–19: vet/gofmt/build clean; `go test ./...` green with PG (-count=1, -count=3 stable) + explicit skips without; env-grep + layering clean; forbidden-pattern greps clean (X-Tenant-ID only in documented finance fallback); scenario→test mapping 17/17; `openspec validate multi-tenant-isolation --strict` passes.
