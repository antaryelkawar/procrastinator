# document-ingestion delta — fix-review-cycle-criticals

## MODIFIED Requirements

### Requirement: Document upload endpoint (outcome correction)

The system SHALL expose `POST /api/users/{userId}/documents` as specified, but the
response outcome is confidence-gated and asynchronous-hold-capable:

- Extraction that crosses the commit threshold and resolves to an Asset SHALL
  respond `201 Created` with a JSON body representing the resulting (created or
  matched, updated) Asset and MAY include a processing prompt/report for the UI.
- Extraction whose confidence falls below the hold threshold SHALL respond
  `202 Accepted`, SHALL NOT create or modify any Asset, and the pending review
  SHALL be surfaced by the ingest review endpoints. The response body SHALL carry
  the held review's state (`pending_review` or equivalent controlled value) and,
  when the document is reprocessable, a `prompt.reprocess_uri` pointing at
  `POST /api/users/{userId}/documents/{id}/reprocess`.
- `400` (missing field), `415` (sniffed type), `413` (size), and `502` (LLM outage
  with no partial writes) semantics are unchanged.

#### Scenario: Held-for-review upload returns 202

- **WHEN** a client uploads a valid PDF whose extraction confidence is below the
  commit threshold
- **THEN** the response status is `202 Accepted`, no Asset is created or modified,
  a Source and a Document in the `in_review` state exist, and a pending review is
  listable under the ingest review queue endpoints

#### Scenario: Held upload exposes a reprocess URI

- **WHEN** an upload is held for review
- **THEN** the response body contains a `prompt.reprocess_uri` of the form
  `/api/users/{userId}/documents/{id}/reprocess` referencing the held document and
  the resolved review entry is listed by `GET /api/users/{userId}/ingest/reviews`

#### Scenario: Outage still yields 502 with no partial writes

- **WHEN** the LLM endpoint is unreachable during an upload
- **THEN** the response status is `502 Bad Gateway`, the Source is retained, and
  no Asset, Document, or review is created or modified

## ADDED Requirements

### Requirement: Ingest review lifecycle

The system SHALL persist a review record when an upload is held for review, with
a state from the controlled vocabulary {`pending`, `approved`, `rejected`}, a link
to the held Document (and retained candidate payload when identity resolution was
ambiguous), and nullable decision timestamps. The review lifecycle SHALL be a
one-way state machine: `pending` may transition at most once to `approved` or
`rejected`; `approved` and `rejected` are terminal. The review queue SHALL list
pending reviews scoped to the tenant; it SHALL expose completed reviews' terminal
state via get but SHALL NOT return them as pending.

#### Scenario: Held upload creates a pending review

- **WHEN** an upload is held for review
- **THEN** a review in state `pending` exists, referencing the held Document, and
  it appears in the tenant's review queue listing

#### Scenario: Approve terminalizes the review once

- **WHEN** a client POSTs to `/api/users/{userId}/ingest/reviews/{id}/approve`
  for a pending review
- **THEN** the response is `200 OK`, the review state becomes `approved`,
  `decided_at` is set, and the candidate Asset + Document are committed

#### Scenario: Re-approve conflicts

- **WHEN** the same approve endpoint is called again on a review already in an
  approved or rejected state
- **THEN** the response status is `409 Conflict`, the review state is unchanged,
  and no duplicate Asset or Document rows are created

#### Scenario: Reject terminalizes without creating an asset

- **WHEN** a client POSTs to the review reject endpoint for a pending review
- **THEN** the review state becomes `rejected`, `decided_at` is set, and no Asset
  is created; the underlying Document's outcome reflects the rejection (e.g. it
  becomes restorable/reprocessable and does not silently disappear)

#### Scenario: Decision timestamps are immutable after terminal state

- **WHEN** a review has reached an approved or rejected state and any further
  decision mutation is attempted
- **THEN** the mutation is refused with `409 Conflict` and both the state and the
  recorded timestamps remain unchanged

### Requirement: Document reprocess in-flight guard

`POST /api/users/{userId}/documents/{id}/reprocess` SHALL restart ingestion for a
held document and respond `202 Accepted`. It SHALL be guarded against concurrent
duplication: while a reprocess for that document is in flight — including a live
pending review for the document's source — a second reprocess SHALL fail with
`409 Conflict`. A comment in the optional JSON request body, when supplied, SHALL
be recorded with the document for the reprocessing pipeline. Reprocessing SHALL
NOT create duplicate Assets for the same document instance (the re-resolved asset
replaces the held one rather than being appended).

#### Scenario: First reprocess accepted, second conflicts

- **WHEN** a reprocess is issued for a held document and, before it completes, a
  second reprocess for the same document is issued
- **THEN** the first responds `202 Accepted` and the second responds `409 Conflict`

#### Scenario: Pending review blocks reprocess

- **WHEN** a pending review exists for the document's source and a reprocess is
  requested for that document
- **THEN** the reprocess responds `409 Conflict` and no duplicate processing run
  is started

#### Scenario: Reprocess does not duplicate assets

- **WHEN** a held document is reprocessed and identity resolution resolves to an
  Asset (pre-existing or newly created)
- **THEN** the document points at exactly one Asset and no orphan/duplicate asset
  left over from the held state exists for that document
