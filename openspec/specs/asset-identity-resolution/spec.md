# asset-identity-resolution

## Purpose

Standardize and implement the logic for normalizing asset identity data (serial numbers, brands, models) and resolving extracted document data to existing or new assets to ensure data consistency and prevent duplication.

## Requirements

### Requirement: Identity normalization

Before matching, identity values SHALL be normalized: leading/trailing whitespace trimmed, internal whitespace runs collapsed to single spaces, and case folded (uppercase for serial numbers; case-insensitive comparison for brand and model). Normalization SHALL be deterministic and applied identically to incoming extractions and stored Asset values.

#### Scenario: Serial numbers match across formatting differences

- **WHEN** an existing Asset has serial number `SN-123 abc` and an extraction yields serial number `  sn-123  ABC `
- **THEN** the extraction is matched to that Asset

### Requirement: Serial-number match

When an extraction yields a serial number, the system SHALL match it against existing Assets by normalized serial number. A match identifies the same Asset. Serial-number matching SHALL take precedence over brand+model matching whenever a serial number is present.

#### Scenario: Serial match links document to existing asset

- **WHEN** an existing Asset has serial number `SN123` and a newly uploaded warranty extracts serial number `SN123`
- **THEN** the new Document is linked to the existing Asset and no new Asset is created

#### Scenario: Serial match wins over conflicting product identity

- **WHEN** an extraction yields serial number matching Asset A but a brand+model matching Asset B
- **THEN** the extraction is resolved to Asset A

### Requirement: Brand and model match

When an extraction yields no serial number but yields both brand and model, the system SHALL match against existing Assets on normalized brand AND normalized model. A match identifies the same Asset.

#### Scenario: Brand+model match links document to existing asset

- **WHEN** an existing Asset has brand `Samsung` and model `WW90T534DAW`, and an extraction with no serial number yields brand `samsung` and model ` WW90T534DAW `
- **THEN** the extraction is matched to that Asset

#### Scenario: Brand alone is insufficient

- **WHEN** an extraction yields brand `Samsung` with no model and no serial number
- **THEN** no brand+model match is attempted

### Requirement: Create on no match

When an extraction yields a usable identity (a serial number, or brand+model) and no existing Asset matches, the system SHALL create a new Asset populated with all validated extracted fields.

#### Scenario: First invoice creates a new asset

- **WHEN** an invoice is uploaded and no existing Asset matches its extracted identity
- **THEN** a new Asset is created with the extracted brand, model, serial number, purchase date, price, currency, and warranty dates populated where present

### Requirement: Field merge on update

When an extraction resolves to an existing Asset, non-empty extracted fields SHALL update the Asset's corresponding fields, and absent/null extracted fields SHALL NOT erase existing Asset values.

#### Scenario: Warranty dates added to an existing asset

- **WHEN** an Asset created from an invoice has no warranty dates, and a later warranty document for the same serial number extracts `warranty_start` and `warranty_end`
- **THEN** the Asset's warranty dates are set and its previously populated fields remain unchanged

#### Scenario: Empty extracted fields never erase existing values

- **WHEN** an existing Asset has a purchase price and a later matched extraction has a null price
- **THEN** the Asset retains its original purchase price

#### Scenario: Newer non-empty values win

- **WHEN** an existing Asset has model `WW90` and a later matched extraction yields model `WW90T534DAW`
- **THEN** the Asset's model is updated to `WW90T534DAW`

### Requirement: No identity, no asset

When an extraction yields neither a serial number nor a brand+model pair and therefore no match is possible, the system SHALL NOT create an Asset and SHALL NOT link the Document to any Asset.

#### Scenario: Unidentifiable extraction creates nothing

- **WHEN** an extraction yields only a document type with no serial number, brand, or model
- **THEN** no Asset is created or modified and the upload is reported as unprocessable
