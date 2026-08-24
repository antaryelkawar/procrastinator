# Spec: asset-registry

Delta for change `invoice-warranty-asset-flow`. All requirements below are ADDED.

## ADDED Requirements

### Requirement: Canonical asset model

The system SHALL persist Assets in PostgreSQL with the fields: unique opaque identifier, brand, model, serial number, purchase date, price, currency, warranty start, warranty end, and record timestamps. All fields except the identifier and timestamps SHALL be optional (nullable) so partially extracted documents can still establish an Asset.

#### Scenario: Asset persists the full extracted field set

- **WHEN** an Asset is created from a complete extraction
- **THEN** reading the Asset back returns the same brand, model, serial number, purchase date, price, currency, warranty start, and warranty end

#### Scenario: Asset supports partially known data

- **WHEN** an extraction yields only brand and model
- **THEN** an Asset can be created with all other fields unset

### Requirement: Serial number uniqueness

When present, an Asset's normalized serial number SHALL be unique across all Assets. Concurrent uploads that resolve the same new serial number SHALL result in a single Asset, never duplicates.

#### Scenario: Duplicate serial does not create a second asset

- **WHEN** two uploads extract the same normalized serial number and no Asset with that serial existed before either upload
- **THEN** exactly one Asset with that serial number exists after both uploads complete

### Requirement: Money representation

Asset price SHALL be stored and transmitted as an exact decimal value with an ISO 4217 currency code. Binary floating-point SHALL NOT be used for monetary values.

#### Scenario: Price round-trips exactly

- **WHEN** an Asset is created with price `39999.99` and currency `INR`
- **THEN** reading the Asset returns price `39999.99` and currency `INR` without floating-point drift

### Requirement: List assets endpoint

The system SHALL expose `GET /api/assets` returning `200 OK` with a JSON array of all Assets in a stable, deterministic order. An empty registry SHALL return an empty array, not an error.

#### Scenario: Lists all assets

- **WHEN** two Assets exist and a client requests `GET /api/assets`
- **THEN** the response is `200 OK` with a JSON array containing both Assets

#### Scenario: Empty registry returns an empty array

- **WHEN** no Assets exist and a client requests `GET /api/assets`
- **THEN** the response is `200 OK` with body `[]`

### Requirement: Get asset endpoint

The system SHALL expose `GET /api/assets/{id}` returning `200 OK` with the Asset JSON for an existing identifier, and `404 Not Found` for an unknown identifier.

#### Scenario: Existing asset is returned

- **WHEN** a client requests `GET /api/assets/{id}` for an existing Asset
- **THEN** the response is `200 OK` with that Asset's JSON representation

#### Scenario: Unknown asset returns 404

- **WHEN** a client requests `GET /api/assets/{id}` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

### Requirement: Asset documents endpoint

The system SHALL expose `GET /api/assets/{id}/documents` returning `200 OK` with a JSON array of the Asset's Documents. Each entry SHALL include the document identifier, document type, classified/extracted fields, the originating Source's filename and upload timestamp. Requesting documents for an unknown Asset identifier SHALL return `404 Not Found`.

#### Scenario: Documents are listed in ingestion order

- **WHEN** an Asset has an invoice Document and a later warranty Document, and a client requests `GET /api/assets/{id}/documents`
- **THEN** the response is `200 OK` with both entries, each showing its type, source filename, and upload timestamp

#### Scenario: Unknown asset returns 404

- **WHEN** a client requests `GET /api/assets/{id}/documents` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

### Requirement: Versioned schema migrations

The PostgreSQL schema for Source, Asset, and Document SHALL be managed exclusively through versioned goose migrations. The application SHALL apply pending migrations at startup and SHALL refuse to serve requests if the schema cannot be brought to the latest version.

#### Scenario: Fresh database is migrated at startup

- **WHEN** the application starts against an empty PostgreSQL database
- **THEN** the Source, Asset, and Document tables exist and the application serves requests

#### Scenario: Unreachable database prevents serving

- **WHEN** the application starts against an unreachable PostgreSQL database
- **THEN** startup fails and no requests are served
