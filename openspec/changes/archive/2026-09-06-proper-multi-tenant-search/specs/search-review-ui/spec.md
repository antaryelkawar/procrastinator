# Spec: search-review-ui

New capability for change `proper-multi-tenant-search`. Adds the React UI surfaces that consume the `search` and `confidence-review` API capabilities: a quick-search typeahead in the app shell, a paged search results page, and an ingest review queue (list pending/reviewed reviews, approve, reject). All API access is **OpenAPI-first**: the UI consumes the orval-generated client and the openapi-typescript types produced by `make codegen` from `procrastinator-backend/api/openapi.yaml`; the generated files are never hand-edited, and the only new search/review API code is thin hand-written app wiring (typed wrappers, schema re-exports, TanStack Query hooks). Every screen lives inside the existing `<AppShell />` and is tenant-scoped to the active user (query keys carry the active user id; requests are disabled while no user is active). This is an MVP: no new per-resource detail pages, no theming, no client-side full-text filtering.

## ADDED Requirements

### Requirement: UI API access is OpenAPI-first and generated files are not hand-edited

The UI SHALL consume the six new operations — quick search, paged search, list reviews, get review, approve review, and reject review — exclusively through the orval-generated client (`ui/src/lib/api/generated/orval/procrastinator.ts`) and the openapi-typescript types (`ui/src/lib/api/generated/paths.d.ts`), both produced by `make codegen` from `procrastinator-backend/api/openapi.yaml`. The only hand-written API code for these operations SHALL be the typed wrappers in `ui/src/lib/api/client.ts`, the schema re-exports in `ui/src/lib/api/schema.ts`, and the TanStack Query hooks in `ui/src/lib/api/hooks.ts`. The UI SHALL NOT define its own endpoint URL strings, request bodies, or response types for these operations beyond wiring the generated ones.

#### Scenario: The new operations come from codegen, not hand-written types

- **WHEN** the change is implemented and `make codegen-drift-check` is run
- **THEN** the committed `paths.d.ts` and `orval/procrastinator.ts` match regeneration from `openapi.yaml` (no drift), and the orval client exports a typed function for each of the six new operations

#### Scenario: No hand-written search/review endpoint types

- **WHEN** the UI source for the search and review surfaces is inspected
- **THEN** the request/response types used by those surfaces are the generated types (re-exported via `ui/src/lib/api/schema.ts`), and no line in `paths.d.ts` or `orval/procrastinator.ts` is hand-authored for a search or review operation

### Requirement: A quick-search typeahead is available in the app shell

The app shell SHALL provide a quick-search box, visible on every screen, that queries the quick-search endpoint as the user types and shows a dropdown of matching hits. The box SHALL debounce keystrokes by 250 ms before issuing a request, SHALL issue no request for a blank (empty or whitespace-only) query, and SHALL show at most 10 hits (the endpoint's default limit). Each dropdown entry SHALL display the hit's `title` and, when present, its `subtitle` and its `confidence`. Selecting a hit — by mouse click or by keyboard (ArrowUp/ArrowDown to move the active option, Enter to select, Escape to dismiss) — SHALL navigate to that resource's page and close the dropdown. The input and its dropdown list SHALL be accessible (a labelled input; the list and its options expose correct ARIA combobox/listbox roles and states; focus is managed on open and on dismiss).

#### Scenario: Typing a matching term shows hits

- **WHEN** a user with visible matching rows types a term and the 250 ms debounce settles
- **THEN** the quick-search endpoint is called with that term and the dropdown shows the returned hits (title, optional subtitle, optional confidence)

#### Scenario: Rapid typing issues a single settled request

- **WHEN** a user types several characters in quick succession
- **THEN** fewer quick-search requests are issued than keystrokes (the 250 ms debounce coalesces them) and the final dropdown reflects the last settled term

#### Scenario: A blank query issues no request

- **WHEN** the box is empty or contains only whitespace
- **THEN** no quick-search request is issued and the dropdown shows no hits

#### Scenario: Selecting a hit navigates

- **WHEN** a user selects a hit (click, or Enter on the active option)
- **THEN** the app navigates to that resource's page (asset or import-batch detail where such a page exists; the type's list page otherwise) and the dropdown closes

#### Scenario: Escape dismisses the dropdown

- **WHEN** the user presses Escape while the dropdown is open
- **THEN** the dropdown closes and focus returns to the input

#### Scenario: The typeahead is accessible

- **WHEN** the app shell is inspected for accessibility
- **THEN** the search input has a label, the options list and its options carry correct ARIA roles/states, and the region passes an automated accessibility check (axe)

### Requirement: A paged search results page

The UI SHALL provide a search results page at route `/search` that renders the paged results for a query. It SHALL render each hit with a type indicator, its `title`, and — when present — its `subtitle` and `confidence`, in the exact order returned by the paged endpoint (the page SHALL NOT re-sort the results). It SHALL present pagination controls driven by the endpoint's `page`, `page_size`, and `total` (page-size default 20). Selecting a hit SHALL navigate to that resource's page. A blank query SHALL render an empty state (not an error); a page beyond the last SHALL render an empty hit list with the `total` unchanged; loading and error states SHALL be shown. The page SHALL be accessible and SHALL render within the app shell.

#### Scenario: First page renders with total

- **WHEN** a user opens the results page for a query with matching rows
- **THEN** the first page of hits (up to `page_size` 20) is shown together with the total count and the pagination controls

#### Scenario: Advancing a page

- **WHEN** a user moves to the next page
- **THEN** the next page's hits are shown and the controls reflect the new page number

#### Scenario: Hits render in endpoint order

- **WHEN** the endpoint returns hits in a combined order
- **THEN** the page displays them in that same order (no client-side re-sort)

#### Scenario: A blank query shows an empty state

- **WHEN** the results page has a blank query
- **THEN** an empty state is shown and no error is displayed

#### Scenario: A request failure shows an error state

- **WHEN** the paged request fails
- **THEN** an error state with a retry affordance is shown

#### Scenario: The results page is accessible

- **WHEN** the results page is inspected for accessibility
- **THEN** the results list, pagination controls, and hit links have correct labels/roles and the page passes an automated accessibility check (axe)

### Requirement: An ingest review queue lists pending and reviewed reviews

The UI SHALL provide an ingest review queue at route `/ingest/reviews` that lists the active user's reviews. It SHALL default to listing `pending` reviews and SHALL offer a status filter to switch among `pending`, `approved`, and `rejected`. Each review row SHALL show its document type, its `confidence` (rendered as absent when the value is null), the originating source's filename, the best-matched asset (its display title, or an explicit "no match" indicator when absent), and its creation time; `approved` and `rejected` rows SHALL additionally show their decision state. The queue SHALL show empty, loading, and error states and SHALL be accessible.

#### Scenario: Pending reviews are listed by default

- **WHEN** a user opens the review queue
- **THEN** only `pending` reviews are listed and no `approved` or `rejected` review is shown

#### Scenario: Filtering by status

- **WHEN** a user selects the `rejected` status filter
- **THEN** only `rejected` reviews are listed

#### Scenario: A confidence-less review shows no confidence

- **WHEN** a review's `confidence` is null (absent at hold time)
- **THEN** its row renders no confidence value (not `0` or `0%`)

#### Scenario: A best-matched asset is shown when present

- **WHEN** a review has a best-matched asset
- **THEN** its row shows that asset's display title; when there is no match, the row shows an explicit "no match" indicator

#### Scenario: An empty queue shows an empty state

- **WHEN** there are no reviews for the selected status
- **THEN** an empty state is shown

#### Scenario: The queue is accessible

- **WHEN** the queue is inspected for accessibility
- **THEN** the rows, the status filter, and the per-review actions are labelled and correctly grouped, and the screen passes an automated accessibility check (axe)

### Requirement: Approve and reject a review from the queue

A `pending` review in the queue SHALL offer Approve and Reject actions, each gated behind a confirmation dialog. Confirming Approve SHALL call the approve endpoint and, on success, refresh the queue and the affected asset/movement data and show a success message; where the approve response returns the resulting asset, the success affordance SHALL allow navigating to that asset. Confirming Reject SHALL call the reject endpoint and, on success, refresh the queue and show a success message. A `409` (review not pending) or `404` (unknown or not visible) response SHALL be surfaced as an error and SHALL NOT be reported as a success; the queue SHALL refresh to reflect the current state. The confirmation dialogs SHALL be accessible (labelled, focus-trapped).

#### Scenario: Approving a pending review

- **WHEN** a user confirms Approve on a pending review and the approve call succeeds
- **THEN** after the refresh the review is shown as `approved` and a success message is shown

#### Scenario: Rejecting a pending review

- **WHEN** a user confirms Reject and the reject call succeeds
- **THEN** after the refresh the review is shown as `rejected` and a success message is shown

#### Scenario: A non-pending review surfaces 409 as an error

- **WHEN** approve or reject returns `409`
- **THEN** an error is shown (no success message) and the queue reflects the unchanged state after refresh

#### Scenario: An unknown or invisible review surfaces 404 as an error

- **WHEN** approve or reject returns `404`
- **THEN** an error is shown (no success message)

#### Scenario: Approve/reject require confirmation

- **WHEN** a user has not confirmed the dialog
- **THEN** no approve or reject request is sent

### Requirement: Upload surfaces the held-for-review (202) outcome

The document upload flow SHALL handle the `202 Accepted` outcome produced by the confidence gate in addition to the existing `201 Created` outcome. On `201` it SHALL show the created asset (which may now carry a `confidence`); on `202` it SHALL show that the upload is held for review (not committed) and SHALL offer a link to the review queue. The existing `422` (no usable identity) and other error outcomes SHALL continue to be surfaced as errors.

#### Scenario: A high-confidence upload shows the asset

- **WHEN** an upload returns `201`
- **THEN** the created asset is shown (with its confidence where present)

#### Scenario: A low-confidence upload shows a held-for-review state

- **WHEN** an upload returns `202`
- **THEN** a "held for review" state is shown (no created asset) with a link to the review queue

#### Scenario: The no-identity error is unchanged

- **WHEN** an upload returns `422`
- **THEN** the existing no-usable-identity error message is shown

### Requirement: Confidence is displayed where present

The UI SHALL display a record's resolution `confidence` on search hits and the asset/detail surfaces wherever the record carries one, and SHALL omit the indicator (rendering neither a value nor `0`) where the value is absent or null.

#### Scenario: A confidence value is shown

- **WHEN** a hit or asset carries `confidence` `0.82`
- **THEN** the UI displays that value (for example `0.82` or `82%`)

#### Scenario: Absent confidence shows no indicator

- **WHEN** a hit or asset's `confidence` is absent or null
- **THEN** the UI shows no confidence indicator and does not render `0` or `0%`
