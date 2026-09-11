## ADDED Requirements

### Requirement: Documents section lists every ingested source with status

The system SHALL expose a top-level **Documents** view listing every ingested item
(source: file or text) for the active owner, each with: filename/label, ingest timestamp,
**processing status**, and — when present — the linked asset (id + display name) the
document is attached to. Processing status values SHALL include at least: `processed`
(document created and linked to an asset), `in_review` (held for review), `failed`
(processing failed, no document row), and `asset_less` (document exists but its asset
link is gone, e.g. after purge). The view SHALL support filtering by status and free text,
and SHALL render as cards at mobile widths (not a phone-hostile desktop table) with the
shared DataTable available at desktop widths.

```text
DOCUMENTS — mobile                                     DOCUMENTS — desktop
+-------------------------------------+                +---------------------------------------------------+
| Documents                [☰] [◐]     |               | Documents                                [☰] [◐]  |
| [ status: All|Processed|Unprocessed] |               | status ▾: All | Processed | Unprocessed | Review  |
| [ search…                          ] |               | search ▾ ... sorted by newest                     |
| +---------------------------------+ |               +---------------------------------------------------+
| | 📷 label.jpg      [ Failed ]    | |               | # | file           | status    | asset  | ⋯     |
| |  2026-09-01   (no asset)        | |               | 1 | inv.pdf        | processed | ▶ Cooler| …     |
| |  [Reprocess]      [Delete ▾]    | |               | 2 | amc.pdf        | processed | ▶ Micro | …     |
| +---------------------------------+ |               | 3 | label.jpg      | failed    | —       | ⋯     |
| | 💬 text note    [ In review ]   | |               | 4 | text note      | in review | ▶ optl  | ⋯     |
| |  (opens review queue)           | |               | ⋯ (per-row menu: Reprocess / Delete / linked asset) |
| +---------------------------------+ |               +---------------------------------------------------+
|  actions row-anchored, ≥44px touch |                | cards on mobile; shared DataTable on desktop      |
+-------------------------------------+                +---------------------------------------------------+
```

#### Scenario: All sources are visible

- **WHEN** a photo upload succeeded (processed), a pasted text was held in review, and an
  upload failed at processing
- **THEN** all three appear in the Documents view with statuses `processed`, `in_review`,
  and `failed` respectively, and the processed item shows its linked asset

#### Scenario: Status filter narrows the list

- **WHEN** the user filters status to `unprocessed` (i.e. `failed` + `asset_less`)
- **THEN** only stuck/failed/asset-less items are shown

#### Scenario: Mobile renders cards

- **WHEN** the view renders at 360px width
- **THEN** items render as cards with status badges and row-anchored actions (no desktop table)

### Requirement: Stuck uploads are recoverable

Every item visible in the Documents view SHALL be actionable: a `failed` or `asset_less`
item SHALL offer **Reprocess**; a processed item SHALL offer **Delete** (soft-delete).
"Recoverable" means the user can always see the item and act on it from this view.

#### Scenario: A stuck upload is visible and offers reprocess

- **GIVEN** an upload whose source was retained but whose extraction failed (no document row)
- **WHEN** the user opens the Documents view
- **THEN** the upload is listed with status `failed` and a Reprocess action

#### Scenario: A processed item offers delete

- **WHEN** the user opens the Documents view
- **THEN** processed items show a Delete action (behind confirmation)

### Requirement: Reprocess a document

Documents-view items SHALL offer a **Reprocess** action that re-runs extraction on that
item's existing retained source (no new upload required). The reprocess SHALL respect the
consensus gate as a normal run (may hold for review), SHALL preserve the existing
document↔asset linkage on success, and SHALL accept an **optional user comment** recorded
in the document metadata/provenance. When the user supplies accompanying text during
reprocess, that text and the existing source content SHALL be processed together rather
than creating a second source-record (upsert mechanics are a design concern).

#### Scenario: Reprocess a failed upload

- **GIVEN** an item with status `failed`
- **WHEN** the user triggers Reprocess (optionally with a comment)
- **THEN** extraction re-runs against the retained source and the resulting outcome
  (committed/held/failed) updates the item's status accordingly

#### Scenario: Reprocess with accompanying text upserts input

- **WHEN** the user reprocesses an item and adds accompanying text
- **THEN** the extraction run consumes existing source content plus the accompanying text,
  and no new source record is created for the text alone

#### Scenario: Successful reprocess keeps the asset link

- **GIVEN** an item currently linked to asset A
- **WHEN** the user reprocesses it and the run commits
- **THEN** the document remains linked to A and A's fields follow the standard merge rules

### Requirement: Soft-delete a document

Documents SHALL be soft-deletable: `delete` sets a marker without removing the row or its
linked asset. Deleted documents SHALL be excluded from asset document lists and from the
Documents view (unless an explicit filter shows them), and SHALL be restorable within the
configured retention window — consistent with asset lifecycle deletion semantics. Source
blobs SHALL be retained by policy.

#### Scenario: Delete hides the document from asset and view

- **WHEN** the user deletes a document linked to asset A
- **THEN** subsequent asset document lists and the Documents view exclude it
- **AND** asset A and its other documents are unaffected

#### Scenario: Restore within retention window

- **GIVEN** a document deleted inside the retention window
- **WHEN** the user (or a purge path) restores it
- **THEN** the document link and asset association reappear

### Requirement: Reprocess an asset

An asset SHALL offer a **Reprocess** action that re-runs extraction across its linked
documents (or a chosen subset) using each document's retained source; the same consensus/
gate rules apply per document, and the asset's fields update per the standard merge rules
(user-set fields stay sticky). The asset SHALL survive reprocessing (identity/linkage
preserved; user-set category and edited fields are not overwritten by inference).

#### Scenario: Asset reprocess preserves sticky fields

- **GIVEN** asset A with user-set category `electronics` and a reprocess run
- **THEN** the run's inferred category SHALL NOT overwrite `electronics` and edited fields
  are preserved

#### Scenario: Asset reprocess may re-hold a document

- **WHEN** reprocessing a document of asset A yields low consensus confidence
- **THEN** that document is held for review and asset A is not corrupted

### Requirement: Duplicate-upload prompt (reprocess vs keep)

When the add flow yields a `duplicate` outcome (same content hash), the UI SHALL present
an explicit prompt with the existing document/asset references and two choices —
**Reprocess** or **Keep existing result**. Choosing Reprocess SHALL trigger the document
reprocess flow; choosing Keep existing SHALL dismiss without further processing.
The system SHALL NOT silently mark a duplicate without presenting the choice.

```text
+---------------------------------------------------------------+
|  ⚠ This document is already on file.                          |
|    "INVNAG2302754.pdf"  →  linked asset: Cooler Master Cabinet|
|                                                               |
|      [ Reprocess ]           [ Keep existing ]                |
|      optional comment: [____________________] (on Reprocess)  |
+---------------------------------------------------------------+
```

#### Scenario: Prompt offers both paths

- **WHEN** the user's added file matches an existing source's hash
- **THEN** the prompt shows the existing file and its linked asset and offers Reprocess
  and Keep existing

#### Scenario: Keep existing is a no-op

- **WHEN** the user chooses Keep existing
- **THEN** no reprocessing runs and the existing document/asset are untouched

### Requirement: Row-level actions are touch-friendly and consistent

Actions in the Documents view SHALL (Reprocess / Delete / open linked asset / open review) be reachable per row, SHALL render as a consistent action menu (one action
primitive per job per the web-platform single-pattern rule), and SHALL meet the
mobile-first touch-target bar (≥44px).

#### Scenario: Consistent action pattern on rows

- **WHEN** the user opens any item's actions in the Documents view
- **THEN** the same action-menu primitive is used (no row-specific hand-rolled menu)
