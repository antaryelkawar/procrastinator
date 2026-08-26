# Spec: multitenancy

Delta for change `multi-tenant-isolation`. Modifies the baseline in `openspec/specs/multitenancy`.

This delta is written against the v3 target contract of `invoice-warranty-asset-flow` (explicit `repo.Tenant(id)` option with context fallback). The two MODIFIED requirements below incorporate that contract in full and add enforcement guarantees; they do not redefine the option-vs-context precedence, the header name/format, or the isolation and test-tenant conventions, which are unchanged.

## MODIFIED Requirements

### Requirement: Tenant identification on every request

Every API request SHALL carry a tenant identifier in the `X-Tenant-ID` header. Middleware SHALL validate the header format (non-empty, at most 64 characters from `[A-Za-z0-9_-]`) and SHALL validate that the identifier names a registered tenant (see "Tenant registry"). A request with a missing header SHALL be rejected with `400 Bad Request`; a request with a malformed header SHALL be rejected with `400 Bad Request`; a request with a well-formed but unregistered tenant SHALL be rejected with `404 Not Found`. In all rejection cases the rejection SHALL occur before any handler or repository code runs and no data SHALL be read or written. For accepted requests, middleware SHALL place the tenant identifier into the request `context.Context` before invoking any handler.

#### Scenario: Request without tenant header is rejected

- **WHEN** a client sends any API request without an `X-Tenant-ID` header
- **THEN** the response is `400 Bad Request` and no data is read or written

#### Scenario: Malformed tenant header is rejected with 400

- **WHEN** a client sends a request with an `X-Tenant-ID` header that is empty, longer than 64 characters, or contains characters outside `[A-Za-z0-9_-]`
- **THEN** the response is `400 Bad Request` and no data is read or written

#### Scenario: Unregistered tenant is rejected with 404

- **WHEN** a client sends a request with a well-formed `X-Tenant-ID` header naming a tenant that is not in the tenant registry
- **THEN** the response is `404 Not Found` and no data is read or written

#### Scenario: Valid registered tenant header reaches the handler via context

- **WHEN** a client sends a request with header `X-Tenant-ID: acme` and `acme` is a registered tenant
- **THEN** downstream handlers, services, and repositories observe tenant `acme` from the request context

### Requirement: Tenant-scoped persistence

Every tenant-owned table (`sources`, `assets`, `documents`) SHALL carry a `tenant_id` column referencing the tenant registry. Every repository call SHALL pass the tenant explicitly via the `repo.Tenant(id)` option; when the option is absent the repository SHALL fall back to the tenant carried in the passed `context.Context`. The explicit option SHALL take precedence over the context when both are present. All reads and writes SHALL be restricted to rows of the resolved tenant. A repository method invoked with neither an explicit tenant option nor a context tenant SHALL return the sentinel error `ErrNoTenant` and SHALL NOT issue any SQL statement — tenant enforcement fails closed at the data layer, not merely by convention.

#### Scenario: Repository writes stamp the context tenant

- **WHEN** an Asset is created in a context carrying tenant `acme` (with no explicit tenant option)
- **THEN** the persisted row has `tenant_id = 'acme'`

#### Scenario: Explicit tenant option wins over context

- **WHEN** a repository method is called with option `repo.Tenant("acme")` in a context carrying tenant `globex`
- **THEN** the operation reads or writes rows of tenant `acme` only

#### Scenario: Repository call without tenant fails closed

- **WHEN** any repository method is called with neither a tenant option nor a context tenant
- **THEN** it returns `ErrNoTenant` and performs no query

#### Scenario: Tenantless call issues no SQL

- **WHEN** any repository method (Get, List, Create, Update, Delete) is called with neither a tenant option nor a context tenant, with a query-recording or panic-on-query executor
- **THEN** the error is `ErrNoTenant` and zero SQL statements reach the database

## ADDED Requirements

### Requirement: Tenant registry

The system SHALL maintain a tenant registry as a `tenants` table holding one row per valid tenant identifier. Every `tenant_id` value stored in a tenant-owned table (`sources`, `assets`, `documents`) SHALL reference a registry row via a foreign key. Tenants SHALL be created only by explicit provisioning (migrations or administrative seeding); a request header value SHALL never implicitly create a tenant. Deleting a registry row that still owns data SHALL be rejected by the database.

#### Scenario: Unknown header value never becomes a tenant

- **WHEN** a request arrives with a well-formed `X-Tenant-ID` naming no registry row
- **THEN** the request is rejected and the registry contains no new row afterwards

#### Scenario: Tenant-owned rows require a registry row

- **WHEN** a row is inserted into `assets` with a `tenant_id` that does not exist in `tenants`
- **THEN** the insert fails with a foreign-key violation

#### Scenario: Deleting a tenant that owns data is rejected

- **WHEN** a `tenants` row is deleted while `sources`, `assets`, or `documents` rows still carry its identifier
- **THEN** the delete fails with a foreign-key violation and the tenant's data is untouched

#### Scenario: Existing data is backfilled into the registry

- **WHEN** the registry migration runs on a database containing tenant-owned rows
- **THEN** every distinct pre-existing `tenant_id` has a corresponding registry row before the foreign keys are enforced

### Requirement: Row-level security defense-in-depth

Every tenant-owned table SHALL have PostgreSQL row-level security enabled and forced, with a policy restricting visibility and writes to rows whose `tenant_id` matches the tenant bound to the current transaction. The application SHALL bind the resolved tenant to a transaction-scoped database setting (never a session-scoped one) at the start of every transaction that touches tenant-owned tables, so pooled connections can never carry a stale tenant. The application database role SHALL NOT be a superuser and SHALL NOT hold `BYPASSRLS`. Row-level security is defense-in-depth: application-level `WHERE tenant_id = ...` scoping remains the primary mechanism and SHALL NOT be removed.

#### Scenario: Unscoped query returns only the bound tenant's rows

- **WHEN** a transaction bound to tenant `acme` executes a raw `SELECT` on `assets` with no `WHERE tenant_id` clause
- **THEN** only rows with `tenant_id = 'acme'` are returned

#### Scenario: Cross-tenant write is rejected by policy

- **WHEN** a transaction bound to tenant `acme` attempts to insert or update a row with `tenant_id = 'globex'`
- **THEN** the statement fails and no `globex` row is created or modified

#### Scenario: Unbound transaction sees nothing

- **WHEN** a transaction that has not bound any tenant queries a tenant-owned table
- **THEN** zero rows are returned and no rows can be written

#### Scenario: Tenant binding does not leak across pooled connections

- **WHEN** a transaction bound to tenant `acme` commits and its connection is reused from the pool for a transaction bound to tenant `globex`
- **THEN** the second transaction observes only `globex` rows, with no residue of `acme`'s binding

### Requirement: Tenant-scoped file storage

Stored file content SHALL be laid out under a per-tenant key prefix of the form `{tenantID}/{fileID}`. The storage layer SHALL resolve the tenant from the passed `context.Context`; a storage operation invoked with no tenant in context SHALL return `ErrNoTenant` and store nothing. Client-supplied paths or filenames SHALL never reach the storage layer as storage keys. The tenant-scoped reference row (`sources`) remains the authoritative record linking a stored object to its tenant.

#### Scenario: Uploaded file lands under the tenant prefix

- **WHEN** a document is uploaded in a context carrying tenant `acme`
- **THEN** the file bytes are stored under a key beginning with `acme/` and the `sources` row records `tenant_id = 'acme'`

#### Scenario: Files of two tenants never share a key prefix

- **WHEN** tenant `acme` and tenant `globex` each upload files
- **THEN** every stored object key begins with its own tenant's prefix and no object is reachable under another tenant's prefix

#### Scenario: Storage call without tenant fails closed

- **WHEN** the file storage is invoked with a context carrying no tenant
- **THEN** it returns `ErrNoTenant` and writes no file
