## ADDED Requirements

### Requirement: Soft-delete asset

The system SHALL provide an endpoint to soft-delete an asset (`DELETE /api/users/{userId}/assets/{assetId}`), which marks the asset with a `deleted_at` timestamp rather than removing the row. Soft-deleted assets SHALL be excluded from list, search, and get responses (404 on direct get), and SHALL be restorable within a configurable retention window (default 30 days) via a restore endpoint. After the retention window, a background purge MAY hard-delete the row and its document links (documents and sources are retained).

#### Scenario: Delete hides asset from list and search

- **WHEN** user soft-deletes asset A
- **THEN** subsequent list and search responses for that owner do not contain A
- **AND** `GET /assets/{A}` returns 404

#### Scenario: Restore within retention window

- **GIVEN** asset A was soft-deleted 5 days ago with a 30-day retention window
- **WHEN** user calls the restore endpoint for A
- **THEN** A reappears in list/search with all prior fields and document links intact

#### Scenario: Delete is owner-scoped

- **WHEN** user X attempts to delete an asset owned by user Y (not in a shared household)
- **THEN** the response is 404 and the asset is unchanged

### Requirement: Merge field strategy

When one asset (the duplicate) is merged into another (the survivor), the survivor SHALL retain its identity and its non-null field values SHALL win on conflict; fields that are null on the survivor but non-null on the duplicate SHALL be copied to the survivor. The survivor's `metadata` SHALL be shallow-merged per key, with the survivor's value winning on conflicting keys. User-corrected fields on the survivor (e.g. `asset_category`) SHALL NOT be overwritten by a merge.

#### Scenario: Null fields are filled from the duplicate

- **GIVEN** survivor A1 lacks a serial number and duplicate A2 has serial "SN123"
- **WHEN** A2 is merged into A1
- **THEN** A1 has serial "SN123"

#### Scenario: Survivor wins on conflict

- **GIVEN** A1 has model "CD600" and A2 has model "CD600-BLK"
- **WHEN** A2 is merged into A1
- **THEN** A1's model remains "CD600"

### Requirement: Merge document preservation

A merge SHALL move all document links from the duplicate to the survivor. Merge SHALL never erase, orphan, or reclassify documents; every document linked to either asset before the merge SHALL appear in the survivor's document list afterwards.

#### Scenario: Merge preserves all documents

- **GIVEN** A1 has invoice D1 and A2 has warranty D2
- **WHEN** A2 is merged into A1
- **THEN** `GET /assets/{A1}/documents` returns both D1 and D2

### Requirement: Merge outcome and audit

After a merge, the duplicate SHALL be soft-deleted with a `merged_into` reference pointing at the survivor, SHALL be excluded from list/search/get responses, and SHALL NOT be matchable by identity resolution. The merge SHALL be auditable: the survivor's record exposes which assets were merged into it and when.

#### Scenario: Merge two duplicate cooler assets

- **GIVEN** assets A1 and A2 both represent the same Cooler Master CD600 cabinet (created by duplicate uploads)
- **WHEN** user merges A2 into A1
- **THEN** A1 retains its identity, gains any fields only present on A2, and lists all documents previously linked to both A1 and A2
- **AND** A2 is soft-deleted with `merged_into = A1` and excluded from list/search

#### Scenario: Merge is auditable

- **WHEN** A2 has been merged into A1
- **THEN** A2's detail exposes that it was merged into A1 and the merge timestamp

### Requirement: Dedupe on document re-upload

When a document whose content hash already exists for the same owner is re-uploaded, the system SHALL NOT create a new asset. The response SHALL indicate the duplicate, reference the existing source/document and its linked asset, and leave the registry unchanged.

#### Scenario: Re-uploading the same invoice PDF

- **GIVEN** invoice PDF P was previously uploaded and created asset A
- **WHEN** the user uploads byte-identical P again
- **THEN** no new asset is created
- **AND** the response references existing asset A and flags `duplicate: true`

#### Scenario: Re-upload after asset deletion

- **GIVEN** document P created asset A, and A was later soft-deleted
- **WHEN** P is re-uploaded
- **THEN** no new asset is created
- **AND** the response indicates the prior document exists and its asset is deleted, offering restore instead of reprocessing

### Requirement: Soft-delete document

The system SHALL provide document deletion: deleting a document SHALL mark it deleted (consistent with the asset soft-delete pattern — marker, not row removal), SHALL NOT
remove its source bytes by policy, SHALL exclude it from asset document lists and from
the Documents view, and SHALL be restorable within the configured retention window. The
linked asset SHALL remain untouched; only the document link is affected. Deleting a
document SHALL NOT be available for another owner's documents (owner-scoped 404).

#### Scenario: Delete hides the document

- **WHEN** the user soft-deletes document D linked to asset A
- **THEN** D is absent from A's document list and from the Documents view default
  filter, and A itself is unchanged

#### Scenario: Restore within retention

- **GIVEN** document D was soft-deleted inside the retention window
- **WHEN** the user restores D
- **THEN** D reappears with its asset link intact

#### Scenario: Cross-owner delete is a 404

- **WHEN** user X attempts to delete a document owned by user Y (no shared household)
- **THEN** the response is 404 and the document is unchanged

### Requirement: Reprocess a document

The system SHALL provide a document reprocess action that re-runs extraction on the
document's existing retained source (no new upload). The run SHALL apply the same
pipeline and consensus gate as a fresh ingest (outcome may hold for review), SHALL
preserve the document↔asset linkage on success (fields merge per the standard rules),
and SHALL accept an optional user comment recorded in the document metadata/provenance.
When accompanying text is supplied with a reprocess, that text SHALL be processed
together with the existing source content rather than creating a new source.

#### Scenario: Reprocess a failed upload

- **GIVEN** an upload whose source was retained but extraction failed (no document row)
- **WHEN** the user triggers reprocess on it
- **THEN** extraction re-runs against the retained source and the item's status updates
  (processed / held / failed) accordingly

#### Scenario: Reprocess preserves the asset link

- **GIVEN** document D linked to asset A
- **WHEN** D is reprocessed and the run commits
- **THEN** the document remains linked to A and A's fields follow the standard merge rules

#### Scenario: Comment is recorded

- **WHEN** the user reprocesses D with comment "vendor corrected model number"
- **THEN** the comment is retained on the document's metadata/provenance

### Requirement: Reprocess an asset

The system SHALL provide an asset reprocess action that re-runs extraction across the
asset's linked documents (each through the standard pipeline with the consensus gate);
on success the asset's fields follow the standard merge rules and user-set fields
(category or edited data) SHALL NOT be overwritten by inference. The asset identity and
document links SHALL remain stable through a reprocess.

#### Scenario: Reprocess updates fields per merge rules

- **GIVEN** asset A with documents D1 and D2
- **WHEN** the user triggers reprocess on A
- **THEN** each document re-runs extraction and A's fields follow the merge rules

#### Scenario: User-set category is not overwritten

- **GIVEN** asset A with user-set category `electronics`
- **WHEN** A is reprocessed and the inference yields `other`
- **THEN** A's category remains `electronics`

#### Scenario: Low-confidence document holds for review

- **WHEN** a reprocess of asset A's document yields consensus below threshold
- **THEN** that document is held for review and A's fields are not corrupted by it
