# financial-ledger

## Purpose

Defines the financial-ledger capability: financial accounts, money movements, derived balances, and movement–document links, scoped per owner.

## Requirements

### Requirement: Financial account model

The system SHALL persist Financial Accounts scoped per owner (`owner_id` with a nullable `owner_household_id`). A Financial Account SHALL have: an opaque identifier, a non-empty display name, an account type from the controlled vocabulary {`bank`, `wallet`, `cash`, `credit_card`}, an ISO 4217 currency assigned at creation, optional institution name and optional external account descriptor (e.g., masked account number) retained as descriptive metadata, and creation/update timestamps. The currency SHALL be immutable after creation. Account names SHALL NOT be treated as identity; two accounts may share a name.

#### Scenario: Account persists its full field set

- **WHEN** an account is created with name "HDFC Savings", type `bank`, currency `INR`, institution "HDFC Bank", and descriptor "XX1234"
- **THEN** reading the account back returns the same name, type, currency, institution, and descriptor

#### Scenario: Unknown account type is rejected

- **WHEN** an account creation is requested with type `investment`
- **THEN** the request is rejected and no account is persisted

#### Scenario: Currency is immutable

- **WHEN** a client attempts to change an existing account's currency from `INR` to `USD`
- **THEN** the request is rejected and the account's currency remains `INR`

#### Scenario: Duplicate names are allowed

- **WHEN** two accounts are created with the same name "Cash" and type `cash`
- **THEN** two distinct accounts with distinct identifiers exist

### Requirement: Financial account API

The system SHALL expose `POST /api/users/{userId}/finance/accounts` returning `201 Created` with the created account JSON, `GET /api/users/{userId}/finance/accounts` returning `200 OK` with a JSON array of the owner's accounts ordered by creation timestamp ascending with ties broken by identifier ascending, and `GET /api/users/{userId}/finance/accounts/{id}` returning `200 OK` with the account JSON including its current derived balance, or `404 Not Found` for an unknown identifier. Invalid creation bodies SHALL be rejected with `400 Bad Request`.

#### Scenario: Create and read back an account

- **WHEN** a client POSTs a valid account to `/api/users/{userId}/finance/accounts` and then requests `GET /api/users/{userId}/finance/accounts/{id}` with the returned identifier
- **THEN** the read response is `200 OK` and includes the created fields and a derived balance of `0`

#### Scenario: Empty ledger returns an empty array

- **WHEN** no accounts exist and a client requests `GET /api/users/{userId}/finance/accounts`
- **THEN** the response is `200 OK` with body `[]`

#### Scenario: Unknown account returns 404

- **WHEN** a client requests `GET /api/users/{userId}/finance/accounts/{id}` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

### Requirement: Money movement model

The system SHALL persist Money Movements scoped per owner (`owner_id` with a nullable `owner_household_id`). A Money Movement SHALL have: an opaque identifier; a kind from the controlled vocabulary {`expense`, `income`, `transfer`}; an exact-decimal amount strictly greater than zero; an ISO 4217 currency; an `occurred_on` calendar date on which the movement happened in the real world; a `recorded_at` timestamp of when the system persisted it; a non-empty counterparty description; an origin of `manual` or `import`; and account references constrained by kind. Kind constraints: an `expense` SHALL reference a source account and no destination account (money leaves to an external counterparty); an `income` SHALL reference a destination account and no source account (money enters from an external counterparty); a `transfer` SHALL reference both a source and a destination account belonging to the same owner scope, which SHALL be distinct. The movement's currency SHALL equal the currency of every account it references; transfers across currencies are not representable.

#### Scenario: Expense movement is persisted

- **WHEN** an `expense` movement of `1250.50 INR` occurring on `2026-08-20` with description "Reliance Digital" is created against account A
- **THEN** reading the movement back returns the same kind, amount, currency, occurred date, description, source account A, and no destination account

#### Scenario: Transfer requires two distinct same-currency accounts

- **WHEN** a `transfer` is created from account A to account A, or from an `INR` account to a `USD` account
- **THEN** the request is rejected and no movement is persisted

#### Scenario: Non-positive amount is rejected

- **WHEN** a movement is created with amount `0` or `-50`
- **THEN** the request is rejected and no movement is persisted

#### Scenario: Blank description is rejected

- **WHEN** a movement is created with a description that is empty or only whitespace
- **THEN** the request is rejected and no movement is persisted

#### Scenario: Income references destination only

- **WHEN** an `income` movement of `75000 INR` is created into account A
- **THEN** the movement has destination account A and no source account

### Requirement: Money representation

Movement amounts and derived balances SHALL be stored and transmitted as exact decimal values with an ISO 4217 currency code. Binary floating-point SHALL NOT be used for monetary values.

#### Scenario: Amount round-trips exactly

- **WHEN** a movement is created with amount `19999.99` and currency `INR`
- **THEN** reading the movement returns amount `19999.99` and currency `INR` without floating-point drift

### Requirement: Manual money movement API

The system SHALL expose `POST /api/users/{userId}/finance/movements` creating a movement with origin `manual` and returning `201 Created` with the movement JSON; `GET /api/users/{userId}/finance/movements` returning `200 OK` with the owner's movements ordered by creation timestamp ascending with ties broken by identifier ascending, supporting optional filtering by account identifier and by `occurred_on` date range; and `GET /api/users/{userId}/finance/movements/{id}` returning `200 OK` or `404 Not Found`. Validation failures SHALL return `400 Bad Request`; a reference to an unknown account SHALL return `404 Not Found` and persist nothing.

#### Scenario: Manual movement is created with manual origin

- **WHEN** a client POSTs a valid expense movement to `/api/users/{userId}/finance/movements`
- **THEN** the response is `201 Created`, the body shows origin `manual`, and the account's derived balance reflects the movement

#### Scenario: List filters by account

- **WHEN** movements exist on accounts A and B and a client requests `GET /api/users/{userId}/finance/movements?account_id={A}`
- **THEN** the response is `200 OK` containing only movements referencing account A

#### Scenario: List filters by occurred date range

- **WHEN** movements exist on `2026-08-01` and `2026-08-20` and a client requests movements from `2026-08-10` to `2026-08-31`
- **THEN** only the `2026-08-20` movement is returned

#### Scenario: Movement referencing an unknown account is rejected

- **WHEN** a client POSTs a movement referencing a non-existent account
- **THEN** the response is `404 Not Found` and no movement is persisted

### Requirement: Derived balance

Each account's current balance SHALL be derived from its money movements: `income` into the account and the incoming leg of a `transfer` increase it by the movement amount; `expense` out of the account and the outgoing leg of a `transfer` decrease it by the movement amount. An account with no movements SHALL have balance `0`. Balances SHALL NOT be independently authoritative: they SHALL NOT be directly set or edited through any API operation.

#### Scenario: Balance accumulates movements

- **WHEN** account A has an income of `250` and an expense of `100`
- **THEN** `GET /api/users/{userId}/finance/accounts/{A}` reports a derived balance of `150`

#### Scenario: Transfer moves value between accounts

- **WHEN** a transfer of `100` exists from account A to account B and neither has other movements
- **THEN** A's derived balance is `-100` and B's derived balance is `100`

#### Scenario: Balance cannot be set directly

- **WHEN** a client attempts to set or modify an account's balance through any account or movement endpoint
- **THEN** the request is rejected and the derived balance is unchanged

### Requirement: Movement provenance

Every movement SHALL record its origin. A movement created through the manual movement API SHALL record origin `manual`. A movement created by a statement-import commit SHALL record origin `import` together with its import provenance: the import batch identifier and the statement line reference it was created from — the import batch, statement line reference, and retained statement Source are defined by the `statement-import` capability of this change — and, when the statement line provides one, the external reference identifier used for duplicate detection. Reading an imported movement SHALL expose origin `import` with the fields `import_batch_id` (opaque identifier), `import_line` (statement line reference), and `external_reference` (present only when the statement line provided one); the movement SHALL remain traceable to the import batch's retained Source. A movement with origin `manual` SHALL expose no import provenance fields.

#### Scenario: Imported movement exposes its provenance

- **WHEN** a movement was created by an import batch commit and a client requests `GET /api/users/{userId}/finance/movements/{id}`
- **THEN** the response shows origin `import` with `import_batch_id` and `import_line` identifying the batch and statement line it was created from

#### Scenario: External reference is retained when the statement provides one

- **WHEN** a movement was created by an import batch commit from a statement line carrying an external reference identifier and a client requests `GET /api/users/{userId}/finance/movements/{id}`
- **THEN** the response shows `external_reference` with that identifier

#### Scenario: Manual movement shows manual origin

- **WHEN** a client reads a movement created via `POST /api/users/{userId}/finance/movements`
- **THEN** the response shows origin `manual` and no import provenance fields

### Requirement: Movement–document link creation

A movement MAY be linked to at most one existing Document of the same owner scope (for example a receipt or invoice captured earlier via document ingestion), and a Document SHALL be linked from at most one movement at a time. The system SHALL expose `POST /api/users/{userId}/finance/movements/{id}/link` accepting a JSON body with a `document_id` field naming the link target. On success the endpoint SHALL respond `200 OK` with the updated movement JSON and the link SHALL record creator kind `manual`. Repeating the same link (same movement, same document) SHALL be an idempotent no-op returning `200 OK`. Linking an already-linked movement to a different document SHALL be rejected with `409 Conflict`; linking a document that is already linked to another movement SHALL be rejected with `409 Conflict`. An unknown movement, an unknown document, or a document belonging to another owner SHALL yield `404 Not Found` and create no link.

#### Scenario: Manual link is created and visible

- **WHEN** a captured receipt Document exists and a client POSTs its identifier to `/api/users/{userId}/finance/movements/{id}/link` of an unlinked movement
- **THEN** the response is `200 OK`, reading the movement shows the link with creator kind `manual`, and neither the movement nor the Document fields are changed

#### Scenario: Repeating the same link is an idempotent no-op

- **WHEN** a client POSTs a `document_id` to `/api/users/{userId}/finance/movements/{id}/link` of a movement already linked to that same document
- **THEN** the response is `200 OK` and exactly one link exists between the movement and the document

#### Scenario: Already-linked movement rejects a second link

- **WHEN** a client attempts to link a different Document to a movement that is already linked
- **THEN** the request is rejected with `409 Conflict` and the existing link is unchanged

#### Scenario: Already-linked document rejects a second movement

- **WHEN** a client attempts to link a Document that is already linked to movement M1 to a different movement M2
- **THEN** the request is rejected with `409 Conflict` and the existing link is unchanged

#### Scenario: Cross-owner link target is invisible

- **WHEN** a client attempts to link a movement to a Document identifier belonging to another owner
- **THEN** the response is `404 Not Found` and no link is created

### Requirement: Movement–document link removal

The system SHALL expose `DELETE /api/users/{userId}/finance/movements/{id}/link` which removes the movement's link if one exists and responds `204 No Content`. The operation SHALL be idempotent: unlinking a movement that has no link SHALL also succeed with `204 No Content` as a no-op. An unknown movement identifier, including one belonging to another owner, SHALL yield `404 Not Found`.

#### Scenario: Unlink removes the link

- **WHEN** a client DELETEs `/api/users/{userId}/finance/movements/{id}/link` of a linked movement
- **THEN** the response is `204 No Content`, reading the movement shows no link, and the previously linked Document is linkable again

#### Scenario: Unlinking an unlinked movement is an idempotent no-op

- **WHEN** a client DELETEs `/api/users/{userId}/finance/movements/{id}/link` of a movement that has no link
- **THEN** the response is `204 No Content` and no state changes

### Requirement: Link metadata and non-modification

Every movement–document link SHALL record whether it was created `manual`ly by a client or `auto`matically by the statement-import pipeline, and reading a movement SHALL expose its link together with the creator kind. Creating or removing a link SHALL NOT modify the movement's amount, currency, occurred date, kind, or accounts, and SHALL NOT modify the linked Document.

#### Scenario: Auto-created link exposes its creator kind

- **WHEN** a movement was linked to a Document automatically during an import batch commit and a client requests `GET /api/users/{userId}/finance/movements/{id}`
- **THEN** the response shows the link with creator kind `auto`

#### Scenario: Link and unlink leave both sides unchanged

- **WHEN** a client links a movement to a Document and later unlinks it
- **THEN** the movement's amount, currency, occurred date, kind, and accounts and the Document's fields are byte-for-byte unchanged throughout

### Requirement: Link conflict retention

When a linked Document carries an extracted price or currency that disagrees with the movement's amount or currency, the system SHALL retain both values unchanged and SHALL mark the link as conflicting. The system SHALL NOT overwrite the movement with the document's values, SHALL NOT overwrite the document with the movement's values, and SHALL NOT silently remove the link.

#### Scenario: Disagreeing values are retained and flagged

- **WHEN** a movement of `40000 INR` is linked to a Document whose extracted price is `39500 INR`
- **THEN** the movement remains `40000 INR`, the Document's extracted price remains `39500 INR`, and the link is marked conflicting

#### Scenario: Agreeing values produce no conflict

- **WHEN** a movement of `40000 INR` is linked to a Document whose extracted price is `40000 INR`
- **THEN** the link is created and not marked conflicting

### Requirement: Movement field immutability

A persisted movement's amount, currency, `occurred_on`, kind, and account references SHALL NOT be editable through any API operation. A request attempting to modify any of them — including via `PATCH /api/users/{userId}/finance/movements/{id}` carrying any of those fields — SHALL be rejected with `400 Bad Request`, and the movement SHALL remain unchanged. Only the description and the document link MAY change after persistence.

#### Scenario: Core fields of a persisted movement cannot be edited

- **WHEN** a client PATCHes a persisted movement's amount, currency, `occurred_on`, kind, or account references
- **THEN** the response is `400 Bad Request` and the movement is unchanged

### Requirement: Manual movement lifecycle

The system SHALL expose `PATCH /api/users/{userId}/finance/movements/{id}` accepting a JSON body with a `description` field. On success it SHALL respond `200 OK` with the updated movement JSON. A blank or whitespace-only description SHALL be rejected with `400 Bad Request`; an unknown movement identifier SHALL yield `404 Not Found`. Description correction SHALL be available for movements of any origin. The system SHALL expose `DELETE /api/users/{userId}/finance/movements/{id}` which deletes a movement with origin `manual` and responds `204 No Content`; deletion SHALL update derived balances accordingly; an unknown movement identifier SHALL yield `404 Not Found`.

#### Scenario: Description correction succeeds

- **WHEN** a client PATCHes a persisted movement's description from "REL DIG 123" to "Reliance Digital"
- **THEN** the response is `200 OK` and reading the movement returns the corrected description with all other fields unchanged

#### Scenario: Blank description correction is rejected

- **WHEN** a client PATCHes a persisted movement's description to an empty or whitespace-only value
- **THEN** the response is `400 Bad Request` and the movement's description is unchanged

#### Scenario: Manual movement can be deleted

- **WHEN** a client DELETEs a manual movement on an account whose only movement it is
- **THEN** the response is `204 No Content`, the movement no longer exists, and the account's derived balance returns to `0`

#### Scenario: Deleting an unknown movement returns 404

- **WHEN** a client DELETEs `/api/users/{userId}/finance/movements/{id}` for an identifier that does not exist
- **THEN** the response is `404 Not Found`

### Requirement: Imported movement lifecycle constraints

A movement with origin `import` SHALL NOT be individually deleted or amount-corrected: `DELETE /api/users/{userId}/finance/movements/{id}` for an imported movement SHALL be rejected with `409 Conflict` and the movement SHALL remain unchanged. Its lifecycle is owned by its import batch; rollback of committed batches and reversal or correction of committed imported movements are out of scope for this change.

#### Scenario: Imported movement cannot be individually deleted

- **WHEN** a client DELETEs a movement with origin `import`
- **THEN** the response is `409 Conflict` and the movement is unchanged

### Requirement: Owner scoping of ledger data

All ledger records (financial accounts, money movements, and their document links) SHALL be owner-scoped: every ledger table SHALL carry an `owner_id` (NOT NULL) and a nullable `owner_household_id`, every persistence operation SHALL resolve the owner from the explicit owner option or the request context and SHALL restrict reads and writes by the owner visibility rule (the owner's own rows plus rows in households the user belongs to), and a persistence operation with no resolvable owner SHALL fail closed with an error rather than operate across owners. No API response SHALL expose another owner's ledger data; account and movement identifiers of another owner SHALL behave as non-existent.

#### Scenario: Same account name in two owners yields two accounts

- **WHEN** owner `acme` and owner `globex` each create an account named "Cash"
- **THEN** two distinct accounts exist and each owner's reads see only its own account

#### Scenario: Other owner's account id is not found

- **WHEN** owner `acme` requests `GET /api/users/{userId}/finance/accounts/{id}` for an account created under owner `globex`
- **THEN** the response is `404 Not Found`

#### Scenario: Persistence without owner fails closed

- **WHEN** a ledger persistence operation is invoked with neither an explicit owner option nor a context owner
- **THEN** it returns an error and performs no query
