# Spec: confidence-review

New capability for change `proper-multi-tenant-search`. Adds the confidence model and the human-in-the-loop review surface for uncertain ingest resolutions. It owns the configurable **review threshold**, the **confidence gate** applied to identity resolution (an extraction whose confidence meets the threshold is committed exactly as the existing pipeline does; one whose confidence is below the threshold — or absent — is held as a **pending review**), the owner-scoped, RLS-protected **pending-review** data model, and the endpoints to list pending reviews, inspect one, **approve** it (commit: merge into the matched Asset or create a new one, reusing the existing identity-resolution and field-merge logic), or **reject** it (discard the candidate, retain the Source). It conforms to the existing owner-model visibility rule, cross-owner isolation, RLS backstop, and OpenAPI source-of-truth conventions. This is an MVP: confidence is a single overall value (no per-field confidence) and there is no re-extraction, no editing of a candidate, and no un-approve.

## ADDED Requirements

### Requirement: Confidence-gated identity resolution

When a document upload produces an extraction that yields a usable identity (a serial number, or a brand+model pair) and carries a `confidence`, the system SHALL gate the application of identity resolution by that confidence against a configurable review threshold. When the confidence is greater than or equal to the threshold, the system SHALL apply the existing identity resolution and commit it (merge into a matched Asset, or create a new Asset, and link the Document) exactly as before. When the confidence is strictly less than the threshold, the system SHALL NOT commit the resolution: it SHALL retain the Source, record a pending-review candidate capturing the extracted fields, the document type, the confidence, and the best-matched existing Asset (if any), and create no Asset and no Document. When the confidence is absent (the LLM did not return one, or it failed validation), the system SHALL treat it as below the threshold and hold the candidate for review (fail-safe: an unconfident or unknown-confidence read is never auto-committed). An extraction that yields no usable identity SHALL be held for review neither created nor rejected by this rule — it SHALL be reported as unprocessable (`422`) as defined by the `document-ingestion` capability.

#### Scenario: High-confidence serial is committed

- **WHEN** an upload extracts serial number `SN123` with confidence `0.9` and the review threshold is `0.7`
- **THEN** identity resolution is applied and committed (a matched Asset is updated or a new Asset is created, and the Document is linked)

#### Scenario: Exactly at threshold is committed

- **WHEN** an upload extracts a usable identity with confidence exactly `0.7` and the review threshold is `0.7`
- **THEN** the resolution is committed (the comparison is greater-than-or-equal)

#### Scenario: Low-confidence is held for review

- **WHEN** an upload extracts serial number `SN999` with confidence `0.4` and the review threshold is `0.7`, and no existing Asset matches
- **THEN** no Asset and no Document are created, the Source is retained, and a pending-review candidate is recorded with the extracted identity, its confidence, and no matched Asset

#### Scenario: Low-confidence with a match holds the candidate and its match

- **WHEN** an upload extracts brand `Samsung` and model `WW90T534DAW` with confidence `0.5` and the review threshold is `0.7`, and an existing Asset in the same owner scope matches on brand+model
- **THEN** no merge occurs, no Asset or Document is created or modified, and a pending-review candidate is recorded whose best-matched Asset is that existing Asset

#### Scenario: Absent confidence is held for review (fail-safe)

- **WHEN** an upload extracts a usable identity but the LLM returned no confidence (or an out-of-range value that was treated as absent)
- **THEN** the resolution is not committed, no Asset or Document is created, and a pending-review candidate is recorded for review

#### Scenario: No usable identity is not a review

- **WHEN** an upload extracts no serial number and no brand+model pair
- **THEN** the `confidence-review` rule neither holds nor commits anything; the upload is reported as unprocessable by the `document-ingestion` capability

### Requirement: Review threshold configuration

The review threshold SHALL be configured exclusively through the environment variable `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD`, a real number in the closed interval `[0.0, 1.0]`. When the variable is unset or empty, the system SHALL use the default `0.7`. When the variable is set to a value that is not a real number or is outside `[0.0, 1.0]`, the application SHALL fail fast at startup with a clear error naming the variable. No threshold value SHALL be hardcoded beyond the documented default.

#### Scenario: Unset threshold uses the default

- **WHEN** the application starts with `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD` unset
- **THEN** the review threshold in effect is `0.7`

#### Scenario: Valid threshold is honored

- **WHEN** the application starts with `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD=0.9`
- **THEN** the review threshold in effect is `0.9`

#### Scenario: Malformed threshold fails fast

- **WHEN** the application starts with `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD=abc` or `=1.5`
- **THEN** startup aborts with an error naming `PROCRASTINATOR_INGEST_REVIEW_THRESHOLD`

### Requirement: Pending-review data model

The system SHALL persist pending reviews scoped per owner (`owner_id` with a nullable `owner_household_id`). A pending review SHALL have: an opaque identifier; a reference to the retained Source of the upload that produced it; a document type from the classification vocabulary; the extracted structured fields of the candidate (as JSON); a `confidence` (the extraction's confidence, which is below the threshold or absent at creation); an optional reference to the best-matched existing Asset in the same owner scope (null when no Asset matched); a lifecycle state from the controlled vocabulary {`pending`, `approved`, `rejected`}; a creation timestamp; and, once decided, a decision timestamp and the deciding user. A pending review SHALL be created in state `pending` and SHALL transition exactly once to `approved` (via approve) or to `rejected` (via reject); `approved` and `rejected` are terminal states. Approving SHALL commit the candidate using the existing identity resolution and field-merge logic (merge into the matched Asset, or create a new Asset, and link the Document to it); the committed Asset's document type and confidence SHALL reflect the candidate's classification and confidence. Rejecting SHALL discard the candidate (no Asset, no Document), retain the Source, and change no existing Asset.

#### Scenario: Pending review is created from a held upload

- **WHEN** an upload is held for review
- **THEN** a pending review exists in state `pending` referencing the retained Source, carrying the candidate's extracted fields, document type, confidence, best-matched Asset (if any), and its owner scope

#### Scenario: Approve with a match merges into the matched Asset

- **WHEN** a pending review whose best-matched Asset is Asset A is approved
- **THEN** the candidate's non-empty extracted fields are merged into Asset A (absent fields do not erase existing values), a Document is linked to A, the review transitions to `approved` with a decision timestamp and deciding user, and no new Asset is created

#### Scenario: Approve without a match creates a new Asset

- **WHEN** a pending review with no best-matched Asset is approved
- **THEN** a new Asset is created from the candidate's extracted fields, a Document is linked to it, and the review transitions to `approved`

#### Scenario: Reject discards the candidate and retains the Source

- **WHEN** a pending review is rejected
- **THEN** the review transitions to `rejected` with a decision timestamp and deciding user, no Asset or Document is created, no existing Asset is modified, and the Source remains retained and retrievable

#### Scenario: Terminal reviews do not transition

- **WHEN** a review is in state `approved` or `rejected`
- **THEN** no operation transitions it to any other state

### Requirement: List pending reviews

The system SHALL expose `GET /api/users/{userId}/ingest/reviews` returning `200 OK` with a JSON array of the requesting user's reviews, ordered by creation timestamp ascending with ties broken by identifier ascending. The endpoint SHALL accept an optional `status` query parameter restricted to the lifecycle vocabulary {`pending`, `approved`, `rejected`}; when `status` is absent it SHALL default to `pending`. An unrecognized `status` value SHALL be rejected with `400 Bad Request`. The array SHALL include only reviews visible to the requester under the owner-model visibility rule. An empty result SHALL return `200 OK` with body `[]`, not an error.

#### Scenario: Default lists pending reviews

- **WHEN** a user has two `pending` reviews and one `approved` review and calls the list endpoint with no `status`
- **THEN** the response is `200 OK` with an array containing only the two `pending` reviews

#### Scenario: Filter by status

- **WHEN** a user has two `pending` reviews and one `rejected` review and calls the list endpoint with `status=rejected`
- **THEN** the response is `200 OK` containing only the `rejected` review

#### Scenario: Empty pending list returns an empty array

- **WHEN** a user has no pending reviews and calls the list endpoint
- **THEN** the response is `200 OK` with body `[]`

#### Scenario: Unrecognized status is rejected

- **WHEN** the list endpoint is called with `status=maybe`
- **THEN** the response is `400 Bad Request`

#### Scenario: Another owner's review is not listed

- **WHEN** user `alice` calls the list endpoint and user `bob` has a pending review
- **THEN** bob's review does not appear in alice's result

### Requirement: Read a review

The system SHALL expose `GET /api/users/{userId}/ingest/reviews/{id}` returning `200 OK` with the review's identifier, state, document type, confidence, extracted candidate fields, the originating Source's filename and upload timestamp, and the best-matched Asset (its identifier and a display title, or absent), or `404 Not Found` for an unknown identifier or one not visible to the requester.

#### Scenario: Existing review is returned

- **WHEN** a client requests `GET /api/users/{userId}/ingest/reviews/{id}` for an existing review in the requester's scope
- **THEN** the response is `200 OK` with the review's fields, its Source metadata, and its best-matched Asset (or its absence)

#### Scenario: Unknown review returns 404

- **WHEN** a client requests a review identifier that does not exist
- **THEN** the response is `404 Not Found`

#### Scenario: Another owner's review is invisible

- **WHEN** user `alice` requests `GET /api/users/{userId}/ingest/reviews/{id}` for a review created under owner `bob`
- **THEN** the response is `404 Not Found` and no field of bob's review is exposed

### Requirement: Approve a review

The system SHALL expose `POST /api/users/{userId}/ingest/reviews/{id}/approve` which commits the candidate of a `pending` review and responds `200 OK` with the resulting Asset (the merged or newly created one) and the review's new state. Commit SHALL be atomic: either the Asset merge/creation, the Document link, and the review transition all succeed, or none do. Approving a review that is not in state `pending` (already `approved` or `rejected`) SHALL be rejected with `409 Conflict` and change nothing. An unknown review identifier, or one not visible to the requester, SHALL yield `404 Not Found`. Approving SHALL record the deciding user (the requesting `{userId}`) and a decision timestamp.

#### Scenario: Approve commits and returns the Asset

- **WHEN** a client POSTs to the approve endpoint of a `pending` review
- **THEN** the response is `200 OK` with the resulting Asset, the review is in state `approved` with a decision timestamp and the deciding user set, and the Document is linked

#### Scenario: Approve is atomic

- **WHEN** a failure occurs while committing an approval
- **THEN** the Asset merge/creation, the Document link, and the review transition all leave no partial state (the review remains `pending` and no Asset or Document changed)

#### Scenario: Approving a non-pending review is rejected

- **WHEN** a client POSTs to the approve endpoint of an already-`approved` or `rejected` review
- **THEN** the response is `409 Conflict` and no Asset, Document, or review state changes

#### Scenario: Approving an unknown review returns 404

- **WHEN** a client POSTs to the approve endpoint of an unknown or another owner's review identifier
- **THEN** the response is `404 Not Found`

### Requirement: Reject a review

The system SHALL expose `POST /api/users/{userId}/ingest/reviews/{id}/reject` which discards the candidate of a `pending` review and responds `200 OK`. Rejecting SHALL transition the review to `rejected` with a decision timestamp and the deciding user, SHALL retain the Source, and SHALL change no Asset and create no Document. Rejecting a review that is not in state `pending` SHALL be rejected with `409 Conflict` and change nothing. An unknown review identifier, or one not visible to the requester, SHALL yield `404 Not Found`.

#### Scenario: Reject discards the candidate

- **WHEN** a client POSTs to the reject endpoint of a `pending` review
- **THEN** the response is `200 OK`, the review is in state `rejected` with a decision timestamp and deciding user, no Asset or Document is created, and the Source is retained

#### Scenario: Rejecting a non-pending review is rejected

- **WHEN** a client POSTs to the reject endpoint of an already-`approved` or `rejected` review
- **THEN** the response is `409 Conflict` and the review state is unchanged

#### Scenario: Rejecting an unknown review returns 404

- **WHEN** a client POSTs to the reject endpoint of an unknown or another owner's review identifier
- **THEN** the response is `404 Not Found`

### Requirement: Review is a proper multi-tenant capability

All review routes SHALL live under the single `/api/users/{userId}` path-tenancy group and SHALL be resolved by the existing user middleware; there SHALL be no header-based tenancy for reviews. The `{userId}` path parameter is the sole identity of the requesting user; a malformed `{userId}` SHALL be rejected with `400 Bad Request` and a well-formed but unregistered `{userId}` SHALL be rejected with `404 Not Found`, in both cases before any review work runs. Every review row SHALL be owner-scoped (`owner_id` NOT NULL, nullable `owner_household_id`) and visible only to the requester under the existing owner-model visibility rule (a row is visible iff `owner_id = me` OR the requester is a member of the row's `owner_household_id`). The review table SHALL be RLS-protected so a session with no bound `app.user_id` sees no reviews. No API response or repository result SHALL expose a review belonging to a different owner scope; an identifier from another owner scope SHALL behave as non-existent (`404`). The reviewer SHALL always be the requesting `{userId}`; a user SHALL NOT be able to approve or reject a review outside their visibility.

#### Scenario: Review routes are path-tenant and header-tenancy-free

- **WHEN** a client calls `GET /api/users/alice/ingest/reviews`
- **THEN** the request is handled under the `/api/users/{userId}` path-tenancy group, resolved by the existing user middleware from the path, with no tenancy header consulted

#### Scenario: Malformed userId is rejected with 400

- **WHEN** a client calls any review endpoint with an empty, over-length, or non-`[A-Za-z0-9_-]` `{userId}`
- **THEN** the response is `400 Bad Request` and no review query is issued

#### Scenario: Unregistered userId is rejected with 404

- **WHEN** a client calls any review endpoint with a well-formed `{userId}` that is not a row in the `users` registry
- **THEN** the response is `404 Not Found` and no review query is issued

#### Scenario: Only the requester's reviews are visible

- **WHEN** user `alice` lists reviews and her scope includes one personal review and one review in a household she is a member of
- **THEN** both are visible to her and no review outside her visibility is returned

#### Scenario: Another owner's personal review is never returned

- **WHEN** user `alice` requests a review owned by `bob` (a different owner, and alice is not a member of any household owning it)
- **THEN** the response is `404 Not Found` and no field of bob's review is exposed

#### Scenario: An unbound session sees no reviews (RLS backstop)

- **WHEN** a review query is executed in a database session with no bound `app.user_id`
- **THEN** zero review rows are returned regardless of the data present

#### Scenario: A member can act on a household review they can see

- **WHEN** user `bob` is a member of household `h1` and a `pending` review has `owner_id = 'alice'` and `owner_household_id = 'h1'`
- **THEN** bob can read and approve or reject that review, and the decision is recorded with bob as the deciding user

### Requirement: OpenAPI source-of-truth coverage

All review operations and the review schema SHALL be added to the source-of-truth OpenAPI 3.1 document (`procrastinator-backend/api/openapi.yaml`) and SHALL be reflected in the generated server types, preserving the existing 1:1 routes↔operations lockstep. The document SHALL define `ingest_review` (with the `state` enum, `confidence`, extracted candidate fields, Source filename/upload metadata, and the optional best-matched Asset), and every non-2xx response of the review operations SHALL reference the standard `error` envelope. The `asset` and `document` schemas SHALL gain an optional, nullable `confidence` field.

#### Scenario: Review operations are documented

- **WHEN** the OpenAPI document is inspected for the review endpoints
- **THEN** `GET /api/users/{userId}/ingest/reviews`, `GET /api/users/{userId}/ingest/reviews/{id}`, `POST /api/users/{userId}/ingest/reviews/{id}/approve`, and `POST /api/users/{userId}/ingest/reviews/{id}/reject` are present as operations

#### Scenario: The review schema and confidence fields are present

- **WHEN** the OpenAPI document's `components.schemas` is inspected
- **THEN** `ingest_review` is defined and referenced by the review operations, and the `asset` and `document` schemas include an optional nullable `confidence` field

#### Scenario: Routes and operations stay in lockstep

- **WHEN** the backend router's registered routes are compared against the operations in the document
- **THEN** each new review route corresponds to exactly one documented operation and no undocumented review route exists
