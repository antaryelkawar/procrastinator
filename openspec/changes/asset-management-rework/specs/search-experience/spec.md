## ADDED Requirements

### Requirement: Search as primary navigation

The application SHALL present a single prominent search bar as the primary way to find anything (assets, documents, movements). The search bar SHALL be reachable globally (persistent in the app shell), keyboard-first (global shortcut, full keyboard navigation of results), and SHALL accept natural queries including brand, model, serial, category, and free text.

#### Scenario: Global keyboard shortcut focuses search

- **WHEN** the user presses the global search shortcut from any page
- **THEN** focus moves to the search bar and typing immediately filters results

#### Scenario: Search by brand finds the Cooler Master cabinet

- **GIVEN** an asset with brand "Cooler Master"
- **WHEN** the user types "cooler master" in the search bar
- **THEN** the cabinet asset appears in the results

### Requirement: Filtered and faceted search

Search SHALL support filters for at least: asset category, brand, purchase-date range, warranty status (active/expired/expiring within N days/unknown), presence of linked documents, and document classification. Filters SHALL be combinable with free text and SHALL be reflected in the URL so searches are shareable.

#### Scenario: Filter by warranty expiring soon

- **WHEN** the user filters search to warranty status "expiring within 90 days"
- **THEN** only assets whose computed warranty_end falls in that window are shown

#### Scenario: Shareable search URL

- **WHEN** the user applies category `appliance` and text "microwave"
- **THEN** the URL encodes both constraints and reloading it reproduces the same result set

### Requirement: Feature-rich results table

Asset search/list results SHALL be rendered in a shared feature-rich data-table component that provides at minimum: column visibility selection, multi-column sorting, and pagination. The same table component SHALL be used for the assets list, search results, review queue, and statement preview — replacing hand-rolled per-page tables.

#### Scenario: User hides and reorders columns

- **WHEN** the user hides the "price" column and sorts by "purchase date" descending in the assets table
- **THEN** the table reflects the change
- **AND** the same column controls are available on the review queue and statement preview tables

### Requirement: Consistent search across entity kinds

Search results SHALL group or label hits by kind (asset, document, movement) so a single query can surface an asset, its invoice document, or a matching ledger movement, each linking to its detail view.

#### Scenario: Query matching both an asset and a movement

- **GIVEN** a purchase of a microwave recorded both as an asset and as a ledger movement
- **WHEN** the user searches "microwave"
- **THEN** results include both hits, labeled by kind, each navigable to its detail view

## MODIFIED Requirements

### Requirement: Search as primary navigation (cycle 2)

The application SHALL treat search as the primary way to find anything (assets, documents,
movements). The primary search bar SHALL live on the **root landing page** (`/`) as its
top element — prominently sized and keyboard-first (global shortcut, full keyboard
navigation of results) — and SHALL accept natural queries including brand, model, serial,
category, and free text. Search results SHALL render as a specific view (not replacing
the landing page), and navigating back to `/` SHALL restore the landing search entry.

(Previously: the search bar was a persistent shell control on every page. The shell is
now hamburger-only; the landing page is the canonical search surface. Results remain on
their own route.)

#### Scenario: Landing top bar is the search entry

- **WHEN** the user opens `/`
- **THEN** the top-most interactive element on the landing page is the search bar

#### Scenario: Submitting from landing opens results view

- **WHEN** the user types "cooler master" and submits
- **THEN** the results view opens with that query applied and the cabinet asset appears

#### Scenario: Global shortcut still focuses search

- **WHEN** the user presses the global search shortcut from any screen
- **THEN** focus moves to the landing search entry (or the global sheet/overlay search)

### Requirement: Feature-rich results table (cycle 2 wording)

Asset search/list results SHALL be rendered in a shared feature-rich data-table component
providing at minimum column visibility selection, multi-column sorting, and pagination;
**at mobile widths the same results SHALL render as cards/list cells rather than a desktop
table**, with the same filtering/sorting semantics. The same component(s) SHALL serve the
assets list, search results, review queue, statement preview, and Documents view —
replacing hand-rolled per-page tables (see web-platform).

(Previously: mobile card rendering was not addressed.)

#### Scenario: Results are cards on mobile

- **WHEN** a scoped search yields ten hits at 360px
- **THEN** the results render as cards (touch-friendly rows) rather than a narrow desktop table

### Requirement: Consistent search across entity kinds (cycle 2 wording)

Search results SHALL group or label hits by kind (asset, document, movement) so a single
query can surface an asset, its invoice document, or a matching ledger movement, each
linking to its detail view; the **Documents view's free-text filter SHALL reuse the same
filter semantics over the documents domain** (filename, status, linked asset name).

(Previously: documents in the result kinds referred to ledger search hits; intra-Documents
filtering is now included for the same semantic consistency.)

#### Scenario: Documents view reuses the same kind semantics

- **WHEN** the user filters the Documents view by text "invoice"
- **THEN** the match applies over filename/status/linked asset name using the same
  filtering semantics as the global search
