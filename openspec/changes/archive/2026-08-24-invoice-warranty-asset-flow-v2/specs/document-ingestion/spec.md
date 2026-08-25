# Spec: document-ingestion

Delta for change `invoice-warranty-asset-flow`. Modifies the archived baseline in `openspec/specs/document-ingestion`.

## MODIFIED Requirements

### Requirement: Document upload endpoint

The system SHALL expose `POST /api/documents` accepting `multipart/form-data` with the uploaded file in a form field named `file` and a tenant identifier in the `X-Tenant-ID` header (see the `multitenancy` capability). The endpoint SHALL process the document synchronously through the full ingestion flow (source retention, LLM classification and extraction, identity resolution, asset creation or update) before responding. On success the endpoint SHALL respond `201 Created` with a JSON body representing the resulting Asset, including its `doc_type` and `metadata`.

#### Scenario: Successful AMC invoice upload creates and returns an asset

- **WHEN** a client POSTs a valid AMC invoice PDF as multipart field `file` to `/api/documents` with `X-Tenant-ID: acme` and the LLM extracts usable asset identity fields
- **THEN** the response status is `201 Created` and the JSON body contains the created Asset with populated extracted fields and metadata

#### Scenario: Successful warranty upload returns the matched asset

- **WHEN** a client POSTs a valid warranty document for the same tenant whose extracted serial number matches an existing Asset
- **THEN** the response status is `201 Created` and the JSON body represents that existing Asset, updated with any newly extracted non-empty fields and merged metadata

#### Scenario: Missing file field is rejected

- **WHEN** a client POSTs to `/api/documents` with a valid tenant header but without a multipart field named `file`
- **THEN** the response status is `400 Bad Request` and no Source, Document, or Asset record is created

### Requirement: Source retention

Every accepted upload SHALL be retained as a Source record before any LLM processing, stamped with the requesting tenant. The raw file bytes SHALL be written to a configurable storage directory, and the Source record SHALL persist `tenant_id`, the original filename, sniffed content type, byte size, storage path, SHA-256 digest of the bytes, and the upload timestamp. A Source SHALL be retained even if downstream processing fails.

#### Scenario: Source survives processing failure

- **WHEN** an upload is accepted but LLM extraction subsequently fails
- **THEN** a Source record for the upload exists under the requesting tenant with its stored bytes intact and retrievable from the recorded storage path

### Requirement: Typed document link between source and asset

When an extraction is successfully linked to an Asset (created or matched), the system SHALL persist a Document record, stamped with the requesting tenant, that references exactly one Source and exactly one Asset, carries the classified document type (`invoice`, `warranty`, `amc`, or `other`), and retains the extracted structured fields **including the generic metadata object** and the raw LLM extraction payload for provenance.

#### Scenario: Document links source to asset with classified type and metadata

- **WHEN** an uploaded AMC contract is processed successfully
- **THEN** a Document record exists linking the upload's Source to the resulting Asset, with document type `amc`, the extracted structured fields plus metadata (e.g., `amc_card_number`, `icr_number`, tax fields), and the raw extraction payload

#### Scenario: Second document links to the same asset

- **WHEN** an AMC document is uploaded after an invoice and both resolve to the same Asset of the same tenant
- **THEN** two Document records exist, each with its own Source and type, both referencing the same Asset
