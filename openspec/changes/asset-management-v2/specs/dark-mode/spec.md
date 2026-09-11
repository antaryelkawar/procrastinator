# dark-mode — Delta

## ADDED Requirements

### Requirement: Theme toggle with light/dark/system

The UI SHALL provide a theme toggle offering **light**, **dark**, and **system** modes. The choice SHALL be persisted (localStorage) and applied on next visit. Implementation SHALL map system to `prefers-color-scheme` and react live when the OS theme changes while set to system.

#### Scenario: Toggle to dark applies immediately

- **WHEN** the user switches the theme to dark from any view
- **THEN** the entire app (shadcn surfaces, tables, dialogs, toasts, the landing page) re-renders dark without a full reload

#### Scenario: Preference persists across sessions

- **WHEN** the user sets dark mode, then reloads the app
- **THEN** the app renders in dark mode with no flash of light theme (theme applied before first paint)

#### Scenario: System mode follows the OS

- **WHEN** the theme is set to system and the OS switches between light and dark while the app is open
- **THEN** the app updates to follow, without user action

#### Scenario: Default is system

- **WHEN** a first-time user opens the app with no stored preference
- **THEN** the app renders in system mode

### Requirement: Dark mode is verified everywhere

The UX test plan (gate, see `ux-test-plan`) SHALL include dark-mode assertions for every primary flow — landing, search results, asset detail, documents, finance, reviews, add/ingest — at both 360px and desktop widths.

#### Scenario: No unreadable contrast on any surface

- **WHEN** the audit traces of the UX test plan are executed in dark mode
- **THEN** all text/forms meet the readable-contrast quality bar on both themes
