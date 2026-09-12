# jsonb-entity-storage — Delta

## ADDED Requirements

### Requirement: Uniform jsonb entity shape

Every persisted entity SHALL have one uniform physical shape in PostgreSQL:

1. **Key columns** — primary key, foreign keys required for referential integrity (e.g. `documents.asset_id`, `documents.source_id`), RLS scoping columns (`owner_id`, `owner_household_id`), and lifecycle columns that constraints depend on (uniqueness/deletion semantics, e.g. `deleted_at`, `merged_into` where uniqueness or FK behavior needs them).
2. **Audit metadata** — `created_at` and `updated_at` (with updated touched on writes).
3. **Payload column** — a single `jsonb` column (non-null, default `'{}'`) holding everything else for that entity.

Entities covered: assets, documents, sources, ingest_reviews, accounts, movements, import batches, users, households. No entity's queryable data (besides keys/audit as above) SHALL live outside the payload column. Migrations for this change SHALL start from scratch: the existing migration chain (00001–00006) is replaced by a fresh schema, because the database is WIP and contains no data to preserve.

#### Scenario: Asset row uses the uniform shape

- **WHEN** an asset is created and its row inspected in Postgres
- **THEN** the row consists of `id`, `owner_id`, `owner_household_id`, `created_at`, `updated_at` and a `payload` jsonb column containing all asset data (name, brand, model, serial, purchase info, category, confidence, lifecycle state)

#### Scenario: Document row references its asset by key column

- **WHEN** a document extracted from a source is persisted
- **THEN** `documents.asset_id` exists as a real FK column (`ON DELETE SET NULL`), while the document's doc type, status, and extracted fields live in `documents.payload`

#### Scenario: Fresh migration chain

- **WHEN** a new database is provisioned and migrations run from scratch
- **THEN** every entity table exists in the uniform shape with no legacies of the 00001–00006 column layout

### Requirement: Lookup fields live in the payload with jsonb indexing

Normalized identity-resolution fields (`norm_name`, `norm_brand`, `norm_model`, `norm_serial`, etc. — per-entity field lists follow the existing domain vocabularies in `commons/entity`) SHALL be stored inside the entity payload as `data.*` paths. The three-stage identity-resolution lookup and other equality lookups SHALL query these paths and SHALL be backed by Postgres jsonb indexes (GIN on the payload with suitable operator classes, or single-expression `CREATE INDEX ... ON t ((payload #>> '{data,norm_brand}'))` — implementation choice documented at design time). Lookup behavior SHALL be behaviorally identical to today's stages (exact serial → brand+model → name+model, owner/household scoped).

#### Scenario: Stage-1 serial lookup is index-backed

- **WHEN** identity resolution performs an exact normalized-serial match for an owner
- **THEN** the query resolves via a jsonb expression/GIN index on the assets payload (confirmed by the query's plan during test) and returns the same matching asset that a column-based lookup returned before this change

#### Scenario: Identity resolution regression suite passes on jsonb storage

- **WHEN** the existing identity-resolution test suite (resolve_test.go cases: exact serial, brand+model, name+model, household vs personal scope) runs against the jsonb schema
- **THEN** all cases pass without behavioral changes

### Requirement: Company/repo layer serializes payload

The repository layer SHALL marshal each domain entity's data portion to/from the payload jsonb column transparently, so domain services and handlers keep using the typed Go structs unchanged.

#### Scenario: Domain logic is untouched by storage change

- **WHEN** `core/processing`, `core/identity`, `core/lifecycle`, `core/search` services run
- **THEN** they receive typed entities as before; only the repo/infra layer knows about the payload column

### Requirement: Registry generic repositories use the uniform shape

The generic repository machinery (kind registry, RLS scoping, soft-delete filters) SHALL be driven by per-entity key-column metadata and reuse the same payload read/write path for every kind.

#### Scenario: Adding a column-free field needs no migration

- **WHEN** a new optional field is added to an entity's extraction vocabulary
- **THEN** no database migration is required — it round-trips through the payload

#### Scenario: Scoping and soft-delete still enforced by key-column metadata

- **WHEN** the existing RLS/tenancy suite (`infra/postgres/rls_test.go`, `e2e/tenancy_e2e_test.go`) and soft-delete filter tests run against the jsonb schema
- **THEN** owner/household scoping and `deleted_at IS NULL` filtering still hold, driven by per-entity key-column metadata — no entity payload data participates in scoping

### Requirement: API mirrors entity structure

API resource schemas (OpenAPI models, hence Go gen types and UI generated clients) SHALL mirror the entity structure: identity/id, the entity's own payload fields as a named nested structure, and audit fields. Responses SHALL NOT flatten payload fields into an ad-hoc column list; every entity's representation has the same canonical shape: `{ id, ..., data: { ... }, createdAt, updatedAt }`. API mirrors entity structure rather than relational columns.

#### Scenario: Asset response exposes data payload nested

- **WHEN** the UI fetches an asset over the API
- **THEN** the response contains `id`, `data` (with name/brand/model/serial/category/… inside), and `createdAt`/`updatedAt`, matching the entity's storage structure one-to-one

#### Scenario: Codegen lockstep holds for the new shape

- **WHEN** the OpenAPI contract is re-modeled and codegen runs for Go and UI
- **THEN** generated types compile and the existing drift/lockstep tests pass for the entity-shaped schemas
