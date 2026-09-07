## ADDED Requirements

### Requirement: Root landing page, never a list view

The application root route (`/`) SHALL render a dedicated landing page whose content is
ingest + search entry, not an asset/account list. The landing page SHALL NOT be replaced
by, or redirect into, a list view; specific views (asset detail, search results, review
queue, documents, finance) SHALL remain on their own routes and SHALL NOT render as the
landing content.

#### Scenario: App root renders the landing page

- **WHEN** the user opens `/`
- **THEN** the landing page renders with the search bar, the add trigger, and the
  Insights section visible
- **AND** no asset/account list replaces it

#### Scenario: List views stay on their own routes

- **WHEN** the user opens `/assets` or `/documents`
- **THEN** the corresponding specific view renders (not the landing page)
- **AND** `/` still renders the landing page on return

### Requirement: Landing layout (search top, add center, insights bottom)

The landing page SHALL present, top-to-bottom: (a) one large, prominent **search bar**;
(b) one big centered **add trigger** ("[+]"); and (c) a labeled **Insights** section
below. The layout SHALL remain consistent between mobile (≤360px) and desktop widths and
SHALL NOT use a desktop-table surface on the landing page.

```text
LANDING — mobile (≈360px)                       LANDING — desktop
+-------------------------------------+       +-----------------------------------------------+
| Procrastinator              [◐] [☰]  |       | Procrastinator                        [◐] [☰] |
| +---------------------------------+ |       | +-------------------------------------------+ |
| | 🔍  Search assets & documents…  | |       | | 🔍  Search brands, models, documents…       | |
| +---------------------------------+ |       | +-------------------------------------------+ |
|             +-----+                |       |                        +-----+                 |
|             | [+] |   big primary  |       |                        | [+] |   big primary   |
|             +-----+   trigger      |       |                        +-----+   trigger       |
| +---------------------------------+ |       | +--------------------------------------------+ |
| | Insights  (placeholder —        | |       | | Insights  (placeholder — empty this cycle) | |
| |  empty this cycle)              | |       | +--------------------------------------------+ |
| +---------------------------------+ |       +-----------------------------------------------+
+-------------------------------------+
[◐] = theme toggle (light/dark/system)   [☰] = hamburger → opens navigation sheet
```

#### Scenario: Layout order is fixed

- **WHEN** the landing page renders at either width
- **THEN** the search bar appears above the add trigger, and the Insights section below it

#### Scenario: Insights placeholder is visible but empty

- **WHEN** the landing page renders
- **THEN** a section labeled "Insights" is present
- **AND** it shows a placeholder treatment (no data renders this cycle)

### Requirement: Add trigger reveals input-method picker

Clicking or touching the [+] trigger SHALL reveal exactly the three input methods —
**Camera**, **Upload**, **Text** — so the user picks a method and then provides input.
The user SHALL NOT pre-select a document type or an account before providing input;
classification and routing remain automatic.

```text
   tap [+]
─────────────►   +-----------------------------------+
                 |  📷 Camera   📎 Upload   ✎ Text    |
                 +-----------------------------------+
                    pick one → provide input → result
```

#### Scenario: User adds via the picker without pre-selection

- **WHEN** the user taps the add trigger and picks Camera, Upload, or Text
- **THEN** the chosen input capture opens
- **AND** no document-type or account selection is requested before input

#### Scenario: Input methods are touch-friendly on mobile

- **WHEN** the landing renders at 360px width
- **THEN** the three method options render as touch targets of at least 44px

### Requirement: Companion text with file inputs

When the user picks Camera or Upload, the add flow SHALL let the user add **optional
accompanying text** alongside the file(s), and the processing pipeline SHALL consume the
file(s) plus that text as one item (text as additional information about the item).
Text alone SHALL also be processable on its own.

#### Scenario: Photo plus note processed together

- **WHEN** the user adds a photo with an accompanying note
- **THEN** the photo and the note are processed together as one item through the same pipeline

#### Scenario: Text alone processes

- **WHEN** the user picks Text and provides pasted/typed content without a file
- **THEN** the text is processed through the same pipeline as a standalone item

### Requirement: Hamburger-only navigation shell

The app shell SHALL show a **hamburger menu icon** on every screen; tapping it SHALL open
the navigation treatment (e.g. sheet) containing the nav links, the active-user control,
and the theme toggle. No permanent sidebar SHALL render on any screen (including the
landing page).

#### Scenario: No permanent sidebar anywhere

- **WHEN** the user visits `/`, `/documents`, or an asset detail page at desktop width
- **THEN** no permanent left sidebar is rendered
- **AND** the hamburger icon is visible and opens the navigation

#### Scenario: Navigation sheet contains controls

- **WHEN** the user opens the navigation on any screen
- **THEN** links to the specific views plus the active-user switcher and theme toggle are reachable

### Requirement: Deferred account selection for statements

When an added item is detected as a statement, the add flow SHALL ask **only then** for
the destination account (inline within the flow); the user SHALL NOT be required to
choose an account before providing input. A statement added without an account SHALL NOT
silently fail — the flow SHALL give the user the account selection at that step.

#### Scenario: Statement detected asks for account inline

- **WHEN** the user adds a statement file without a prior account choice
- **THEN** the flow surfaces the account selector for that item and proceeds once chosen

#### Scenario: Non-statement items never ask for an account

- **WHEN** the user adds a receipt photo
- **THEN** no account selection is requested

### Requirement: Smooth navigation to specific views

The landing page SHALL link or hand off to the specific views so the user can reach:
asset detail, search results, the review queue, the documents section, and the finance
screens. Navigation SHALL keep the landing page the canonical root (returning to `/`
restores the landing state).

#### Scenario: Search from landing lands on results view

- **WHEN** the user submits a query in the landing search bar
- **THEN** the search results view opens with that query applied
- **AND** navigating back to `/` restores the landing page

#### Scenario: Results and detail open as specific views

- **WHEN** the user opens a search hit or an asset/document link
- **THEN** the corresponding detail/results view renders with navigation available via the hamburger
