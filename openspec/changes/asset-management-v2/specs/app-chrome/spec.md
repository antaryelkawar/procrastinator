# app-chrome — Delta

## ADDED Requirements

### Requirement: Route surface is the new information architecture

After the feature reorg, the reachable route surface SHALL be exactly: `/` (landing), `/search`, `/ingest/reviews`, `/assets`, `/assets/:id`, `/documents`, `/finance/accounts`. Any route not in this list SHALL NOT be reachable by direct URL: it SHALL either return a 404-style "not found" state or redirect to the closest surviving view — never a stale page, a blank shell, or a dead link. The legacy routes that existed before the reorg (old landing redirect, stale page names, moved entry points) SHALL be removed from the router rather than merely hidden: no navigation affordance in the app SHALL link to them, and deep-linking an old URL from the address bar SHALL never render old content.

#### Scenario: Legacy route deep-link redirects

- **WHEN** a user loads a pre-reorg route URL directly in the address bar
- **THEN** the router resolves it to either the closest surviving route (redirect) or a 404 not-found state — old page content is never rendered

#### Scenario: No dead links in the app

- **WHEN** a live integration walk follows every link affordance in the app shell, navigation sheet, and all reachable pages
- **THEN** every navigation results in one of the reachable routes above; no link targets a removed or 404-only route

#### Scenario: /assets/:id deep-link still works

- **WHEN** a user loads `/assets/{id}` directly for an asset they own
- **THEN** the asset detail view renders (this route is part of the reachable surface, not legacy)

### Requirement: Home is reachable from the app shell

The landing page `/` SHALL be reachable from every view using the app's sole chrome (there is no persistent bar to lean on). The hamburger navigation sheet SHALL include a **Home** entry as its first navigation link (above Review Queue), and the centered brand wordmark on the landing page SHALL be a tap target that (re)navigates to `/`. A user on any view can therefore always return home via the sheet, and keyboard/screen-reader users have a named "Home" link.

#### Scenario: Home entry in the sheet

- **WHEN** the user on any non-landing view opens the navigation sheet and taps "Home"
- **THEN** the sheet dismisses and the app navigates to `/` (the landing page renders)

#### Scenario: Home named for assistive tech

- **WHEN** a screen-reader user explores the navigation sheet
- **THEN** a link labeled "Home" exists and is the first navigation item

#### Scenario: Back to base after deep navigation

- **WHEN** the user is on `/assets/{id}` after a long search journey
- **THEN** there is at least one tappable affordance (sheet Home entry) returning to `/` without using the browser back button

### Requirement: No top bar — brand mark + top-left hamburger

The previous top bar (brand left, theme toggle right, hamburger top-right) SHALL be removed entirely. No view SHALL render a persistent horizontal bar. The new chrome layout is:

1. **Brand wordmark** — a stylized "PROCRASTINATOR" mark centered in the viewport on the landing page (hero treatment, not a bar element); the wordmark is a tap affordance navigating to `/`.
2. **Hamburger (☰)** — floats in the **top-left** corner of every view (including landing), opening the sole slide-in navigation sheet (per `landing-page`).
3. **Sheet absorbs the profile and theme controls** — the profile (avatar + name) row and the dark mode toggle (light / dark / system) live INSIDE the navigation sheet; they are removed from the top bar area.

#### Scenario: Top bar is gone

- **WHEN** any route renders on mobile 360px or desktop ≥1280px
- **THEN** no horizontal top bar exists; the only chrome is the top-left floating hamburger (plus the top-right context [+] on non-landing views)

#### Scenario: Brand is a hero wordmark, not a bar element

- **WHEN** the landing page renders at 360px
- **THEN** the PROCRASTINATOR wordmark renders centered above the search bar as a large stylized mark (not a small header strip), and tapping it navigates to (or refreshes) `/`

#### Scenario: Theme toggle lives in the sheet

- **WHEN** the user opens the navigation sheet on any view
- **THEN** a light/dark/system toggle is present in the sheet, the persisted theme preference still applies (no flash on the next load), and no toggle exists outside the sheet

#### Scenario: Profile lives in the sheet

- **WHEN** any route renders
- **THEN** no avatar/name element appears outside the sheet; the avatar + name row is the first item inside the sheet (per `landing-page`)

### Requirement: Context [+] on every non-landing view

Every view except the landing page SHALL show a [+] icon button in the **top-right** corner that creates the thing local to that section:

| View | [+] adds |
|---|---|
| `/ingest/reviews` | a new review (ingest entry) |
| `/assets` | a new asset |
| `/documents` | a new document (upload) |
| `/finance/accounts` | a new account |

The [+] button SHALL be ≥44px touch target, reachable by keyboard, and labeled with the concrete action (e.g. "Add asset") for screen readers. Tapping it SHALL open the section's creation flow using the intuitive input pattern of `intuitive-input` — never the rejected old-style field-by-field forms. The landing page keeps its big centered [+] and SHALL NOT additionally show a corner [+].

#### Scenario: Corner plus on each section (parameterized)

- **WHEN** the user is on each of the rows in the table above
- **THEN** a top-right [+] icon button is present and opening it starts that section's add flow in the composer pattern (for documents this is the unified ingest composer of `landing-page`)

#### Scenario: Landing has no corner plus

- **WHEN** the landing page renders
- **THEN** only the big centered [+] exists (no top-right [+])

#### Scenario: Add asset is intuitive, not a form

- **WHEN** the user taps [+] on `/assets`
- **THEN** the creation flow opens with the directive/composer input of `intuitive-input` — no multi-field form with empty brand/model/serial boxes as the first interaction, and no presentation of one field per column
