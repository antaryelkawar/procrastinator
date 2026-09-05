# Spec: document-ingestion (delta)

Modified capability for change `proper-multi-tenant-search`. The upload endpoint's success response is now determined by the confidence gate: a high-confidence resolution auto-commits and responds `201` (unchanged), while a low-confidence (or confidence-less) resolution is held as a pending review and responds `202`. The `422` (no usable identity) and `502` (LLM failure) paths are unchanged. The confidence gate, review threshold, and pending-review data model are owned by the `confidence-review` capability.

## MODIFIED Requirements

### Requirement: Document upload endpoint

The system SHALL expose `POST /api/users/{userId}/documents` accepting `multipart/form-data` with the uploaded file in a form field named `file`. The endpoint SHALL process the document synchronously through the full ingestion flow (source retention, LLM classification and extraction — including the extraction's `confidence` — and confidence-gated identity resolution) before responding. The confidence gate determines the success response: when the extraction yields a usable identity and its confidence meets the review threshold, the resolution is committed (an Asset is created or merged and the Document is linked) and the endpoint SHALL respond `201 Created` with a JSON body representing the resulting Asset; when the extraction yields a usable identity but its confidence is strictly below the review threshold or is absent, the resolution is held for human review (the Source is retained and a pending-review candidate is recorded, and no Asset or Document is created) and the endpoint SHALL respond `202 Accepted` with a JSON body referencing the pending review. An extraction that yields no usable identity SHALL be reported as unprocessable (`422 Unprocessable Entity`) regardless of confidence, as defined by the "Processing failure handling" requirement.

#### Scenario: Successful invoice upload creates and returns an asset

- **WHEN** a client POSTs a valid PDF invoice as multipart field `file` to `/api/users/{userId}/documents` and the LLM extracts usable asset identity fields with a confidence at or above the review threshold
- **THEN** the response status is `201 Created` and the JSON body contains the created Asset with the populated extracted fields

#### Scenario: Successful warranty upload returns the matched asset

- **WHEN** a client POSTs a valid warranty document whose extracted serial number matches an existing Asset and whose confidence meets the review threshold
- **THEN** the response status is `201 Created` and the JSON body represents that existing Asset, updated with any newly extracted non-empty fields

#### Scenario: Low-confidence upload is held for review

- **WHEN** a client POSTs a valid document whose extraction yields a usable identity with a confidence strictly below the review threshold
- **THEN** the response status is `202 Accepted`, the JSON body references a pending review, the Source is retained, and no Asset or Document is created

#### Scenario: Confidence-less upload is held for review

- **WHEN** a client POSTs a valid document whose extraction yields a usable identity but carries no confidence
- **THEN** the response status is `202 Accepted` and the resolution is held as a pending review (fail-safe: an unknown-confidence read is never auto-committed)

#### Scenario: Missing file field is rejected

- **WHEN** a client POSTs to `/api/users/{userId}/documents` without a multipart field named `file`
- **THEN** the response status is `400 Bad Request` and no Source, Document, or Asset record is created
