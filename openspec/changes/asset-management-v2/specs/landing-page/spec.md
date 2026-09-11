# landing-page — Delta

## ADDED Requirements

### Requirement: Landing page is the general ingest + show page

The route `/` SHALL render the landing page directly — it SHALL NOT redirect to another route — and the landing page SHALL always remain the app's home. The layout, top-to-bottom, is:

1. A **PROCRASTINATOR brand wordmark** centered as a hero mark (tap affordance → `/`), not a bar element (per `app-chrome`).
2. A large search bar.
3. A centered [+] button that is the app's primary ingest entry point — opening the **unified Add composer** (see below), NOT the earlier three-card layout.
4. A labeled **Insights** placeholder section.
5. A static chat-bar placeholder pill fixed at the bottom.

The page is mobile-first (designed at 360px), has NO persistent navigation / top bar (per `app-chrome`), and carries only the top-left floating hamburger as chrome.

```
 ┌──────────── 360px (mobile) ──────────┐    ┌────────────── desktop ≥1024px ──────────────┐
 │ ☰                         (top-left) │    │ ☰                               (top-left)  │
 │                                      │    │                                             │
 │        P R O C R A S T I N A T O R   │    │        P R O C R A S T I N A T O R          │
 │        (stylized hero wordmark)      │    │                                             │
 │  ┌─────────────────────────────┐     │    │  ┌─────────────────────────────────────┐    │
 │  │ 🔍  Search your stuff…      │     │    │  │ 🔍  Search your stuff…              │    │
 │  └─────────────────────────────┘     │    │  └─────────────────────────────────────┘    │
 │                                      │    │                                             │
 │                   ┌──────┐           │    │                     ┌──────┐                │
 │                   │  +   │           │    │                     │  +   │                │
 │                   └──────┘           │    │                     └──────┘                │
 │                                      │    │                                             │
 └──────────────────────────────────────┘    └─────────────────────────────────────────────┘
```

The landing [+] does NOT get a corner [+] of its own (the corner [+] lives on every non-landing view per `app-chrome`).

#### Scenario: Logged-in user lands on the clean shell

- **WHEN** a signed-in user opens `/` on a 360px-wide phone in light mode
- **THEN** they see in order: top-left hamburger, centered PROCRASTINATOR wordmark, search bar, centered [+] button, Insights placeholder, and a fixed bottom chat-bar placeholder — no top bar, no sidebar, no corner [+]

#### Scenario: Wordmark navigates home

- **WHEN** the user taps the PROCRASTINATOR wordmark
- **THEN** no harmful navigation occurs (it targets `/`, keeping the user home) and it is exposed as a link/button for assistive tech

#### Scenario: Search from the landing page

- **WHEN** the user submits a query in the landing search bar
- **THEN** they are navigated to the search results view with the query preserved

#### Scenario: Placeholder chat bar is inert but present

- **WHEN** the landing page renders in light or dark mode, on mobile or desktop
- **THEN** the chat-bar placeholder pill is visually consistent and disabled (not interactive), signaling a future natural-language assistant

#### Scenario: Insights is a labeled placeholder

- **WHEN** the landing page renders
- **THEN** a section labeled "Insights" renders placeholder content and routes users to landed views when its widgets are implemented later

### Requirement: Landing [+] opens the unified Add composer

Research basis (websearcher, 2026-09-09 — replaces the earlier three-card design, original sources/synthesis): the **unified composer with attachment strip** (messaging-app idiom: one input area + persistent camera/file/paste icon strip) plus a **bottom-sheet action surface** where a type choice is needed; **drag-and-drop-first** was evaluated and rejected as primary (desktop idiom, poor 360px fit), retained as a secondary desktop drop target on the composer. The earlier three-card layout (Camera/Image, Document, Text as sibling form cards) is **rejected**.

Tapping the landing [+] SHALL open a **single unified Add composer** (bottom sheet on mobile, inline sheet/dialog on wide screens) containing:

1. One **primary flexible input** ("What are you adding? Describe or type it…") into which the user may paste or type an ingest note/text.
2. A persistent **attachment strip** of icon actions: 📷 Camera / 📁 File (PDF, image…) / ⌨ Paste-text — any combination usable in one composer session, with the composed fragment visible as removable attachments/chips.
3. An **optional ingestion note/directive field** ("Anything the scanner should know?") always available, carried through the ingestion directive channel per `document-ingestion`.
4. Submit starts the appropriate ingestion type (file → document upload flow; pasted text → text ingest; camera capture → image ingest) through the existing add endpoints — with no doc-type or account pre-selection.

The resulting ingestion path through the pipeline (classification/extraction/duplicates) is unchanged: a submitted content-hash duplicate SHALL still produce the duplicate-reprocess conflict flow of `duplicate-reprocess-flow` (409 / DuplicateReport → reprocess-or-keep prompt).

#### Scenario: One composer, all types

- **WHEN** the user taps the landing [+] and in the composer attaches a PDF/Photo AND types "warranty for the microwave, serial under the barcode" in the note field
- **THEN** submitting produces a single ingestion handled by the existing pipeline with the directive note attached — no separate card selection step, no doc-type/account pre-selection

#### Scenario: Paste-text-only ingest accepted without a file

- **WHEN** the user opens the composer, types or pastes text only, and submits
- **THEN** the text is ingested as its own entry (no file required) — same as before, but reached through the unified composer instead of a Text card

#### Scenario: Files become removable chips in session

- **WHEN** the user attaches two files in one composer session, then removes one chip
- **THEN** the composition updates accordingly before submit; only the chips present at submit are ingested

#### Scenario: Mobile ergonomics

- **WHEN** the composer opens at 360px width
- **THEN** it presents as a bottom sheet reachable and dismissable with one hand; no horizontal overflow and no hidden scroll content

#### Scenario: Duplicate upload still hits the reprocess prompt

- **WHEN** the composer submits a file whose content hash matches an existing document
- **THEN** the duplicate-reprocess conflict flow (reprocess / keep existing, per `duplicate-reprocess-flow`) runs exactly as for the earlier card design — resubmitting directly does not bypass or change the 409 contract

#### Scenario: Camera capture inside the composer

- **WHEN** the user picks the 📷 strip action on a mobile device, captures a photo, adds a note, and submits
- **THEN** the capture flows through the same upload/ingestion path with the note attached — no second capture-or-form step

### Requirement: Hamburger is the only navigation mechanism

The app SHALL NOT render a persistent sidebar on any route — and (delta 2) NO top bar of any kind (delta 2 supersedes the earlier top-right-hamburger contract). The sole navigation mechanism is a hamburger (☰) icon floating in the **top-left** corner of every view (landing included), which opens the sole slide-in navigation sheet (dismissible: scrim tap, ✕, or completing a link tap). Also in the sheet (moved out of the deleted top bar): the profile (avatar + name — first item) and the theme toggle.

The sheet content, top-to-bottom:

1. **Profile section** — circular profile logo/avatar + user name at the top.
2. **Home** link (navigates to `/`) — per `app-chrome`, first nav item.
3. **Navigation links** — Review Queue, Assets, Documents, Accounts.
4. **Theme toggle** — light / dark / system (persisted, no flash on next load).

Sections that are not yet implemented (Subscriptions, Relationships, Inventory) SHALL appear as disabled placeholder items (visually TBD) rather than being silently omitted; they are inert (no navigation).

#### Scenario: Hamburger opens the sheet on any route

- **WHEN** the user taps ☰ (top-left) on `/`, `/assets`, `/documents`, a finance view, or the review queue (mobile or desktop)
- **THEN** the slide-in sheet overlays the content listing profile + Home + Review Queue / Assets / Documents / Accounts + theme toggle, with Subscriptions / Relationships / Inventory rendered disabled (TBD)

#### Scenario: Sheet navigation lands on the view

- **WHEN** the user taps "Assets" in the sheet
- **THEN** the sheet dismisses and the route changes to the assets view; the app persists the no-sidebar rule

#### Scenario: Home link returns from a deep view

- **WHEN** the user on `/finance/accounts` taps "Home" in the sheet
- **THEN** the route becomes `/` and the landing page renders (wordmark, search, [+], Insights, chat pill; hamburger stays top-left)

#### Scenario: Theme toggle inside the sheet persists

- **WHEN** the user switches the theme to Dark in the sheet and reloads the app
- **THEN** dark mode is applied before first paint with no flash, and the system-follow option remains selectable

#### Scenario: Profile section is the account entry

- **WHEN** the sheet renders
- **THEN** the avatar + user name row is the first item and opens the profile/active-user context (not a dead placeholder)

#### Scenario: Sheet dismisses without navigation

- **WHEN** the user taps the scrim (or ✕) instead of a link
- **THEN** the sheet closes, the route is unchanged, and focus returns to the hamburger trigger
