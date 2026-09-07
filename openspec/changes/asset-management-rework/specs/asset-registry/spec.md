## MODIFIED Requirements

### Requirement: Canonical asset model

The system SHALL persist Assets in PostgreSQL, scoped per owner, with a **structured core** — unique opaque identifier, `owner_id`, **canonical product `name` (what the item is)**, brand, model, serial number, purchase date, warranty end, price, currency, **intrinsic asset category (`asset_category`) with `category_confidence`, and soft-delete marker (`deleted_at`, nullable)** — **plus a generic `metadata` JSONB object** holding arbitrary key-value pairs extracted from documents. All structured fields except the identifier, owner (`owner_id`), and timestamps SHALL be optional (nullable) so partially extracted documents can still establish an Asset. `metadata` SHALL default to an empty object and SHALL NOT be used for identity resolution. The document classification (`invoice`, `warranty`, `amc`, `receipt`, `statement`, `other`) SHALL NOT be stored on the Asset; it is a property of the Document record only.

(Previously: the structured core was "unique opaque identifier, `owner_id`, brand, model, serial number, purchase date, warranty end, price, currency, and document type (`invoice`, `warranty`, `amc`, or `other`)" with `document type` a mandatory field on the Asset — there was no `name`, no `asset_category`, no `category_confidence`, and no `deleted_at`.)

#### Scenario: Asset persists the full extracted field set

- **WHEN** an Asset is created from a complete extraction
- **THEN** reading the Asset back returns the same canonical name, brand, model, serial number, purchase date, price, currency, warranty end, and an intrinsic `asset_category` with `category_confidence`

#### Scenario: Asset supports partially known data

- **WHEN** an extraction yields only brand and model
- **THEN** an Asset can be created with category and all other fields unset

#### Scenario: No document-type field on the asset

- **WHEN** an Asset is created from an invoice document
- **THEN** the Asset carries an `asset_category` describing the item itself
- **AND** no Asset field holds the value `invoice` as a type of the asset

### Requirement: List assets endpoint

The system SHALL expose `GET /api/users/{userId}/assets` returning `200 OK` with a JSON array of all non-deleted Assets in a stable, deterministic order. An empty registry SHALL return an empty array, not an error. Soft-deleted assets SHALL be excluded by default and SHALL only be returned when an explicit `include_deleted` query parameter is set.

(Previously: "returning `200 OK` with a JSON array of all Assets in a stable, deterministic order" — there was no soft-delete and therefore no exclusion or `include_deleted` parameter.)

#### Scenario: Lists all assets

- **WHEN** two Assets exist and a client requests `GET /api/users/{userId}/assets`
- **THEN** the response is `200 OK` with a JSON array containing both Assets

#### Scenario: Empty registry returns an empty array

- **WHEN** no Assets exist and a client requests `GET /api/users/{userId}/assets`
- **THEN** the response is `200 OK` with body `[]`

#### Scenario: Soft-deleted assets are hidden

- **WHEN** an Asset has been soft-deleted and a client requests `GET /api/users/{userId}/assets`
- **THEN** the deleted Asset is absent from the response array

### Requirement: Get asset endpoint

The system SHALL expose `GET /api/users/{userId}/assets/{assetId}` returning `200 OK` with the Asset JSON for an existing, non-deleted identifier, and `404 Not Found` for an unknown or soft-deleted identifier.

(Previously: "returning `200 OK` with the Asset JSON for an existing identifier, and `404 Not Found` for an unknown identifier" — soft-deletion did not exist.)

#### Scenario: Existing asset is returned

- **WHEN** a client requests `GET /api/users/{userId}/assets/{assetId}` for an existing Asset
- **THEN** the response is `200 OK` with that Asset's JSON representation

#### Scenario: Unknown asset returns 404

- **WHEN** a client requests `GET /api/users/{userId}/assets/{assetId}` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

#### Scenario: Soft-deleted asset returns 404

- **WHEN** a client requests `GET /api/users/{userId}/assets/{assetId}` for a soft-deleted Asset
- **THEN** the response is `404 Not Found`

### Requirement: Asset documents endpoint

The system SHALL expose `GET /api/users/{userId}/assets/{assetId}/documents` returning `200 OK` with a JSON array of the Asset's Documents. Each entry SHALL include the document identifier, document classification (`invoice`, `receipt`, `warranty`, `amc`, `statement`, or `other`), classified/extracted fields, and the originating Source's filename and upload timestamp. Documents accumulated through merges SHALL all appear. Requesting documents for an unknown Asset identifier SHALL return `404 Not Found`.

(Previously: "Each entry SHALL include the document identifier, document type, classified/extracted fields, the originating Source's filename and upload timestamp." — the classification vocabulary was effectively `invoice`/`warranty`/`other`, and merges did not exist.)

#### Scenario: Documents are listed in ingestion order

- **WHEN** an Asset has an invoice Document and a later warranty Document, and a client requests `GET /api/users/{userId}/assets/{assetId}/documents`
- **THEN** the response is `200 OK` with both entries, each showing its classification, source filename, and upload timestamp

#### Scenario: Merged asset shows all source documents

- **WHEN** duplicate asset A2 (with its own documents) has been merged into A1
- **THEN** `GET /api/users/{userId}/assets/{A1}/documents` returns the documents formerly linked to A2 as well as A1's own

#### Scenario: Unknown asset returns 404

- **WHEN** a client requests `GET /api/users/{userId}/assets/{assetId}/documents` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

## ADDED Requirements (cycle 2)

### Requirement: Edit asset data endpoint

The system SHALL expose editing of an asset's structured data fields across the
canonical core — canonical name, brand, model, serial number, purchase date, warranty
start/end, price, currency, and asset category — not only the category. Editing SHALL be
owner-scoped (cross-owner → 404) and SHALL NOT apply to soft-deleted assets (404).
User-edited fields SHALL be recorded as user-set: a user-set category retains the
existing sticky semantics (not overwritten by later extraction/merge), and fields the
user explicitly sets SHALL be preserved rather than silently re-inferred by reprocessing
of linked documents.

#### Scenario: Edit name and price

- **WHEN** the user edits asset A setting canonical name "Microwave Oven" and price 4999
- **THEN** A persistently carries the new values and subsequent reads return them

#### Scenario: Edited field survives a document reprocess

- **GIVEN** asset A whose name the user edited to "Microwave Oven"
- **WHEN** one of A's documents is reprocessed and the inference yields name "Microwave"
- **THEN** the reprocess does not overwrite the user-set name

#### Scenario: Edit is owner-scoped

- **WHEN** user X attempts to edit an asset owned by user Y (no shared household)
- **THEN** the response is 404 and the asset is unchanged
