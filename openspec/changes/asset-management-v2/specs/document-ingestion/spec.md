# document-ingestion — Delta

## MODIFIED Requirements

### Requirement: Document upload endpoint

The system SHALL expose `POST /api/users/{userId}/documents` accepting `multipart/form-data` with the uploaded file in a form field named `file`, plus an optional free-text note (a user directive for the extraction, see `duplicate-reprocess-flow`). The endpoint SHALL process the document synchronously through the full ingestion flow (source retention, LLM classification and extraction, identity resolution, asset creation or update) before responding. On success the endpoint SHALL respond `201 Created` with a JSON body representing the resulting asset. *(Previously: the response body was a flat/structured asset representation without the entity-shaped nested payload, duplicate content-hash uploads were silently marked duplicate by the pipeline, and there was no note parameter — all three clauses are new here; the "Successful warranty upload returns the matched asset" scenario was merged into the duplicate/outcome contract below.)* When the upload's content hash matches an existing document in the caller's scope, the endpoint SHALL NOT silently mark the upload duplicate; it SHALL return the duplicate-detection outcome so the caller can prompt the user to reprocess or keep the existing document (see `duplicate-reprocess-flow`). The unified add handler SHALL accept the ingest call without doc-type or account pre-selection parameters, with the note supplied alongside the file. *(Delta 2: the client-side ingest entry is the unified Add composer of `landing-page` / `intuitive-input`, not the earlier three cards or legacy forms — this requirement constrains only the entry interaction; the endpoint contract (multipart file + optional note + 409 duplicate outcome) is byte-identical, so the composer and the documents-view corner [+] both feed this same endpoint.)*

#### Scenario: Successful invoice upload creates and returns an asset

- **WHEN** a client POSTs a valid PDF invoice as multipart field `file` to `/api/users/{userId}/documents` and the LLM extracts usable asset identity fields
- **THEN** the response status is `201 Created` and the JSON body contains the created asset in entity-shaped form (`data` nested payload per `jsonb-entity-storage`)

#### Scenario: Ingest note is accepted and attached

- **WHEN** a client uploads a valid image with an optional note "the serial is under the barcode"
- **THEN** ingestion succeeds and the note is persisted with the document and passed to the extraction as a user directive

#### Scenario: Ingest without a note still works

- **WHEN** a client uploads a valid PDF without a note field
- **THEN** ingestion proceeds normally with an empty user directive

#### Scenario: Duplicate upload no longer silently marks duplicate

- **WHEN** a client POSTs a file whose content hash matches an existing document
- **THEN** the response communicates the duplicate condition (structured status, e.g. `409`/included flag + existing document reference) instead of creating or silently marking a duplicate; re-extraction happens only after the user's reprocess choice

#### Scenario: Ingest without pre-selection

- **WHEN** a client uploads with no doc-type/account hints supplied
- **THEN** ingestion proceeds and classification is decided by the pipeline, not the payload

#### Scenario: Missing file field is rejected (carried over)

- **WHEN** a client POSTs to `/api/users/{userId}/documents` without a multipart field named `file`
- **THEN** the response status is `400 Bad Request` and no source, document, or asset record is created
