## ADDED Requirements

### Requirement: Asset lifecycle endpoints in the contract

The OpenAPI contract SHALL define: `DELETE /api/users/{userId}/assets/{assetId}` (soft-delete), `POST /api/users/{userId}/assets/{assetId}/restore`, `POST /api/users/{userId}/assets/{assetId}/merge` (body: `duplicate_asset_id`), and `PATCH /api/users/{userId}/assets/{assetId}` (user corrections, at minimum `asset_category`). All SHALL be represented in generated server types and the client SDK like every other operation.

#### Scenario: Delete and merge operations are generated

- **WHEN** codegen runs against the updated contract
- **THEN** typed client functions exist for delete, restore, merge, and patch asset operations

### Requirement: Unified add endpoint in the contract

The OpenAPI contract SHALL define `POST /api/users/{userId}/add` accepting multipart file(s) or a text body, returning a uniform per-item outcome (`asset_committed`, `held_for_review`, `duplicate`, `statement_preview`, `failed`) with the relevant references. The existing `POST /documents` endpoint remains for compatibility and delegates to the same pipeline.

#### Scenario: Add endpoint returns uniform outcomes

- **WHEN** a client POSTs one receipt image and one statement CSV to `/add`
- **THEN** the response contains one outcome per item with the kinds `asset_committed` (or `held_for_review`) and `statement_preview` respectively

### Requirement: Enhanced search endpoint in the contract

The OpenAPI contract SHALL extend search to accept structured filters (category, brand, purchase-date range, warranty status, has-documents, document classification) combinable with free text, and SHALL return hits labeled by entity kind (asset, document, movement) with pagination.

#### Scenario: Filtered search request is typed

- **WHEN** a client requests search with `q=microwave&category=appliance&warranty_status=active`
- **THEN** the request binds through generated types and the response contains hits labeled by kind

### Requirement: Statement import endpoints remain ledger-scoped

The statement import-batch endpoints (`/finance/import-batches*`) SHALL remain in the contract unchanged in behavior for the ledger; the unified add endpoint routes statement files into the same import-batch pipeline rather than duplicating it.

#### Scenario: Statement added via /add creates an import batch

- **WHEN** a statement CSV is submitted through `/add`
- **THEN** a standard import batch (preview state) is created and referenced in the outcome
- **AND** the existing `/finance/import-batches` endpoints can list, commit, and discard it

## ADDED Requirements (cycle 2)

### Requirement: Documents section endpoints in the contract

The OpenAPI contract SHALL define a documents listing for the Documents section — a
per-owner list of ingested sources joined with their document/asset status — accepting
status filters and free text, returning items with: identifier, filename/label, ingest
timestamp, processing status (`processed`/`in_review`/`failed`/`asset_less`), and linked
asset reference when present; plus `DELETE /api/users/{userId}/documents/{documentId}`
(soft-delete + restore within retention), and
`POST /api/users/{userId}/documents/{documentId}/reprocess` (optional comment,
optional accompanying text) returning the standard outcome. All SHALL be represented in
generated server types and the client SDK like every other operation.

#### Scenario: Documents list operation is generated

- **WHEN** codegen runs against the updated contract
- **THEN** typed client functions exist for documents listing (with status filters),
  document delete/restore, and document reprocess

#### Scenario: Status join is visible in the response

- **GIVEN** a source that failed at processing (no document row) and a processed source
- **WHEN** a client requests the documents list
- **THEN** both items appear with statuses `failed` and `processed` respectively, and the
  processed item references its linked asset

### Requirement: Asset reprocess and extended edit endpoints in the contract

The OpenAPI contract SHALL extend the asset surface with
`POST /api/users/{userId}/assets/{assetId}/reprocess` (re-runs extraction over the
asset's linked documents; returns a per-document outcome summary) and SHALL extend the
asset edit request payload to cover the full structured core (name, brand, model,
serial number, purchase date, warranty start/end, price, currency, category), marking
edited fields as user-set where applicable. All SHALL be represented in generated server
types and the client SDK.

#### Scenario: Reprocess and edit operations are generated

- **WHEN** codegen runs against the updated contract
- **THEN** typed client functions exist for asset reprocess and for the extended asset edit payload

#### Scenario: Edit payload covers the structured core

- **WHEN** a client PATCHes an asset setting name, serial, purchase date, and price
- **THEN** the request binds through generated types and the asset returns the new values
