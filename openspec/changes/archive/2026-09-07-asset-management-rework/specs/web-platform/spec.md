## ADDED Requirements

### Requirement: Current latest-stable frontend stack

Every framework and library used by the web UI SHALL be the current latest stable release of the right tool for its job, as verified against upstream releases at implementation time, and SHALL be pinned in the committed lockfile. Pre-release, canary, experimental, or unmaintained versions SHALL NOT be used. The dependency manifest SHALL be audited as part of this change so no chosen package lags behind its latest stable line without a recorded reason.

#### Scenario: Dependency audit records version status

- **WHEN** the implementation phase begins
- **THEN** a dependency audit lists each UI dependency, its pinned version, the latest stable upstream version, and a justification for any gap

#### Scenario: Stack is on current stable majors

- **GIVEN** the UI builds on React, Vite, Tailwind CSS, TanStack Query, and the router
- **WHEN** the change is implemented
- **THEN** each of these is on its current stable major line, pinned in the lockfile

#### Scenario: New libraries are added at latest stable

- **WHEN** the rework introduces a new dependency (e.g. a table library)
- **THEN** it is added at its current latest stable version with a lockfile pin

### Requirement: Shared feature-rich table library

The UI SHALL adopt one shared, actively maintained headless table library (TanStack Table or an equivalent current-stable alternative) to provide column visibility selection, multi-column sorting, and pagination. This library SHALL back a single shared DataTable component used by the assets list, search results, review queue, and statement preview; hand-rolled per-page table implementations SHALL be removed.

#### Scenario: One table component serves all list surfaces

- **WHEN** the user opens the assets list, the search results, the review queue, and a statement preview
- **THEN** all four render through the same shared DataTable component with column visibility, sorting, and pagination controls available on each

#### Scenario: Duplicated table implementations removed

- **WHEN** the rework is complete
- **THEN** no page in the UI renders tabular data through a hand-rolled table instead of the shared DataTable

### Requirement: Consolidated UI primitives

The UI SHALL consolidate on one primitive per job from the component library: one select/dropdown pattern, one upload/dropzone pattern, one dialog pattern, and one row-action/menu pattern. Duplicate or parallel implementations of the same primitive (e.g. native styled selects alongside library selects, two different upload UXs, ad-hoc row menus) SHALL be removed.

#### Scenario: Single select pattern everywhere

- **WHEN** the user encounters a dropdown on any page (filters, forms, account pickers)
- **THEN** it is the same shared select component with consistent behavior and styling

#### Scenario: Single upload pattern

- **WHEN** the user adds files from any surface
- **THEN** the same dropzone/upload component is used

### Requirement: Simple but modern design direction

The UI SHALL present few, clear surfaces with Add and Search as the primary, always-visible actions, and SHALL follow current component-library conventions (the project's shadcn/ui-style design system) with clean typography and consistent spacing. Navigation SHALL NOT require the user to choose between overlapping flows (e.g. separate document upload vs statement import pages). Surfaces SHALL read as a polished, native-feeling application rather than a bare document page.

#### Scenario: Primary actions are one gesture away

- **WHEN** the user is on any page of the app
- **THEN** the Add action and the search entry are both reachable in one gesture from the shell

#### Scenario: No duplicate-flow navigation entries

- **WHEN** the user opens the main navigation
- **THEN** there is a single Add entry (no separate "upload document" and "import statement" entries)

#### Scenario: Consistent modern styling

- **WHEN** the user moves between the landing page, documents, assets, search, and review screens
- **THEN** typography scale, spacing, and component styling follow the same design-system tokens with no page-specific ad-hoc styles

## MODIFIED Requirements

### Requirement: Current latest-stable frontend stack (cycle 2)

Every framework and library used by the web UI SHALL be the current latest stable release of the right tool for its job, as verified against upstream releases at implementation time, and SHALL be pinned in the committed lockfile. The stack SHALL be **re-audited in this cycle**, and the re-audit SHALL (a) evaluate whether any framework swap (e.g. Next.js, react-router framework mode, SvelteKit, TanStack Start) is materially better for the product goals (mobile-native-feeling UX, web support), recording the keep/swap decision with rationale, and (b) adopt the **current stable majors** of every dependency — including previously deferred majors (verify against upstream at implementation time; examples seen during spec: Vite 8, TypeScript 7, vitest 5, plugin-react 6, jsdom 30, testing-library/jest-dom 7) — or record a reason a major is intentionally not adopted. Pre-release, canary, experimental, or unmaintained versions SHALL NOT be used.

(Previously: the audit only listed dependencies and treated newer majors as out-of-scope
justified gaps; a framework swap was not evaluated and newer majors were deferred.)

#### Scenario: Framework-swap evaluation recorded

- **WHEN** the audit runs
- **THEN** the audit artifact records a swap-vs-keep decision with rationale per candidate framework

#### Scenario: Deferred majors are revisited

- **GIVEN** cycle 1 deferred Vite 8, TypeScript 7, and vitest 5 as justified gaps
- **WHEN** the cycle-2 audit runs
- **THEN** each deferred major is either adopted at current stable or given a fresh recorded justification

#### Scenario: No pre-release pins

- **WHEN** the audit completes
- **THEN** no dependency is pinned to a pre-release, canary, beta, or experimental tag

### Requirement: Shared feature-rich table library (cycle 2 wording)

The UI SHALL use one shared, actively maintained headless table library to provide
column visibility selection, multi-column sorting, and pagination behind one DataTable
component; **on mobile widths the same data SHALL render as cards/list cells using the
same component or a deliberate responsive treatment — desktop tables SHALL NOT be shown
on phones.** Hand-rolled per-page table implementations SHALL be removed.

(Previously: mobile treatment of tables was not addressed; a desktop table could be
rendered on a 360px viewport.)

#### Scenario: Mobile list treatment is not a desktop table

- **WHEN** the user opens the Documents view, assets list, or search results at 360px
- **THEN** the data renders as responsive cards/list cells rather than a narrow desktop table

#### Scenario: Single table source remains

- **WHEN** the rework is complete
- **THEN** no page renders tabular data through a hand-rolled table instead of the shared DataTable

## ADDED Requirements (cycle 2)

### Requirement: Dark mode toggle

The app SHALL provide a dark-mode toggle offering **light, dark, and system** choices.
The theme SHALL apply via the component design-system's class strategy (e.g. the shadcn/
Tailwind v4 `@custom-variant dark` pattern), SHALL persist across sessions, and SHALL be
applied without a flash of wrong-theme content on load. All surfaces (landing, lists,
documents, review, finance, dialogs, toasts) SHALL respect the active theme.

#### Scenario: Toggle switches theme on every surface

- **WHEN** the user selects dark mode
- **THEN** every surface renders with dark tokens and the selection persists across reloads

#### Scenario: System follows the OS preference

- **WHEN** the user selects system
- **THEN** the rendered theme follows the OS color scheme on initial load and on change

#### Scenario: No theme flash on load

- **WHEN** the app loads with dark previously selected
- **THEN** the dark theme applies before content paints (no light flash)

### Requirement: Mobile-first native-feeling quality bar

The UI SHALL be designed mobile-first: touch targets SHALL be at least 44px; content
SHALL never horizontally overflow the viewport; small viewports SHALL render lists as
cards (not squeezed desktop tables); camera/upload affordances SHALL use native-feeling
pickers on mobile; interactive feedback SHALL be immediate (optimistic/toast feedback or
inline states), and navigation SHALL require no more taps than a native app would. On
desktop the same app SHALL render as a first-class web experience (readable layout,
no phone-like constraints forced on wide screens).

#### Scenario: Touch targets meet the bar

- **WHEN** the app renders at 360px and at desktop
- **THEN** all interactive controls (inputs, buttons, row actions, menu) are ≥44px touch targets at mobile width

#### Scenario: No horizontal overflow

- **WHEN** any surface renders at 360px
- **THEN** the page has no horizontal scrollbar

#### Scenario: Desktop is not constrained by mobile

- **WHEN** the app renders at a wide viewport
- **THEN** all surfaces use the available width without forcing a mobile-only layout onto desktop

### Requirement: UX test plan gate

The implementation plan (subspec) SHALL include a **UX-oriented test plan** as a gating
artifact: implementation of this change SHALL NOT be signed off until the UX test plan
exists and passes. The plan SHALL cover, at minimum, at both 360px and a desktop width:
(1) the landing page top-level layout and the add picker flows — camera, upload, and
text, including photo-plus-companion-text and text-alone; (2) search-as-navigation from
the landing root (keyboard + touch); (3) dark mode on/off/system on representative
surfaces; (4) the Documents section — listing, status filters, linked-asset navigation,
reprocess, and delete; (5) reprocess/edit/delete flows for assets and documents;
(6) the duplicate-upload reprocess-vs-keep prompt; (7) the general quality bar
(native-feel polish, touch targets, no desktop tables on phones, no horizontal overflow).

#### Scenario: Gate blocks signoff until the plan passes

- **WHEN** the implementation is otherwise complete but the UX test plan is absent or failing
- **THEN** the change is not signed off and the gate remains open

#### Scenario: Plan enumerates the required behaviors

- **WHEN** the subspec is produced
- **THEN** the UX test plan enumerates each required behavior above with expected results
  for mobile (360px) and desktop widths
