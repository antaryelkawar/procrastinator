# duplicate-reprocess-flow — Delta

## ADDED Requirements

### Requirement: Duplicate upload asks before deciding

When an uploaded document is detected as a content-hash duplicate of an existing document (same owner scope), the system SHALL prompt the user to choose between:

1. **Reprocess** — discard the cached outcome and re-run the extraction pipeline on the (possibly same-named but user-updated) document; and
2. **Keep existing** — acknowledge the duplicate and do nothing further.

The prompt SHALL identify the existing document (its filename/upload date) and the asset it is linked to, so the user can decide with context. The system SHALL NOT silently mark the upload duplicate-and-done without user confirmation — the only exception is the bounded timeout below, which is an explicit fallback, not a silent default.

#### Scenario: Duplicate upload prompts the user

- **WHEN** a user uploads a document whose content hash matches an existing document in their scope
- **THEN** the UI shows a prompt with "reprocess" and "keep existing" choices, before any new asset/document processing proceeds

#### Scenario: Choosing reprocess re-runs extraction

- **WHEN** the user chooses "reprocess" on a duplicate upload
- **THEN** the document re-enters the multi-worker extraction → consensus → resolution pipeline and may produce updated asset data

#### Scenario: Choosing keep existing dismisses

- **WHEN** the user chooses "keep existing"
- **THEN** no re-extraction runs, no new rows are created, and the user is returned to the landing page

#### Scenario: Legacy silent behavior is removed

- **WHEN** a duplicate upload occurs and no user choice has been made
- **THEN** the pipeline does not silently mark the document duplicate-and-done; it stays pending until the user answers

#### Scenario: Timeout fallback to keep existing

- **WHEN** a duplicate upload's pending prompt is not answered within 10 minutes
- **THEN** the upload outcome is recorded as keep-existing, no re-extraction runs, and the user sees a visible toast ("no response — keeping existing document")

### Requirement: Ingest entry has no type or account pre-selection

The ingest entry (landing [+] unified composer, per `landing-page` — delta 2 replaces the earlier three-card hub; documents-view corner [+], per `documents-section`) SHALL NOT require a document-type or account pre-selection before upload; type classification remains a pipeline decision (existing `core/statement` classification), never a user gate.

#### Scenario: Upload straight from the unified composer

- **WHEN** the user opens the unified Add composer via the landing [+] (or the documents-view corner [+]) and attaches any PDF/image
- **THEN** ingestion starts with no doc-type/account selection modal in between — the composer's optional directive-note field (if typed) is attached as an ingestion directive

The ingest entry SHALL offer the optional ingestion note/directive (e.g. "Anything the scanner should know?") as a plain always-available field within the composer — never a separate step — and SHALL pass it with the ingestion as a user directive (like the documents-section reprocess comment). The resulting duplicate-detection contract above is unchanged by the composer entry point.
