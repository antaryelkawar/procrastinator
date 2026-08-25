# Spec: asset-identity-resolution

Delta for change `invoice-warranty-asset-flow`. Modifies the archived baseline in `openspec/specs/asset-identity-resolution`.

## MODIFIED Requirements

### Requirement: Serial-number match

When an extraction yields a serial number, the system SHALL match it against existing Assets **of the same tenant** by normalized serial number. A match identifies the same Asset. Serial-number matching SHALL take precedence over brand+model matching whenever a serial number is present. Serial numbers of other tenants SHALL NOT be considered.

#### Scenario: Serial match links document to existing asset

- **WHEN** an existing Asset of tenant `acme` has serial number `SN123` and a newly uploaded warranty for tenant `acme` extracts serial number `SN123`
- **THEN** the new Document is linked to the existing Asset and no new Asset is created

#### Scenario: Serial match wins over conflicting product identity

- **WHEN** an extraction yields serial number matching Asset A but a brand+model matching Asset B (same tenant)
- **THEN** the extraction is resolved to Asset A

#### Scenario: Serial of another tenant does not match

- **WHEN** tenant `globex` has an Asset with serial `SN123` and tenant `acme` uploads a document extracting serial `SN123`
- **THEN** a new Asset is created for tenant `acme`

### Requirement: Brand and model match

When an extraction yields no serial number but yields both brand and model, the system SHALL match against existing Assets **of the same tenant** on normalized brand AND normalized model. A match identifies the same Asset.

#### Scenario: Brand+model match links document to existing asset

- **WHEN** an existing Asset of tenant `acme` has brand `Samsung` and model `WW90T534DAW`, and an extraction for tenant `acme` with no serial number yields brand `samsung` and model ` WW90T534DAW `
- **THEN** the extraction is matched to that Asset

### Requirement: Field merge on update

When an extraction resolves to an existing Asset, non-empty extracted structured fields SHALL update the Asset's corresponding fields, and absent/null extracted fields SHALL NOT erase existing Asset values. The Asset's `metadata` SHALL be shallow-merged per key: keys present in the new extraction's metadata overwrite existing values for those keys, and keys absent from the new extraction SHALL be retained. The Asset's `doc_type` SHALL be set to the classification of the most recently ingested Document (last write wins).

#### Scenario: Warranty end added to an existing asset

- **WHEN** an Asset created from an invoice has no warranty end, and a later warranty document for the same serial number extracts `warranty_end`
- **THEN** the Asset's warranty end is set and its previously populated fields remain unchanged

#### Scenario: Metadata merges without erasing

- **WHEN** an Asset has metadata `{"amc_card_number": "C1"}` and a later matched extraction contributes metadata `{"icr_number": "I9"}`
- **THEN** the Asset's metadata is `{"amc_card_number": "C1", "icr_number": "I9"}`

#### Scenario: Newer non-empty values win

- **WHEN** an existing Asset has model `WW90` and a later matched extraction yields model `WW90T534DAW`
- **THEN** the Asset's model is updated to `WW90T534DAW`

#### Scenario: Latest document type wins

- **WHEN** an Asset created from an `invoice` document later matches an `amc` document
- **THEN** the Asset's `doc_type` is `amc`
