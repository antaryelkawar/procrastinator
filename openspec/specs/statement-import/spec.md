# Spec: statement-import

Delta for change `statement-ledger-ingestion`. Introduces the new `statement-import` capability: the Import Batch lifecycle for bank/wallet statement files. Movements created here are governed by the `financial-ledger` capability of the same change; Sources and Documents referenced here are defined by the `document-ingestion` capability.

## ADDED Requirements

### Requirement: Import batch model

The system SHALL persist Import Batches scoped per tenant. An Import Batch SHALL have: an opaque identifier; a lifecycle state from the controlled vocabulary {`preview`, `committed`, `discarded`}; a reference to the target Financial Account the statement describes; a reference to the retained Source of the uploaded statement file; the original filename; the detected statement format (`csv` or `pdf`); per-status line counts; and creation/update timestamps. The lifecycle SHALL be a one-way state machine: a batch is created in `preview` and may transition exactly once to `committed` (via commit) or to `discarded` (via discard); `committed` and `discarded` are terminal states. Every parsed statement line SHALL be persisted as part of the batch with: a stable statement line reference (its 1-based position within the parsed statement), the raw line content, the extracted fields (occurred date, signed amount, description, and optional external reference identifier), a line status from the controlled vocabulary {`valid`, `duplicate`, `possible-duplicate`, `error`}, and, for `error` lines, a human-readable reason.

#### Scenario: Uploaded batch starts in preview with its lines

- **WHEN** a statement file is successfully uploaded
- **THEN** the persisted batch is in state `preview`, references its target account and retained Source, and carries one persisted line per parsed statement line, each with a line reference and a status

#### Scenario: Terminal states do not transition

- **WHEN** a batch is in state `committed` or `discarded`
- **THEN** no operation transitions it to any other state

### Requirement: Statement upload endpoint

The system SHALL expose `POST /api/finance/import-batches` accepting `multipart/form-data` with the uploaded statement file in a form field named `file` and the target account identifier in a form field named `account_id`. The endpoint SHALL process the upload synchronously — retain the Source, parse the statement, persist the batch and its lines in state `preview` — before responding `201 Created` with a JSON body representing the batch including its lines and their statuses. The system SHALL accept only CSV (`text/csv`) and PDF (`application/pdf`) uploads, determined by content sniffing of the file bytes rather than the client-supplied Content-Type header alone; other types SHALL be rejected with `415 Unsupported Media Type`. Uploads larger than a configurable size limit (default 50 MiB) SHALL be rejected with `413 Payload Too Large`. A missing `file` field or a missing/blank `account_id` SHALL be rejected with `400 Bad Request`. An unknown target account SHALL yield `404 Not Found`. A syntactically accepted file that yields no parseable statement lines — including an image-only/scanned PDF with no text layer (OCR is out of scope) — SHALL be rejected with `422 Unprocessable Entity`; in all rejection cases no Import Batch SHALL be persisted.

#### Scenario: Successful CSV upload returns a preview batch

- **WHEN** a client POSTs a valid bank statement CSV as multipart field `file` with a valid `account_id` to `/api/finance/import-batches`
- **THEN** the response is `201 Created` with the batch in state `preview`, and the batch's lines carry per-line statuses

#### Scenario: Unsupported file type is rejected

- **WHEN** a client uploads a file whose sniffed content type is not CSV or PDF
- **THEN** the response is `415 Unsupported Media Type` and no batch is persisted

#### Scenario: Oversized upload is rejected

- **WHEN** a client uploads a file exceeding the configured size limit
- **THEN** the response is `413 Payload Too Large` and no batch is persisted

#### Scenario: Upload for an unknown account is rejected

- **WHEN** a client uploads a valid CSV with an `account_id` that does not exist
- **THEN** the response is `404 Not Found` and no batch is persisted

#### Scenario: Image-only PDF is rejected as unprocessable

- **WHEN** a client uploads a scanned PDF with no text layer
- **THEN** the response is `422 Unprocessable Entity` and no batch is persisted

### Requirement: Statement source retention

Every accepted upload SHALL be retained as a Source record before parsing, with the same retention semantics as document ingestion: the raw file bytes SHALL be written to a configurable storage directory, and the Source record SHALL persist the original filename, sniffed content type, byte size, storage path, SHA-256 digest of the bytes, and the upload timestamp. A Source SHALL be retained even if parsing subsequently fails or the upload is rejected as unprocessable, and SHALL be retained for the lifetime of the batch, including after the batch is committed or discarded. Reading an import batch SHALL expose its Source reference so that every committed movement remains traceable to the original statement file.

#### Scenario: Source survives parse failure

- **WHEN** an upload is accepted but parsing fails
- **THEN** a Source record for the upload exists with its stored bytes intact and retrievable from the recorded storage path, and no batch is persisted

#### Scenario: Committed batch still exposes its Source

- **WHEN** a batch has been committed and a client requests `GET /api/finance/import-batches/{id}`
- **THEN** the response references the retained Source of the original statement file

### Requirement: Statement parsing and per-line preview

Parsing SHALL extract for each statement line: the occurred date, the signed amount with its direction relative to the target account (money out vs money in), a counterparty description, and, when the statement provides one, an external reference identifier. A line from which date, amount, and description cannot all be extracted SHALL be marked `error` with a reason and SHALL NOT block the remaining lines. Parsing and preview SHALL NOT create, modify, or delete any Money Movement and SHALL NOT change any derived balance; canonical writes happen only at commit. The batch and its per-line preview SHALL be retrievable via `GET /api/finance/import-batches/{id}` until the batch is discarded or committed, and remain retrievable afterwards.

#### Scenario: Preview creates no movements

- **WHEN** a statement has been uploaded and its batch is in state `preview`
- **THEN** no movements exist from that batch and the target account's derived balance is unchanged

#### Scenario: Unparseable line is marked error with a reason

- **WHEN** a statement contains a line whose date cannot be parsed
- **THEN** that line has status `error` with a human-readable reason and the remaining lines are classified normally

#### Scenario: Preview is re-readable

- **WHEN** a client requests `GET /api/finance/import-batches/{id}` for a batch in state `preview`
- **THEN** the response is `200 OK` with the batch and every line's line reference, extracted fields, and status

### Requirement: Deterministic duplicate detection

Each parsed line SHALL be classified deterministically before the batch is persisted. A line SHALL be classified `duplicate` when it carries an external reference identifier equal to the external reference of an already-committed movement on the same target account and tenant, or when it duplicates an earlier line within the same batch. A line that is not a `duplicate` SHALL be classified `possible-duplicate` when its content fingerprint — the tuple of occurred date, exact amount, and normalized description (case-insensitive, with whitespace collapsed) — equals the content fingerprint of an existing movement on the same target account and tenant of any origin. All other fully parsed lines SHALL be classified `valid`. Classification SHALL be deterministic: the same statement file against the same ledger state SHALL produce the same per-line statuses, and re-importing a previously committed statement SHALL classify every re-imported line as `duplicate`.

#### Scenario: External reference match is a duplicate

- **WHEN** a statement line carries external reference `TXN-123` and a committed movement on the same account already has external reference `TXN-123`
- **THEN** the line is classified `duplicate`

#### Scenario: Content fingerprint match is a possible duplicate

- **WHEN** a statement line has occurred date `2026-08-20`, amount `1250.50`, and description "Reliance Digital", and a manual movement with the same date, amount, and a description differing only in case and spacing exists on the same account
- **THEN** the line is classified `possible-duplicate`

#### Scenario: Repeated line within one batch is a duplicate

- **WHEN** the same external reference or the same content fingerprint appears twice in one statement file
- **THEN** the first occurrence is classified on its own merits and each later occurrence is classified `duplicate`

#### Scenario: Re-importing a committed statement yields all duplicates

- **WHEN** a statement file was committed and the identical file is uploaded again for the same account
- **THEN** every line of the new batch is classified `duplicate` and committing it creates no movements

### Requirement: Import batch commit

The system SHALL expose `POST /api/finance/import-batches/{id}/commit`. Committing a batch in state `preview` SHALL atomically create one Money Movement per `valid` line — either all of them or none — and transition the batch to `committed`, responding `200 OK` with a JSON summary of created and skipped line counts. Lines with status `duplicate`, `possible-duplicate`, or `error` SHALL be skipped and SHALL NOT create movements; because imported movements cannot be individually deleted in this change, classification alone decides inclusion, and a client wanting a skipped line SHALL add it via the manual movement API. Each created movement SHALL have: origin `import`; kind `expense` when the line is money out of the target account and `income` when money in (imports never create transfers); the target account as source (expense) or destination (income); the line's exact amount and occurred date as the movement's amount and `occurred_on`; the account's currency; the line's description; the external reference identifier when the line provides one; and the batch identifier and statement line reference as provenance. Commit SHALL be idempotent: committing an already-`committed` batch SHALL respond `200 OK` with the same summary and create no additional movements. Committing a `discarded` batch SHALL be rejected with `409 Conflict`. Committing a batch with zero `valid` lines SHALL succeed, transition the batch to `committed`, and create zero movements.

#### Scenario: Commit creates movements for valid lines only

- **WHEN** a batch in `preview` has 8 `valid`, 1 `duplicate`, and 1 `error` line and a client POSTs to its commit endpoint
- **THEN** the response is `200 OK`, exactly 8 movements with origin `import` exist carrying the batch identifier and their line references, and the target account's derived balance reflects exactly those 8 movements

#### Scenario: Commit is atomic

- **WHEN** a failure occurs while committing a batch
- **THEN** no movement from that batch exists and the batch is not in state `committed`

#### Scenario: Re-commit is an idempotent no-op

- **WHEN** a client POSTs to the commit endpoint of an already-`committed` batch
- **THEN** the response is `200 OK` with the same summary and the number of movements from that batch is unchanged

#### Scenario: Committing a discarded batch is rejected

- **WHEN** a client POSTs to the commit endpoint of a `discarded` batch
- **THEN** the response is `409 Conflict` and no movements are created

### Requirement: Import batch discard

The system SHALL expose `POST /api/finance/import-batches/{id}/discard`. Discarding a batch in state `preview` SHALL transition it to `discarded` and respond `200 OK` with the batch JSON. Discard SHALL be idempotent: discarding an already-`discarded` batch SHALL respond `200 OK` as a no-op. Discarding a `committed` batch SHALL be rejected with `409 Conflict`. A discarded batch SHALL retain its persisted lines and its retained Source, and SHALL never be committable.

#### Scenario: Preview batch is discarded

- **WHEN** a client POSTs to the discard endpoint of a batch in state `preview`
- **THEN** the response is `200 OK`, the batch is in state `discarded`, its lines and Source remain readable, and no movements exist from it

#### Scenario: Re-discard is an idempotent no-op

- **WHEN** a client POSTs to the discard endpoint of an already-`discarded` batch
- **THEN** the response is `200 OK` and the batch remains `discarded`

#### Scenario: Committed batch cannot be discarded

- **WHEN** a client POSTs to the discard endpoint of a `committed` batch
- **THEN** the response is `409 Conflict` and the batch remains `committed`

### Requirement: Auto-linking of committed movements

At commit, the system SHALL deterministically attempt to link each newly created movement to an existing captured Document. A Document SHALL be a link candidate when it belongs to the same tenant, is currently linked to no movement, and carries an extracted price whose amount and currency are both present and exactly equal to the movement's amount and currency. When exactly one candidate exists, the system SHALL create the link with creator kind `auto`. When zero or more than one candidate exists, the system SHALL create no link for that movement. Auto-linking SHALL NOT modify the movement's amount, currency, occurred date, or accounts, and SHALL NOT modify any Document.

#### Scenario: Exactly one matching document is auto-linked

- **WHEN** a commit creates a movement of `40000 INR` and exactly one unlinked Document of the same tenant has extracted price `40000 INR`
- **THEN** the movement is linked to that Document with creator kind `auto`

#### Scenario: Multiple matching documents produce no link

- **WHEN** a commit creates a movement of `40000 INR` and two unlinked Documents of the same tenant each have extracted price `40000 INR`
- **THEN** no link is created for that movement

#### Scenario: Already-linked document is not a candidate

- **WHEN** a commit creates a movement of `40000 INR` and the only Document with extracted price `40000 INR` is already linked to another movement
- **THEN** no link is created for that movement

### Requirement: Import batch list and read API

The system SHALL expose `GET /api/finance/import-batches` returning `200 OK` with a JSON array of the tenant's import batches ordered by creation timestamp ascending with ties broken by identifier ascending, and `GET /api/finance/import-batches/{id}` returning `200 OK` with the batch JSON including its state, per-status line counts, Source reference, and its lines with their statuses, or `404 Not Found` for an unknown identifier. An empty import history SHALL return `200 OK` with body `[]`.

#### Scenario: Empty import history returns an empty array

- **WHEN** no import batches exist and a client requests `GET /api/finance/import-batches`
- **THEN** the response is `200 OK` with body `[]`

#### Scenario: Batch read returns state, counts, and lines

- **WHEN** a client requests `GET /api/finance/import-batches/{id}` for an existing batch
- **THEN** the response is `200 OK` with the batch's state, per-status line counts, Source reference, and every line with its status

#### Scenario: Unknown batch returns 404

- **WHEN** a client requests `GET /api/finance/import-batches/{id}` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

### Requirement: Ingestion bounds

Statement ingestion SHALL enforce measurable resource bounds: the uploaded file size SHALL NOT exceed a configurable limit (default 50 MiB); a single batch SHALL contain at most 100 000 statement lines, and an upload whose parsed line count exceeds that limit SHALL be rejected with `422 Unprocessable Entity`, retaining the Source but persisting no batch; and parsing and preview classification for any file within these bounds SHALL complete within 60 seconds.

#### Scenario: Batch exceeding the line limit is rejected

- **WHEN** a client uploads a CSV with more than 100 000 statement lines
- **THEN** the response is `422 Unprocessable Entity`, the Source is retained, and no batch is persisted

#### Scenario: In-bounds file parses within the time bound

- **WHEN** a client uploads a 100 000-line CSV within the size limit
- **THEN** the upload responds with the preview batch within 60 seconds

### Requirement: Tenant scoping of import data

All import records (import batches and their lines) SHALL be tenant-scoped: every import table SHALL carry a `tenant_id`, every persistence operation SHALL resolve the tenant from the explicit tenant option or the request context and SHALL restrict reads and writes to that tenant, and a persistence operation with no resolvable tenant SHALL fail closed with an error rather than operate across tenants. No API response SHALL expose another tenant's import data; batch identifiers of another tenant SHALL behave as non-existent, and duplicate detection SHALL consider only movements of the uploading tenant.

#### Scenario: Other tenant's batch id is not found

- **WHEN** tenant `acme` requests `GET /api/finance/import-batches/{id}` for a batch created under tenant `globex`
- **THEN** the response is `404 Not Found`

#### Scenario: Duplicate detection ignores other tenants' movements

- **WHEN** tenant `globex` has a committed movement with external reference `TXN-123` on an account and tenant `acme` uploads a statement line with the same external reference `TXN-123` for its own account
- **THEN** the line is not classified `duplicate` on account of globex's movement

#### Scenario: Persistence without tenant fails closed

- **WHEN** an import persistence operation is invoked with neither an explicit tenant option nor a context tenant
- **THEN** it returns an error and performs no query
