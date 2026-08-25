# Spec: asset-registry

Delta for change `invoice-warranty-asset-flow`. Modifies the archived baseline in `openspec/specs/asset-registry`.

## MODIFIED Requirements

### Requirement: Canonical asset model

The system SHALL persist Assets in PostgreSQL, scoped per tenant, with a **structured core** — unique opaque identifier, `tenant_id`, brand, model, serial number, purchase date, warranty end, price, currency, and document type (`invoice`, `warranty`, `amc`, or `other`) — **plus a generic `metadata` JSONB object** holding arbitrary key-value pairs extracted from documents. All structured fields except the identifier, tenant, document type, and timestamps SHALL be optional (nullable) so partially extracted documents can still establish an Asset. `metadata` SHALL default to an empty object and SHALL NOT be used for identity resolution.

#### Scenario: Asset persists the full structured core plus metadata

- **WHEN** an Asset is created from a complete AMC extraction including metadata `{"amc_card_number": "LG2401", "cgst_rate": "9"}`
- **THEN** reading the Asset back returns the same brand, model, serial number, purchase date, warranty end, price, currency, document type `amc`, and the same metadata entries

#### Scenario: Asset supports partially known data

- **WHEN** an extraction yields only brand and model
- **THEN** an Asset can be created with all other structured fields unset and an empty metadata object

### Requirement: Serial number uniqueness

When present, an Asset's normalized serial number SHALL be unique across all Assets **of the same tenant**. Concurrent uploads within one tenant that resolve the same new serial number SHALL result in a single Asset, never duplicates. The same normalized serial number MAY exist in different tenants.

#### Scenario: Duplicate serial does not create a second asset

- **WHEN** two uploads for the same tenant extract the same normalized serial number and no Asset with that serial existed in that tenant before either upload
- **THEN** exactly one Asset with that serial number exists in that tenant after both uploads complete

### Requirement: List assets endpoint

The system SHALL expose `GET /api/assets` returning `200 OK` with a JSON array of all Assets **of the requesting tenant** in a stable, deterministic order. An empty registry SHALL return an empty array, not an error.

#### Scenario: Lists only the requesting tenant's assets

- **WHEN** tenant `acme` has two Assets, tenant `globex` has one, and a client requests `GET /api/assets` with `X-Tenant-ID: acme`
- **THEN** the response is `200 OK` with a JSON array containing exactly the two `acme` Assets

#### Scenario: Empty registry returns an empty array

- **WHEN** no Assets exist for the requesting tenant and a client requests `GET /api/assets`
- **THEN** the response is `200 OK` with body `[]`

### Requirement: Get asset endpoint

The system SHALL expose `GET /api/assets/{id}` returning `200 OK` with the Asset JSON (including `doc_type` and `metadata`) for an existing identifier **owned by the requesting tenant**, and `404 Not Found` for an unknown identifier or an identifier owned by another tenant.

#### Scenario: Existing asset is returned

- **WHEN** a client requests `GET /api/assets/{id}` for an existing Asset of their tenant
- **THEN** the response is `200 OK` with that Asset's JSON representation including `doc_type` and `metadata`

#### Scenario: Unknown or foreign asset returns 404

- **WHEN** a client requests `GET /api/assets/{id}` for an identifier that does not exist in their tenant
- **THEN** the response is `404 Not Found`

### Requirement: Asset documents endpoint

The system SHALL expose `GET /api/assets/{id}/documents` returning `200 OK` with a JSON array of the Asset's Documents **for the requesting tenant**. Each entry SHALL include the document identifier, document type (`invoice`, `warranty`, `amc`, or `other`), classified/extracted fields including metadata, and the originating Source's filename and upload timestamp. Requesting documents for an unknown or foreign Asset identifier SHALL return `404 Not Found`.

#### Scenario: Documents are listed in ingestion order

- **WHEN** an Asset has an invoice Document and a later AMC Document, and a client requests `GET /api/assets/{id}/documents` for that Asset's tenant
- **THEN** the response is `200 OK` with both entries, each showing its type, source filename, and upload timestamp

#### Scenario: Unknown asset returns 404

- **WHEN** a client requests `GET /api/assets/{id}/documents` for an identifier that does not exist in their tenant
- **THEN** the response is `404 Not Found`
