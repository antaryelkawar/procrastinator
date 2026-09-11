# documents-section — Delta

## ADDED Requirements

### Requirement: Add-document entry is the composer, not a form

The documents view SHALL expose a top-right [+] "Add document" button (per `app-chrome` — context [+] on every non-landing view) that opens the unified ingest/creation flow following the directive-first composer of `intuitive-input` (single describe-text input + attach strip + optional directive note). The documents view SHALL NOT offer an old-style field-by-field "new document" form, and — like the landing [+] — it SHALL route submissions through the same ingestion endpoints, keeping the duplicate-reprocess 409 flow intact.

#### Scenario: Corner plus on documents opens the composer

- **WHEN** the user taps [+] on `/documents`
- **THEN** the unified ingest composer opens (attach strip + describe input + optional directive note), not a multi-field form; a submitted duplicate still triggers the duplicate-reprocess prompt

#### Scenario: Documents view keeps the base list contract unchanged

- **WHEN** the documents view renders after this delta
- **THEN** everything specified below still holds (statuses, filters, search, reprocess w/ optional comment, delete, edit asset)

## ADDED Requirements

### Requirement: Documents overview view

A Documents view (> `/documents`) SHALL list every document belonging to the user (household-scoped rows included), each row showing:

- the source filename/upload date,
- its **processing status** — one of `processed`, `in_review`, `failed`, `asset_less`,
- for processed documents, a link to the asset each document is linked to.

The view SHALL be filterable by status and searchable by filename.

```
┌──────────────────────────────────────────────────────────────────┐
│  Documents                                    [status ▾] [🔬 q]  │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ microwave-invoice.pdf   processed  ✓  → Asset: Microwave …   ⟲⟳│ │   ⟲ reprocess  ⟳ edit …
│ │ blurry-pic.jpg          failed       ✗  (retry link)        │ │
│ │ receipt-2.png           in_review    ⏳ "2 candidates"      │ │
│ │ screenshot.png          asset_less   –  (no matching asset) │ │
│ └──────────────────────────────────────────────────────────────┘ │
│  Row menu: Reprocess… (optional comment) / Delete / Open asset   │
│  `processed` rows: document-reprocess is NOT offered (the       │
│  pipeline already succeeded) — instead: Open / Edit asset /     │
│  Reprocess asset (per the reprocess-an-asset requirement)       │
└──────────────────────────────────────────────────────────────────┘
```

#### Scenario: Processed document shows its asset

- **WHEN** the documents view renders a `processed` document
- **THEN** the row displays the linked asset's name as a link to its asset detail page

#### Scenario: Filter by failed status

- **WHEN** the user filters the list to status `failed`
- **THEN** only failed documents are listed, each with a reprocess affordance

#### Scenario: Empty state

- **WHEN** the user has no documents matching the current filter
- **THEN** an empty state is shown with a link back to the landing [+] ingest hub

### Requirement: Reprocess unprocessed documents with optional comment

For documents in `failed`, `in_review`, or `asset_less` status, the user SHALL be able to trigger **reprocess**: the document goes back through the extraction pipeline (parallel multi-worker extraction → consensus → resolution). The user may attach an **optional comment** (free text) that is persisted with the document and passed to the extraction directive (e.g. "the serial is under the barcode").

#### Scenario: Reprocess a failed document with a hint

- **WHEN** the user clicks reprocess on a failed document and types "receipt, not invoice" as comment
- **THEN** the document re-enters the processing pipeline, the comment is stored and influences re-extraction, and the row transitions to `in_review` or `processed` when done

#### Scenario: Reprocess without comment

- **WHEN** the user reprocesses without entering a comment
- **THEN** the pipeline runs with no user directive attached

#### Scenario: Reprocess is not offered for already-pending work

- **WHEN** the document is already en route through processing
- **THEN** the reprocess action is disabled or the request is rejected without duplicating in-flight work

### Requirement: Delete a document

The user SHALL be able to delete a document from the documents view (confirm dialog), which detaches it from its asset and soft-deletes the document row (purgeable later). The linked asset SHALL NOT be deleted unless it becomes `asset_less` AND the user deletes it separately.

#### Scenario: Delete detaches but preserves the asset

- **WHEN** the user deletes the only processed document behind an asset
- **THEN** the document is removed from the list and the asset remains (now `asset_less`-eligible), rather than disappearing

### Requirement: Edit asset data from documents view

From a processed row (or its asset link), the user SHALL be able to open the linked asset and edit its data fields; edits save through the existing asset update path and re-show immediately after.

#### Scenario: Edit fields inline

- **WHEN** the user edits the product name/price on the linked asset from the documents context and saves
- **THEN** the asset is updated (user-set category precedence respected) and the documents view continues showing the same row without stale data

### Requirement: Reprocess an asset

A processed document's asset SHALL offer **reprocess asset**: re-running resolution/merge-aware refresh of that asset from its documents, without recreating duplicates.

#### Scenario: Asset reprocess refreshes data, not identity

- **WHEN** the user reprocesses an asset
- **THEN** the asset's extraction-derived fields refresh and its identity/`created_at` remain the same; if the documents now better match another existing asset, the merge flow applies instead
