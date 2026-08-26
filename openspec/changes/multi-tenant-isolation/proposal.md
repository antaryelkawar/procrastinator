# Proposal: multi-tenant-isolation

## Why

Tenant isolation today is conventional, not enforced: any non-empty `X-Tenant-ID` header string silently becomes a tenant, a repository call that forgets `repo.Tenant(...)` runs across all tenants, filter fields and operators are string-interpolated into SQL without a whitelist, and uploaded files live in a flat directory with no tenant partitioning. Authentication is deferred, so the storage layer is the only line of defense — it must fail closed.

## What Changes

- **Fail-closed data layer.** Every repository operation resolves a tenant from the explicit `repo.Tenant(id)` option or the request context; with neither, it returns `ErrNoTenant` and issues no SQL. (Aligns implementation with the v3 `multitenancy` delta from `invoice-warranty-asset-flow`; this change adds the enforcement guarantee and tests, it does not redefine the option-vs-context precedence.)
- **Tenant registry.** A `tenants` table becomes the source of truth for valid tenant IDs. Requests naming an unregistered tenant are rejected; tenant-owned rows reference the registry. Provisioning/seeding of tenants is explicit (migration + admin seeding), never implicit from a header.
- **Header contract alignment.** Missing *or malformed* `X-Tenant-ID` → `400 Bad Request` (per spec); a well-formed but unknown tenant → `404 Not Found`. Fixes spec/impl drift around invalid-header status codes.
- **Defense-in-depth at the database.** Row-Level Security on every tenant-owned table, driven by a transaction-scoped tenant setting, so a forgotten `WHERE tenant_id = ...` cannot leak rows even if application code regresses.
- **Tenant-scoped file storage.** Stored file keys are prefixed per tenant (`{tenantID}/{fileID}`), the storage API resolves the tenant from the context, and raw client-supplied paths never reach the storage layer.
- **SQL-injection-safe dynamic queries.** Filter fields, operators, and order-by columns are validated against per-entity whitelists; unknown names are rejected with an error, never interpolated.

## Capabilities

### New Capabilities

(None — all behavior lands in existing capabilities.)

### Modified Capabilities

- `multitenancy`: fail-closed tenant resolution in the data layer (`ErrNoTenant`, no query issued); header validation contract (malformed → 400, unknown → 404); new requirements for the tenant registry, RLS defense-in-depth, and tenant-scoped file storage. Builds on — does not redefine — the `repo.Tenant` option + context-fallback contract from `invoice-warranty-asset-flow` v3.
- `backend-platform`: new requirement that dynamically constructed queries whitelist filter fields/operators/order-by and reject unknowns.
- `test-infrastructure`: new requirement that test tenants are explicitly registered (seeded) in the tenant registry, preserving the explicit-test-tenant convention under the registry.

## Impact

- **Code:** `procrastinator-backend/commons/tenant`, `commons/repo` (options validation surface), `commons/service` (FileStorage contract), `infra/postgres` (generic repository tenant resolution, whitelist, RLS transaction hook), `infra/filestorage` (per-tenant key layout), `api/httpx` (tenant middleware: format + registry validation), `api/cmd/procrastinator` (wiring).
- **Schema:** new goose migration — `tenants` table, FKs from `sources`/`assets`/`documents`, RLS policies + `FORCE ROW LEVEL SECURITY`, seeded dev/test tenants. Existing rows need their tenant IDs present in the registry (backfill in the same migration).
- **API:** malformed `X-Tenant-ID` now reliably `400`; unknown tenant `404`. No endpoint-shape changes.
- **Dependencies:** none new (pgx + goose already present).
- **Coordination:** `invoice-warranty-asset-flow` (v3) is in flight and holds open deltas on `multitenancy`, `backend-platform`, and `test-infrastructure`. This change's deltas are written against that v3 target contract (explicit `repo.Tenant` option, context fallback) and only ADD new requirements or MODIFY requirements whose v3 text is unchanged from baseline, so the two changes can archive in either order without semantic conflict.
