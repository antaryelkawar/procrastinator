# Proposal: multi-tenant-isolation

## Why

Isolation today is conventional, not enforced: any non-empty identifier string silently becomes a tenant, a repository call that forgets `repo.Tenant(...)` runs across all data, filter fields and operators are string-interpolated into SQL without a whitelist, and uploaded files live in a flat directory. Authentication is deferred, so the data layer is the only line of defense — it must fail closed. The tenancy model has been redefined to focus on the User as the primary isolation boundary, with Households providing shared scope.

## What Changes

- **User-based tenancy.** A "Tenant" is now a User. The `tenants` table (kept for stability) stores User IDs in its `id` column. No separate organization layer exists.
- **URL-based transport.** The `X-Tenant-ID` header is dropped. The active user (tenant) is identified by the `{userId}` parameter in the URL path (e.g., `/api/users/{userId}/documents`).
- **Fail-closed data layer.** Every repository operation resolves a user from the explicit `repo.Tenant(id)` option or the request context; with neither, it returns `ErrNoTenant` and issues no SQL.
- **User registry.** The `tenants` table is the source of truth for valid User IDs. Requests naming an unregistered user are rejected; data rows reference this registry.
- **Household and Scope.** Introduces `households` and `household_members` for shared data access. Tenant-owned rows (assets, documents, sources) gain a `scope` dimension (`personal` vs `household`). A user can access their personal data or data in any household they belong to.
- **Defense-in-depth at the database.** Row-Level Security on every tenant-owned table, driven by a transaction-scoped user setting. RLS remains strictly tenant-level (User ID) for simplicity; household-based sharing is enforced at the application level.
- **User-scoped file storage.** Stored file keys are prefixed per user (`{userId}/{uuid}`), the storage API resolves the user from the context, and raw client-supplied paths never reach the storage layer.
- **SQL-injection-safe dynamic queries.** Filter fields, operators, and order-by columns are validated against per-entity whitelists; unknown names are rejected with an error, never interpolated.

## Capabilities

### New Capabilities

(None — all behavior lands in existing capabilities.)

### Modified Capabilities

- `multitenancy`: user-based tenancy; URL-based transport (`/api/users/{userId}/...`); household/scope model for shared access; fail-closed resolution (`ErrNoTenant`); user registry and RLS defense-in-depth.
- `backend-platform`: whitelisted dynamic queries (filter fields/operators/order-by).
- `test-infrastructure`: test users must be explicitly registered in the registry.

## Impact

- **Code:** `procrastinator-backend/commons/tenant`, `commons/repo` (options validation surface), `commons/service` (FileStorage contract), `infra/postgres` (generic repository tenant resolution, whitelist, RLS transaction hook), `infra/filestorage` (per-tenant key layout), `api/httpx` (tenant middleware: format + registry validation), `api/cmd/procrastinator` (wiring).
- **Schema:** new goose migration — `tenants` table, FKs from `sources`/`assets`/`documents`, RLS policies + `FORCE ROW LEVEL SECURITY`, seeded dev/test tenants. Existing rows need their tenant IDs present in the registry (backfill in the same migration).
- **API:** malformed `X-Tenant-ID` now reliably `400`; unknown tenant `404`. No endpoint-shape changes.
- **Dependencies:** none new (pgx + goose already present).
- **Coordination:** `invoice-warranty-asset-flow` (v3) is in flight and holds open deltas on `multitenancy`, `backend-platform`, and `test-infrastructure`. This change's deltas are written against that v3 target contract (explicit `repo.Tenant` option, context fallback) and only ADD new requirements or MODIFY requirements whose v3 text is unchanged from baseline, so the two changes can archive in either order without semantic conflict.
