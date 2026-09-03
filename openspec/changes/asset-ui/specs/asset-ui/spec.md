# Spec: asset-ui

Delta for change `asset-ui`. Introduces the new `asset-ui` capability: the responsive web frontend for the Procrastinator personal finance app. All backend behavior referenced here is defined by the `document-ingestion`, `asset-registry`, `financial-ledger`, `statement-import`, and `multitenancy` capabilities and is not re-specified here.

## ADDED Requirements

### Requirement: Single-page application shell

The system SHALL provide a single-page web application served as a static build, with client-side routes for document upload, asset list, asset detail, finance accounts, money movements, and statement import. The application SHALL be usable on desktop and phone-sized viewports from the same build: at phone width, primary navigation SHALL remain reachable and all lists and forms SHALL remain fully usable without horizontal scrolling of the page. The application SHALL render in modern evergreen browsers without requiring browser plugins.

#### Scenario: Every screen is reachable by navigation

- **WHEN** a user opens the application and uses the primary navigation
- **THEN** they can reach the document upload, asset list, finance accounts, money movements, and statement import screens without a full page reload

#### Scenario: Deep link opens the matching screen

- **WHEN** a user opens the URL of an existing asset's detail page directly in a new browser tab
- **THEN** the application loads and displays that asset's detail screen

#### Scenario: Phone-width layout stays usable

- **WHEN** the application is viewed at a 360px-wide viewport
- **THEN** navigation, lists, and forms are fully operable and no page-level horizontal scrollbar appears

### Requirement: Active user context

The application SHALL operate under an explicit active user (tenant) whose identifier is visible in the UI at all times. Every API request the application issues SHALL be scoped to the active user via the `/api/users/{userId}/…` URL path per the `multitenancy` capability. Changing the active user SHALL re-fetch all displayed data under the new user and SHALL NOT display data fetched under the previous user.

#### Scenario: Requests are scoped to the active user

- **WHEN** the active user is `alice` and the application loads the asset list
- **THEN** the underlying request targets `/api/users/alice/assets` and no request is issued under any other user identifier

#### Scenario: Switching the active user replaces all displayed data

- **WHEN** the active user is switched from `alice` to `bob`
- **THEN** every screen subsequently renders only data fetched under user `bob`, with no stale records from `alice` shown

### Requirement: API data layer

All server data SHALL be fetched through a centralized data layer that caches read responses, deduplicates concurrent identical requests, shows a loading indicator while a screen's data is in flight, and invalidates affected cached data after any successful mutation. A failed request SHALL surface an error state on the screen and SHALL NOT leave the previous data silently replaced by nothing.

#### Scenario: Mutation refreshes dependent views

- **WHEN** a user successfully uploads a document that creates an asset and then navigates to the asset list
- **THEN** the asset list shows the new asset without requiring a manual page reload

#### Scenario: Failed load shows an error, not a blank screen

- **WHEN** the asset list request fails with a server error
- **THEN** the screen shows an error indicator with a retry action and does not render an empty list as if no assets existed

### Requirement: Document upload screen

The system SHALL provide an upload screen accepting PDF, PNG, and JPEG files via drag-and-drop and via click-to-select, uploading each file as multipart field `file` to `POST /api/documents` for the active user. Each in-flight upload SHALL show a live progress indicator driven by upload progress events. Completed uploads SHALL show a per-file result: success with the resulting asset's identity, or a failure with a human-readable reason mapped from the API status — invalid request (`400`), too large (`413`), unsupported type (`415`), unidentifiable content (`422`), or processing failure (`502`). A failed upload SHALL NOT block or cancel other queued uploads.

#### Scenario: Drag-and-drop upload shows progress then success

- **WHEN** a user drops a valid PDF invoice onto the upload area
- **THEN** the file shows a progress indicator while uploading and, on `201 Created`, a success entry showing the created asset's identity

#### Scenario: Unsupported file reports a reason

- **WHEN** a user uploads a file whose sniffed content type is unsupported
- **THEN** that file's entry shows a failure with a reason indicating the file type is not accepted, and other files continue uploading

#### Scenario: LLM outage surfaces a retryable failure

- **WHEN** an upload fails with `502 Bad Gateway`
- **THEN** the file's entry shows a processing-failure message and offers the user a way to retry that file

### Requirement: Asset list screen

The system SHALL provide an asset list screen showing every asset of the active user from `GET /api/assets`, displaying at least brand, model, serial number, and warranty end when present. Assets with a warranty end in the past SHALL be visually distinguished from assets still under warranty. An empty registry SHALL render an explicit empty state guiding the user to upload a document. Selecting an asset SHALL navigate to its detail screen.

#### Scenario: Assets render with key fields

- **WHEN** two assets exist and the user opens the asset list
- **THEN** both assets are shown with their brand, model, and serial number where present

#### Scenario: Expired warranty is highlighted

- **WHEN** an asset's warranty end is before today's date
- **THEN** that asset is visually marked as out of warranty in the list

#### Scenario: Empty registry shows guidance

- **WHEN** the active user has no assets
- **THEN** the screen shows an empty state with a call to action to upload a document, not a bare blank table

### Requirement: Asset detail screen

The system SHALL provide an asset detail screen for `GET /api/assets/{id}` showing all populated structured fields of the asset — brand, model, serial number, purchase date, warranty end, price with currency — and the asset's documents from `GET /api/assets/{id}/documents`, each entry showing document type, source filename, and upload timestamp. An unknown asset identifier SHALL render a not-found state. Monetary values SHALL be displayed exactly as returned by the API with their currency, and dates SHALL be displayed in an unambiguous locale-independent format.

#### Scenario: Asset fields and documents are shown

- **WHEN** an asset has an invoice and a warranty document and the user opens its detail screen
- **THEN** the asset's populated fields are shown and both documents are listed with their type, filename, and upload timestamp

#### Scenario: Unknown asset shows not-found

- **WHEN** the user navigates to an asset identifier that does not exist for the active user
- **THEN** the screen shows a not-found state with a way back to the asset list

#### Scenario: Price displays exactly

- **WHEN** an asset's price is `39999.99 INR`
- **THEN** the screen displays `39999.99` with its currency, without floating-point artifacts such as `39999.989999…`

### Requirement: Finance accounts screen

The system SHALL provide a finance accounts screen listing every account of the active user from `GET /api/finance/accounts` with its name, type, currency, and current derived balance, and a form to create an account via `POST /api/finance/accounts` with name, type (`bank`, `wallet`, `cash`, `credit_card`), currency, and optional institution and descriptor. Client-side validation SHALL reject an empty name and a missing type or currency before any request is sent. A rejected creation (`400`) SHALL display the server's reason without clearing the form's other fields.

#### Scenario: Accounts render with balances

- **WHEN** an account has an income of `250` and an expense of `100`
- **THEN** the accounts screen shows that account with a balance of `150`

#### Scenario: Invalid creation is caught client-side

- **WHEN** the user submits the create-account form with a blank name
- **THEN** the form shows a validation error and no create request is sent

#### Scenario: Server rejection is surfaced

- **WHEN** account creation fails with `400 Bad Request`
- **THEN** the form displays the failure reason and retains the values the user entered

### Requirement: Money movements screen

The system SHALL provide a money movements screen listing the active user's movements from `GET /api/finance/movements`, each entry showing kind, signed display of amount with currency, occurred date, description, origin (`manual` or `import`), and the account(s) involved. The list SHALL support filtering by account and by occurred-date range using the API's filter parameters. Imported movements SHALL be visually distinguished and SHALL NOT offer delete. Manual movements SHALL offer deletion with a confirmation step. Any movement SHALL offer description correction via `PATCH /api/finance/movements/{id}`.

#### Scenario: List filters by account and date range

- **WHEN** the user selects account A and the date range 2026-08-10 to 2026-08-31
- **THEN** only movements referencing account A within that range are requested and shown

#### Scenario: Imported movement cannot be deleted from the UI

- **WHEN** a movement's origin is `import`
- **THEN** the UI presents no delete action for that movement

#### Scenario: Manual deletion requires confirmation and updates balances

- **WHEN** the user confirms deletion of a manual movement
- **THEN** the movement disappears from the list and the affected account's displayed balance reflects the deletion

#### Scenario: Description correction round-trips

- **WHEN** the user edits a movement's description from "REL DIG 123" to "Reliance Digital"
- **THEN** the list shows the corrected description with amount, date, and kind unchanged

### Requirement: Movement creation form

The system SHALL provide a form to create a manual movement via `POST /api/finance/movements` supporting kinds `expense`, `income`, and `transfer`. The form SHALL require a positive decimal amount, an occurred date, and a non-empty description, and SHALL enforce kind-specific account selection: `expense` selects a source account, `income` a destination account, `transfer` two distinct accounts of the same currency. Client-side validation SHALL catch a non-positive amount, blank description, missing account, identical transfer accounts, and cross-currency transfer before any request is sent. On success the form SHALL reset and the movements list and affected balances SHALL refresh.

#### Scenario: Transfer validation is client-side

- **WHEN** the user selects the same account as transfer source and destination
- **THEN** the form shows a validation error and no create request is sent

#### Scenario: Successful creation refreshes views

- **WHEN** the user creates a valid expense of `1250.50 INR`
- **THEN** the movement appears in the list with origin `manual` and the source account's balance reflects it

### Requirement: Movement–document linking

The system SHALL let the user link an unlinked movement to an existing document via `POST /api/finance/movements/{id}/link` and remove a link via `DELETE /api/finance/movements/{id}/link`. A linked movement SHALL display its linked document's type and filename and the link's creator kind (`manual` or `auto`). A link flagged as conflicting by the API SHALL be visually marked, with both the movement's and the document's values displayed unchanged. A conflict rejection (`409`) SHALL be surfaced as a human-readable message.

#### Scenario: Linked movement shows its document

- **WHEN** a movement is linked to a receipt document
- **THEN** the movement's entry shows the document's type, filename, and the link's creator kind

#### Scenario: Conflicting link is flagged without overwriting

- **WHEN** a movement of `40000 INR` is linked to a document with extracted price `39500 INR`
- **THEN** the link is marked conflicting and both `40000 INR` and `39500 INR` remain displayed as-is

#### Scenario: Link conflict rejection is explained

- **WHEN** the user attempts to link a document that is already linked to another movement
- **THEN** the UI shows a message that the document is already linked and neither link is changed

### Requirement: Statement import screen

The system SHALL provide a statement import flow: the user selects a target account and a CSV or PDF statement file and uploads them as multipart fields `account_id` and `file` to `POST /api/finance/import-batches`. On success the screen SHALL show the resulting preview batch: the original filename, detected format, per-status line counts, and every parsed line with its occurred date, signed amount, description, and status (`valid`, `duplicate`, `possible-duplicate`, `error` with its reason). The user SHALL be able to commit the batch via `POST …/commit` or discard it via `POST …/discard`, each with a confirmation step. After commit, the screen SHALL show the created/skipped summary and the movements list and account balances SHALL refresh. A rejected upload (`400`/`404`/`413`/`415`/`422`) SHALL display a human-readable reason.

#### Scenario: Preview shows per-line statuses

- **WHEN** a statement uploads successfully with 8 valid, 1 duplicate, and 1 error line
- **THEN** the preview shows the per-status counts and lists each line with its status, and the error line shows its reason

#### Scenario: Commit confirms then summarizes

- **WHEN** the user confirms commit of a preview batch
- **THEN** the batch transitions to `committed`, the screen shows the created and skipped counts, and the account's displayed balance reflects exactly the created movements

#### Scenario: Discard removes the batch from the committable flow

- **WHEN** the user confirms discard of a preview batch
- **THEN** the batch is shown as `discarded`, no movements are created from it, and the commit action is no longer offered

#### Scenario: Scanned PDF rejection is explained

- **WHEN** the user uploads an image-only PDF statement
- **THEN** the UI explains that the statement contained no parseable lines and no batch was created

### Requirement: Import history

The system SHALL provide an import history view listing the active user's import batches from `GET /api/finance/import-batches` with filename, target account, state (`preview`, `committed`, `discarded`), per-status line counts, and creation timestamp. Selecting a batch SHALL show its full detail from `GET /api/finance/import-batches/{id}` including its lines. A batch still in `preview` SHALL offer commit and discard from its detail view.

#### Scenario: History lists batches with states

- **WHEN** one committed and one preview batch exist
- **THEN** the history view shows both with their filenames, states, and line counts

#### Scenario: Preview batch is actionable from detail

- **WHEN** the user opens a batch still in `preview` from history
- **THEN** its lines are shown and commit and discard actions are offered

### Requirement: Consistent error and loading feedback

Every screen SHALL render distinct states for loading, loaded-empty, loaded, and error. API error responses SHALL be mapped to human-readable messages; an unexpected or network-level failure SHALL show a generic problem message with a retry action. No user-initiated destructive action (movement deletion, batch commit, batch discard) SHALL execute without an explicit confirmation step.

#### Scenario: Network failure offers retry

- **WHEN** any screen's request fails due to a network error
- **THEN** the screen shows a problem message with a retry action that re-issues the request

#### Scenario: Destructive actions always confirm

- **WHEN** the user initiates movement deletion, batch commit, or batch discard
- **THEN** an explicit confirmation step is shown and the action executes only after confirmation

### Requirement: Exact money and date rendering

Monetary amounts SHALL be rendered from the API's exact decimal representation with the ISO 4217 currency code or a locale-correct currency symbol; binary floating-point SHALL NOT be used to store or compute displayed monetary values. Dates SHALL render in an unambiguous format (e.g., ISO `YYYY-MM-DD` or a localized long form) and SHALL NOT shift across time zones — an `occurred_on` calendar date SHALL display as that exact date regardless of the viewer's time zone.

#### Scenario: Amount renders without drift

- **WHEN** a movement's amount is `19999.99 INR`
- **THEN** the UI displays `19999.99` with the INR currency, with no rounding or binary-artifact digits

#### Scenario: Occurred date is time-zone stable

- **WHEN** a movement's `occurred_on` is `2026-08-20` and the viewer is in any time zone
- **THEN** the displayed date is 2026-08-20, not the adjacent day
