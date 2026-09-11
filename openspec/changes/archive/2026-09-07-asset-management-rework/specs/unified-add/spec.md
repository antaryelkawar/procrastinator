## ADDED Requirements

### Requirement: Single unified add endpoint and entry point

The system SHALL provide one primary "add" entry point accepting: file upload (photo/image, PDF), pasted or typed free text, and statement files (CSV/PDF). Format and intent SHALL be auto-detected; the user SHALL NOT be required to choose between "document upload" and "statement import" flows. The UI SHALL expose a single prominent Add surface.

#### Scenario: Add a photo of a product label

- **WHEN** the user drops a JPEG photo of a device label into the Add surface
- **THEN** it is accepted, processed through the unified pipeline, and yields an asset (or review hold) — same as any other document

#### Scenario: Add pasted receipt text

- **WHEN** the user pastes the text of a receipt into the Add surface
- **THEN** the text is stored as a source, processed through the same pipeline, and can produce an asset

#### Scenario: Add a statement file without a separate flow

- **WHEN** the user drops a bank statement CSV into the Add surface
- **THEN** the system detects it as a statement and routes it into the ledger import pipeline (preview/commit) without the user visiting a separate import page
- **AND** the statement source is recorded like any other added item

### Requirement: Unified add result feedback

Every add submission SHALL return a uniform outcome: `asset_committed` (with asset ref), `held_for_review` (with review ref), `duplicate` (with existing document/asset refs), `statement_preview` (with import batch ref), or `failed` (with reason). The UI SHALL render these outcomes consistently regardless of input kind.

#### Scenario: Mixed batch of files added at once

- **WHEN** the user adds two receipts and one statement in one action
- **THEN** each item yields its own uniform outcome and the user can navigate to each result (asset, review queue, or statement preview) from one summary view

## MODIFIED Requirements

### Requirement: Single unified add endpoint and entry point (cycle 2)

The system SHALL provide one primary "add" entry point accepting: file upload (photo/image, PDF), pasted or typed free text, and statement files (CSV/PDF) — and SHALL accept
**one or more files plus optional accompanying text as a single item** (the text is
additional information about the item, not a separate document; text alone is also a valid
item). Format and intent SHALL be auto-detected; the user SHALL NOT be required to choose
between "document upload" and "statement import" flows, and SHALL NOT be required to
pre-select a document type or an account before providing input. The UI SHALL expose a
single prominent Add surface (the [+] trigger on the landing page).

(Previously: each item was a bare file or a bare text; accompanying text and the no-pre-selection rule were not specified.)

#### Scenario: Photo plus note processes as one item

- **WHEN** the user adds a photo with an accompanying note
- **THEN** both are treated as one item and processed together through the same pipeline

#### Scenario: Text alone processes like a file

- **WHEN** the user submits typed/pasted text without a file
- **THEN** the text is processed through the same pipeline and returns the same outcome structure

#### Scenario: No pre-selection is required

- **WHEN** the user starts the add flow from the landing page
- **THEN** no document-type or account selection is requested before input

### Requirement: Unified add result feedback (cycle 2 wording)

Every add submission SHALL return a uniform outcome per item — `asset_committed`,
`held_for_review`, `duplicate`, `statement_preview`, or `failed` — and for `duplicate`
outcomes the outcome SHALL carry the existing document and asset references plus the
availability of a **reprocess** choice so the UI can offer reprocess-vs-keep (see
document-management). The UI SHALL render these outcomes consistently regardless of input
kind.

(Previously: a duplicate was a silent badge with no reprocess affordance.)

#### Scenario: Duplicate outcome carries the reprocess affordance

- **WHEN** an item's outcome is `duplicate`
- **THEN** the outcome includes the existing document+asset references and the UI offers
  Reprocess and Keep existing
