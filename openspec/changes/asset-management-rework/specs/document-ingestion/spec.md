## MODIFIED Requirements

### Requirement: Document upload endpoint

The system SHALL expose `POST /api/users/{userId}/documents` accepting `multipart/form-data` with the uploaded file in a form field named `file`. The endpoint SHALL process the document synchronously through the unified processing pipeline (source retention, content-hash duplicate check, classification and extraction, consensus, identity resolution, asset creation or update) before responding. On success the endpoint SHALL respond `201 Created` with a JSON body representing the resulting Asset; when the upload is a byte-duplicate of an existing Source for the same owner it SHALL respond `200 OK` with the existing Document and Asset references and a `duplicate: true` flag instead of reprocessing.

(Previously: "The endpoint SHALL process the document synchronously through the full ingestion flow (source retention, LLM classification and extraction, identity resolution, asset creation or update) before responding. On success the endpoint SHALL respond `201 Created`..." — there was no content-hash duplicate check; a byte-identical re-upload was reprocessed through the LLM and could create duplicate assets.)

#### Scenario: Successful invoice upload creates and returns an asset

- **WHEN** a client POSTs a valid PDF invoice as multipart field `file` to `/api/users/{userId}/documents` and extraction yields usable asset identity fields
- **THEN** the response status is `201 Created` and the JSON body contains the created Asset with the populated extracted fields

#### Scenario: Byte-duplicate upload returns the existing asset

- **WHEN** a client POSTs a file whose SHA-256 matches an existing Source for the same owner
- **THEN** the response status is `200 OK` with `duplicate: true` and references to the existing Document and its Asset
- **AND** no new Source content, Document, or Asset is created

#### Scenario: Missing file field is rejected

- **WHEN** a client POSTs to `/api/users/{userId}/documents` without a multipart field named `file`
- **THEN** the response status is `400 Bad Request` and no Source, Document, or Asset record is created

### Requirement: Typed document link between source and asset

When an extraction is successfully linked to an Asset (created or matched), the system SHALL persist a Document record that references exactly one Source and exactly one Asset, carries the classified document classification (`invoice`, `receipt`, `warranty`, `amc`, `statement`, or `other`), and retains the extracted structured fields and the raw extraction payload(s) for provenance. Every Asset created from a document SHALL have at least one such linked Document, and every new non-duplicate document resolving to an existing Asset SHALL be appended to that Asset's document list.

(Previously: "carries the classified document type (`invoice`, `warranty`, or `other`), and retains the extracted structured fields and the raw LLM extraction payload for provenance." — the vocabulary is extended with `receipt`/`amc`/`statement`, and the guaranteed-linkage invariants are new.)

#### Scenario: Document links source to asset with classified type

- **WHEN** an uploaded invoice is processed successfully
- **THEN** a Document record exists linking the upload's Source to the resulting Asset, with classification `invoice`, the extracted structured fields, and the raw extraction payload

#### Scenario: Second document links to the same asset

- **WHEN** a warranty document is uploaded after an invoice and both resolve to the same Asset
- **THEN** two Document records exist, each with its own Source and classification, both referencing the same Asset

#### Scenario: New document for an existing asset appears in its document list

- **WHEN** a new, previously unseen AMC contract PDF resolves to an existing Asset for the same appliance
- **THEN** the Asset's document list grows by one entry classified `amc`
- **AND** the Asset's fields are updated only with newly extracted non-empty values

## ADDED Requirements (cycle 2)

### Requirement: Companion text with file input

An ingested item SHALL be able to consist of file(s) plus optional accompanying text,
with the accompanying text treated as additional information about the item and processed
together with the file(s) through the same pipeline (not as a separate document). Text
alone (no file) SHALL also form a valid item.

#### Scenario: Photo with a note processes as one item

- **WHEN** an upload contains a photo and the accompanying text "gifted by dad in 2024"
- **THEN** the pipeline processes photo + text together as one item and creates at most
  one document for it

#### Scenario: Standalone text ingests

- **WHEN** an upload contains only pasted text with no file
- **THEN** the text is stored as a source and processed through the same pipeline

### Requirement: Duplicate outcome requires a user decision

When an upload is a byte-duplicate of an existing source for the same owner, the system
SHALL NOT silently mark it duplicate without offering the user a choice: the duplicate
response SHALL carry the existing document and asset references AND SHALL surface a
reprocess option to the user (Reprocess on the existing source) versus keeping the
existing result. Choosing reprocess SHALL run the existing-document reprocess flow (see
asset-lifecycle), reusing the retained source; choosing keep SHALL leave the registry
unchanged.

#### Scenario: Duplicate upload prompts reprocess-vs-keep

- **GIVEN** an upload whose SHA-256 matches an existing source
- **WHEN** the outcome is returned
- **THEN** the response references the existing document/asset AND the UI presents
  Reprocess and Keep existing choices

#### Scenario: Keep existing is a no-op

- **WHEN** the user chooses Keep existing on a duplicate prompt
- **THEN** the registry is unchanged and no reprocessing runs

#### Scenario: Reprocess reuses the retained source

- **WHEN** the user chooses Reprocess on a duplicate prompt
- **THEN** extraction re-runs on the existing source (no new upload) and the outcome
  follows the standard pipeline rules
