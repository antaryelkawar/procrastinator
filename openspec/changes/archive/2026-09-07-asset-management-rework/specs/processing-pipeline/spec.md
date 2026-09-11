## ADDED Requirements

### Requirement: Parallel multi-model extraction
The processing module SHALL run extraction of each accepted document through at least two independent extraction workers (which may use different models or prompting strategies) concurrently, and SHALL combine their outputs through a consensus step before any canonical write. The consensus step SHALL produce a numeric consensus confidence in [0.0, 1.0]: agreement of all workers on the identity fields yields a confidence ≥ 0.8 (high); exactly one successful worker caps confidence at 0.6; disagreement between workers on an identity field caps confidence at 0.5 and marks that field unresolved. A document whose consensus confidence is below the auto-commit threshold (default 0.7, configurable) SHALL be held for review rather than auto-committed. Per-worker failures SHALL not abort the document: a single successful worker is sufficient to proceed at the capped confidence. If ALL workers fail, the outcome SHALL be `failed(reason)` with the per-worker errors recorded, and the Source SHALL be retained.

#### Scenario: Two workers agree on identity fields
- **GIVEN** a document processed by workers W1 and W2
- **WHEN** both extract brand "Cooler Master", model "CD600", and matching serial
- **THEN** the consensus output carries those fields with confidence ≥ 0.8 and the document is eligible for auto-commit

#### Scenario: Workers disagree on a field
- **GIVEN** W1 extracts serial "SN123" and W2 extracts serial "SN128"
- **WHEN** consensus runs
- **THEN** the serial field is marked unresolved, consensus confidence is capped at 0.5, and the document is held for review rather than auto-committing a guessed value

#### Scenario: One worker fails
- **GIVEN** W2 times out while W1 completes successfully
- **WHEN** the document is processed
- **THEN** processing continues with W1's output, consensus confidence is capped at 0.6, and the failure is recorded in the document's provenance metadata

#### Scenario: All workers fail
- **GIVEN** every configured extraction worker fails for a document
- **WHEN** processing completes
- **THEN** the outcome is `failed` with the per-worker errors recorded
- **AND** the Source is retained and no Asset or Document is created or modified

### Requirement: Brand and model recognition from document text
The extraction pipeline SHALL recognize brand names and model identifiers that appear anywhere in the document content, including within product description strings (e.g. "CABINET COOLER MASTER CD600 BLACK" ⇒ brand "Cooler Master", model "CD600"). A configurable known-brand lexicon SHALL be consulted during consensus to validate or correct extracted brands.

#### Scenario: Brand embedded in description line
- **WHEN** a receipt contains the line "CABINET COOLER MASTER CD600 BLACK"
- **THEN** the consensus extraction yields brand "Cooler Master" and model "CD600"
- **AND** the asset is categorized accordingly

#### Scenario: Serial number on a photo of a product label
- **WHEN** a photo of a device label clearly shows a serial number
- **THEN** the consensus extraction yields the serial number
- **AND** the resulting asset identity is resolved by that serial

### Requirement: Product description splitting
When a document presents a combined product description string, the pipeline SHALL split it into brand (validated against the known-brand lexicon), a canonical product name (what the item is), and a model identifier. The full description string SHALL NOT be stored wholesale in the model field.

#### Scenario: Microwave oven description split
- **WHEN** a document contains "MICRO WAVE OVEN CONVECTION 30BRC2"
- **THEN** the consensus output separates canonical name "Microwave Oven" from model "30BRC2"
- **AND** neither field contains the entire raw string

#### Scenario: Cabinet description split with brand
- **WHEN** a receipt contains "CABINET COOLER MASTER CD600 BLACK"
- **THEN** the consensus output separates brand "Cooler Master", name "Cabinet", model "CD600"

### Requirement: Efficient existing-asset lookup during processing
Before creating any asset, the processing module SHALL look up existing assets in the same owner scope in a deterministic order: (1) normalized serial number; (2) normalized brand+model; (3) normalized name+model. Lookup cost SHALL be independent of registry size (observably: lookup latency does not grow linearly with the owner's asset count) and each stage SHALL return at most a bounded candidate set (default 10). When a match is found, the document SHALL be linked to the existing asset rather than creating a duplicate. (The index/normalization strategy to achieve this is a design decision, not specified here.)

#### Scenario: Lookup time is flat as the registry grows
- **GIVEN** owner X has 100 assets and owner Y has 10,000 assets
- **WHEN** the same document is processed for each owner
- **THEN** the identity-lookup phase completes within the same order of magnitude for both (e.g. Y's lookup is less than 5× X's), demonstrating no full-registry scan

#### Scenario: Serial match links without creating a duplicate
- **GIVEN** an owner with 10,000 existing assets including serial "012PFPM00313"
- **WHEN** a document extracts that serial
- **THEN** the document is linked to the existing asset and no new asset is created

#### Scenario: Fallback to brand+model when no serial
- **WHEN** a document extracts brand "LG" and model "FHT1408ZWL" with no serial, and an asset with that normalized brand+model exists
- **THEN** the document is linked to that existing asset

#### Scenario: Bounded candidate set
- **WHEN** any lookup stage runs
- **THEN** it returns at most the configured candidate limit (default 10) even if more rows match loosely

### Requirement: Deterministic warranty duration computation
When a document states warranty as a duration relative to purchase (e.g. "warranty 2 years", "1 yr warranty", "guarantee: 24 months") and a purchase date is known, the system SHALL compute `warranty_end = purchase_date + duration` deterministically in code rather than relying on the model to perform date arithmetic. Extracted explicit end dates SHALL take precedence over computed ones.

#### Scenario: Receipt states "warranty 2 years"
- **GIVEN** a receipt with purchase date 2026-01-15 and the text "warranty 2 years"
- **WHEN** the document is processed
- **THEN** the resulting asset has warranty_end 2028-01-15

#### Scenario: Explicit warranty end date wins
- **GIVEN** a document with purchase date 2026-01-15, text "warranty 2 years", and an explicit warranty expiry field of 2027-06-30
- **WHEN** the document is processed
- **THEN** warranty_end is 2027-06-30

### Requirement: Standalone processing module
Document processing SHALL be implemented as a standalone module with a single entry point (`process(source) -> outcome`) decoupled from the HTTP upload handler, so that any ingestion path (file upload, pasted text, statement file, future email watcher) uses the same pipeline. Processing outcomes SHALL be one of: `committed(asset)`, `held_for_review(document)`, `duplicate(existing_document)`, or `failed(reason)`.

#### Scenario: Same pipeline for file and text input
- **WHEN** a user submits a PDF invoice through the file path and later pastes the text of a different receipt through the text path
- **THEN** both submissions flow through the same processing module and produce equivalent outcome structures

### Requirement: Processing latency target
The processing pipeline SHALL complete the extraction and consensus phase for a typical single-page document within a configurable budget (default 30 seconds end-to-end for the synchronous response, including parallel workers). When the budget is exceeded the document SHALL be held for review with partial provenance rather than dropped.

#### Scenario: Slow model does not lose the document
- **GIVEN** the extraction workers exceed the latency budget
- **WHEN** the response is returned
- **THEN** the document is recorded as held_for_review and can complete processing asynchronously or be reviewed manually
