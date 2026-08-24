# Spec: document-ingestion

Delta for change `invoice-warranty-asset-flow`. All requirements below are ADDED.

## ADDED Requirements

### Requirement: Document upload endpoint

The system SHALL expose `POST /api/documents` accepting `multipart/form-data` with the uploaded file in a form field named `file`. The endpoint SHALL process the document synchronously through the full ingestion flow (source retention, LLM classification and extraction, identity resolution, asset creation or update) before responding. On success the endpoint SHALL respond `201 Created` with a JSON body representing the resulting Asset.

#### Scenario: Successful invoice upload creates and returns an asset

- **WHEN** a client POSTs a valid PDF invoice as multipart field `file` to `/api/documents` and the LLM extracts usable asset identity fields
- **THEN** the response status is `201 Created` and the JSON body contains the created Asset with the populated extracted fields

#### Scenario: Successful warranty upload returns the matched asset

- **WHEN** a client POSTs a valid warranty document whose extracted serial number matches an existing Asset
- **THEN** the response status is `201 Created` and the JSON body represents that existing Asset, updated with any newly extracted non-empty fields

#### Scenario: Missing file field is rejected

- **WHEN** a client POSTs to `/api/documents` without a multipart field named `file`
- **THEN** the response status is `400 Bad Request` and no Source, Document, or Asset record is created

### Requirement: Upload validation

The system SHALL accept only PDF (`application/pdf`), PNG (`image/png`), and JPEG (`image/jpeg`) uploads, determined by content sniffing of the file bytes rather than the client-supplied Content-Type header alone. The system SHALL reject uploads larger than a configurable size limit (default 20 MiB). Rejected uploads SHALL NOT be persisted.

#### Scenario: Unsupported file type is rejected

- **WHEN** a client uploads a file whose sniffed content type is not PDF, PNG, or JPEG
- **THEN** the response status is `415 Unsupported Media Type` and nothing is persisted

#### Scenario: Oversized upload is rejected

- **WHEN** a client uploads a file exceeding the configured size limit
- **THEN** the response status is `413 Payload Too Large` and nothing is persisted

### Requirement: Source retention

Every accepted upload SHALL be retained as a Source record before any LLM processing. The raw file bytes SHALL be written to a configurable storage directory, and the Source record SHALL persist the original filename, sniffed content type, byte size, storage path, SHA-256 digest of the bytes, and the upload timestamp. A Source SHALL be retained even if downstream processing fails.

#### Scenario: Source survives processing failure

- **WHEN** an upload is accepted but LLM extraction subsequently fails
- **THEN** a Source record for the upload exists with its stored bytes intact and retrievable from the recorded storage path

#### Scenario: Source metadata is complete

- **WHEN** an upload is accepted
- **THEN** the Source record contains the original filename, content type, byte size, storage path, SHA-256 digest, and upload timestamp

### Requirement: Typed document link between source and asset

When an extraction is successfully linked to an Asset (created or matched), the system SHALL persist a Document record that references exactly one Source and exactly one Asset, carries the classified document type (`invoice`, `warranty`, or `other`), and retains the extracted structured fields and the raw LLM extraction payload for provenance.

#### Scenario: Document links source to asset with classified type

- **WHEN** an uploaded invoice is processed successfully
- **THEN** a Document record exists linking the upload's Source to the resulting Asset, with document type `invoice`, the extracted structured fields, and the raw extraction payload

#### Scenario: Second document links to the same asset

- **WHEN** a warranty document is uploaded after an invoice and both resolve to the same Asset
- **THEN** two Document records exist, each with its own Source and type, both referencing the same Asset

### Requirement: Processing failure handling

If LLM extraction fails (unreachable endpoint, timeout, non-2xx LLM response, or unparseable extraction payload), the endpoint SHALL respond `502 Bad Gateway` with an error body, the Source SHALL be retained, and no Document or Asset SHALL be created or modified. If extraction succeeds but yields no usable asset identity (no serial number and no brand+model) and no existing Asset matches, the endpoint SHALL respond `422 Unprocessable Entity`, the Source SHALL be retained, and no Document or Asset SHALL be created.

#### Scenario: LLM outage returns 502 without partial writes

- **WHEN** the LLM endpoint is unreachable during an upload
- **THEN** the response status is `502 Bad Gateway`, the Source is retained, and no Asset or Document is created or modified

#### Scenario: Unidentifiable document returns 422

- **WHEN** an uploaded document is classified as `other` and extraction yields no serial number and no brand+model
- **THEN** the response status is `422 Unprocessable Entity`, the Source is retained, and no Asset or Document is created
