# multitenancy

## Purpose

This specification describes the multitenancy capability, focusing on tenant isolation in storage and APIs.
## Requirements
### Requirement: Tenant identification on every request

Every API request SHALL carry a tenant identifier in the `X-Tenant-ID` header. Middleware SHALL validate the header (non-empty, at most 64 characters from `[A-Za-z0-9_-]`) and place the tenant identifier into the request `context.Context` before invoking any handler. A request with a missing or invalid header SHALL be rejected with `400 Bad Request` before any handler or repository code runs.

#### Scenario: Request without tenant header is rejected

- **WHEN** a client sends any API request without an `X-Tenant-ID` header
- **THEN** the response is `400 Bad Request` and no data is read or written

#### Scenario: Valid tenant header reaches the handler via context

- **WHEN** a client sends a request with header `X-Tenant-ID: acme`
- **THEN** downstream handlers, services, and repositories observe tenant `acme` from the request context

### Requirement: Tenant-scoped persistence

Every tenant-owned table SHALL carry a `tenant_id` column (storing the User ID) referencing the user registry (`tenants.id`). Every repository call SHALL pass the user ID explicitly via the `repo.Tenant(id)` option; when the option is absent, the repository SHALL fall back to the user carried in the passed `context.Context`. The explicit option SHALL take precedence over the context. All reads and writes SHALL be restricted to rows of the resolved user. A repository method invoked with neither an explicit user option nor a context user SHALL return the sentinel error `ErrNoTenant` and SHALL NOT issue any SQL statement.

#### Scenario: Repository writes stamp the context user

- **WHEN** a Document is created in a context carrying user `alice`
- **THEN** the persisted row has `tenant_id = 'alice'`

#### Scenario: Repository call without user fails closed

- **WHEN** any repository method is called with neither a user option nor a context user
- **THEN** it returns `ErrNoTenant` and performs no query

### Requirement: Cross-tenant isolation

No API response or repository result SHALL expose rows belonging to a different tenant. Identifiers (asset ids, serial numbers, brand+model pairs) are scoped per tenant: the same normalized serial number MAY exist once per tenant, and a lookup in one tenant SHALL NOT match rows of another tenant.

#### Scenario: Same serial in two tenants yields two assets

- **WHEN** tenant `acme` uploads a document extracting serial `SN1` and tenant `globex` uploads a different document extracting the same serial `SN1`
- **THEN** two distinct Assets exist, one per tenant, and each tenant's reads see only its own Asset

#### Scenario: Asset id of another tenant is not found

- **WHEN** tenant `acme` requests `GET /api/assets/{id}` for an Asset id created under tenant `globex`
- **THEN** the response is `404 Not Found`

### Requirement: Test tenant convention

Automated tests SHALL exercise all tenant-scoped behavior using explicit test tenant identifiers (e.g., `test-tenant`, and a second tenant for isolation tests). Tests SHALL NOT depend on a default or ambient tenant.

#### Scenario: Isolation test uses two explicit tenants

- **WHEN** the integration test suite runs cross-tenant isolation cases
- **THEN** each case seeds and reads data under explicit, distinct tenant ids

### Requirement: User-based tenant identification in URL

Supersedes "Tenant identification on every request": the `X-Tenant-ID` header SHALL be removed, and the `{userId}` URL parameter SHALL be the sole tenant identifier.

Every API request SHALL identify the active user (tenant) via the `{userId}` parameter in the URL path. Middleware SHALL extract this ID using the router's parameter resolution (e.g., `chi.URLParam`). Middleware SHALL validate the ID format (non-empty, at most 64 characters from `[A-Za-z0-9_-]`) and SHALL validate that it names a registered user (see "User registry"). A request with a missing or malformed `{userId}` SHALL be rejected with `400 Bad Request`; a request with a well-formed but unregistered user SHALL be rejected with `404 Not Found`. Registry database errors SHALL result in `500 Internal Server Error`. In all rejection cases, the rejection SHALL occur before any handler or repository code runs and no data SHALL be read or written. For accepted requests, middleware SHALL place the user ID into the request `context.Context`.

#### Scenario: Request with malformed userId is rejected with 400

- **WHEN** a client sends a request with a `{userId}` that is empty, longer than 64 characters, or contains characters outside `[A-Za-z0-9_-]`
- **THEN** the response is `400 Bad Request` and no data is read or written

#### Scenario: Unregistered user is rejected with 404

- **WHEN** a client sends a request with a well-formed `{userId}` naming a user that is not in the user registry
- **THEN** the response is `404 Not Found` and no data is read or written

#### Scenario: Valid registered user ID reaches the handler via context

- **WHEN** a client sends a request to `/api/users/alice/documents` and `alice` is a registered user
- **THEN** downstream handlers, services, and repositories observe user `alice` from the request context

### Requirement: User registry

The system SHALL maintain a user registry as a `tenants` table (naming retained for stability) holding one row per valid user ID in the `id` column. Every `tenant_id` value stored in a tenant-owned table (`sources`, `assets`, `documents`) SHALL reference a registry row. Users SHALL be created only by explicit provisioning.

#### Scenario: Tenant-owned rows require a registry row

- **WHEN** a row is inserted into `assets` with a `tenant_id` that does not exist in `tenants`
- **THEN** the insert fails with a foreign-key violation

### Requirement: Households and Membership

The system SHALL maintain `households` (id, tenant_id, display_name, created_at) and `household_members` (household_id, user_id) tables. A user_id in `household_members` SHALL reference the user registry. Membership is a many-to-many relationship: a user may belong to multiple households, and a household may have multiple users.

#### Scenario: User is member of multiple households

- **WHEN** user `alice` is added to `household_members` for household `h1` and `h2`
- **THEN** both memberships are recorded and active

### Requirement: Scoped Access

Tenant-owned rows (`sources`, `assets`, `documents`) SHALL carry a `scope` dimension. This SHALL be implemented as a `scope_type` column (enum: `personal`, `household`) and an optional `owner_household_id` column.
- Rows with `scope_type = 'personal'` are owned by the user identified by `tenant_id`.
- Rows with `scope_type = 'household'` are owned by the household identified by `owner_household_id`.
A user SHALL have read/write access to a row IF:
1. The row's `tenant_id` matches the user ID AND its `scope_type` is `personal`.
2. OR the row's `owner_household_id` identifies a household where the user is a member.

#### Scenario: User can read their personal document

- **WHEN** user `alice` requests a document where `tenant_id = 'alice'` and `scope_type = 'personal'`
- **THEN** the document is returned

#### Scenario: User can read household document

- **WHEN** user `alice` is a member of household `h1`, and requests a document where `owner_household_id = 'h1'` and `scope_type = 'household'`
- **THEN** the document is returned

#### Scenario: User cannot read another user's personal document

- **WHEN** user `alice` requests a document where `tenant_id = 'bob'` and `scope_type = 'personal'`, and `alice` != `bob`
- **THEN** the request is rejected or the document is not found

### Requirement: Row-level security defense-in-depth

Every tenant-owned table SHALL have PostgreSQL row-level security enabled. The policy SHALL restrict visibility and writes to rows whose `tenant_id` matches the user ID bound to the current transaction. Household-based scope filtering is enforced at the application level; RLS provides a hard boundary at the User/Tenant level.

#### Scenario: Unscoped query returns only the bound user's rows

- **WHEN** a transaction bound to user `alice` executes a raw `SELECT` on `assets` with no `WHERE tenant_id` clause
- **THEN** only rows with `tenant_id = 'alice'` are returned

### Requirement: User-scoped file storage

Stored file content SHALL be laid out under a per-user key prefix of the form `{userId}/{fileID}`. The storage layer SHALL resolve the user from the context.

#### Scenario: Uploaded file lands under the user prefix

- **WHEN** a document is uploaded in a context carrying user `alice`
- **THEN** the file bytes are stored under a key beginning with `alice/`

