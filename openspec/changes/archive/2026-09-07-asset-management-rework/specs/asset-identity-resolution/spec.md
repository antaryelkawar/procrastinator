## MODIFIED Requirements

### Requirement: Field merge on update

When an extraction resolves to an existing Asset, non-empty extracted fields SHALL update the Asset's corresponding fields, and absent/null extracted fields SHALL NOT erase existing Asset values. The Asset's `metadata` SHALL be shallow-merged per key: keys present in the new extraction's metadata overwrite existing values for those keys, and keys absent from the new extraction SHALL be retained. A user-corrected `asset_category` SHALL NOT be overwritten by subsequent extractions; otherwise an inferred category MAY be replaced by a higher-confidence inference. The Asset SHALL NOT carry a document-type field updated by ingestion — document classification lives only on the Document record.

(Previously: "The Asset's `doc_type` SHALL be set to the classification of the most recently ingested Document (last write wins)." — that sentence is removed; assets no longer carry a document type at all.)

#### Scenario: Warranty dates added to an existing asset

- **WHEN** an Asset created from an invoice has no warranty dates, and a later warranty document for the same serial number extracts `warranty_start` and `warranty_end`
- **THEN** the Asset's warranty dates are set and its previously populated fields remain unchanged

#### Scenario: Empty extracted fields never erase existing values

- **WHEN** an existing Asset has a purchase price and a later matched extraction has a null price
- **THEN** the Asset retains its original purchase price

#### Scenario: Newer non-empty values win

- **WHEN** an existing Asset has model `WW90` and a later matched extraction yields model `WW90T534DAW`
- **THEN** the Asset's model is updated to `WW90T534DAW`

#### Scenario: User-set category is sticky

- **WHEN** a user has corrected Asset A's category to `electronics` and a later matched extraction infers `other`
- **THEN** A's category remains `electronics`

## ADDED Requirements

### Requirement: Re-upload dedupe at identity layer

When an incoming extraction resolves to an existing Asset and the originating Source's content hash matches a Document already linked to that Asset, the system SHALL NOT modify any Asset field and SHALL NOT create a second Document; it SHALL report the existing Document as a duplicate.

#### Scenario: Same invoice bytes re-uploaded

- **GIVEN** Asset A has Document D from Source with hash H
- **WHEN** a new upload with identical bytes (hash H) resolves to A
- **THEN** no new Document is created, no Asset field changes, and the outcome references D as a duplicate

### Requirement: Soft-deleted assets are excluded from matching

Identity resolution SHALL NOT match incoming extractions against soft-deleted Assets. If the only match is a soft-deleted Asset, the system SHALL surface the deleted match to the caller (so the UI can offer restore) instead of silently creating a duplicate Asset.

#### Scenario: Upload matching a deleted asset

- **GIVEN** Asset A with serial `SN123` is soft-deleted
- **WHEN** a new upload extracts serial `SN123`
- **THEN** the system reports the deleted match rather than creating a new Asset
- **AND** the caller is offered the option to restore A

### Requirement: Efficient lookup order

Identity resolution SHALL attempt matches in a deterministic order — (1) normalized serial number, (2) normalized brand+model, (3) normalized name+model — within the same owner scope, returning a bounded candidate set per stage (default 10). The first definitive match SHALL short-circuit the remaining stages. Lookup latency SHALL be independent of registry size (no full-registry scans; the data structures achieving this are a design concern).

#### Scenario: Serial match short-circuits later stages

- **WHEN** an extraction matches an existing asset on normalized serial
- **THEN** the brand+model and name+model stages are not executed

#### Scenario: Lookup time is flat as the registry grows

- **GIVEN** owner X has 100 assets and owner Y has 10,000 assets
- **WHEN** identity resolution runs the same extraction for each owner
- **THEN** both lookups complete within the same order of magnitude (Y's is less than 5× X's), demonstrating no full-registry scan
