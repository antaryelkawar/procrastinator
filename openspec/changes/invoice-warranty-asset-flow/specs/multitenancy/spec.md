# Spec: multitenancy

Delta for change `invoice-warranty-asset-flow`. All requirements below are ADDED.

## ADDED Requirements

### Requirement: Tenant identification on every request

Every API request SHALL carry a tenant identifier in the `X-Tenant-ID` header. Middleware SHALL validate the header (non-empty, at most 64 characters from `[A-Za-z0-9_-]`) and place the tenant identifier into the request `context.Context` before invoking any handler. A request with a missing or invalid header SHALL be rejected with `400 Bad Request` before any handler or repository code runs.

#### Scenario: Request without tenant header is rejected

- **WHEN** a client sends any API request without an `X-Tenant-ID` header
- **THEN** the response is `400 Bad Request` and no data is read or written

#### Scenario: Valid tenant header reaches the handler via context

- **WHEN** a client sends a request with header `X-Tenant-ID: acme`
- **THEN** downstream handlers, services, and repositories observe tenant `acme` from the request context

### Requirement: Tenant-scoped persistence

Every table (`sources`, `assets`, `documents`) SHALL carry a `tenant_id` column. Every repository method SHALL read the tenant from the passed `context.Context` and SHALL restrict all reads and writes to rows of that tenant. A repository method invoked with a context that carries no tenant SHALL return an error rather than operate across tenants.

#### Scenario: Repository writes stamp the context tenant

- **WHEN** an Asset is created in a context carrying tenant `acme`
- **THEN** the persisted row has `tenant_id = 'acme'`

#### Scenario: Repository call without tenant fails closed

- **WHEN** any repository method is called with a context that has no tenant
- **THEN** it returns an error and performs no query

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
