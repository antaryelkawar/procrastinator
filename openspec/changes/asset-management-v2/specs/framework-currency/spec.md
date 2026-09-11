# framework-currency — Delta

## ADDED Requirements

### Requirement: Verify versions before upgrading

Framework versions SHALL be verified against authoritative sources (npm registry, GitHub releases) at implementation time — training data SHALL NOT be trusted. The audit performed 2026-09-08 established:

| Library | Verified latest stable | Decision |
|---|---|---|
| vite | 8.2.2 (Rolldown) | upgrade |
| react / react-dom | 19.2.8 | upgrade (19.x; no v20) |
| react-router | 8.3.1 (Remix merged in) | upgrade, stay on react-router |
| @tanstack/react-query | 5.102.x | stay on v5 (v6 is RC only) |
| tailwindcss | 4.3.3 (CSS-first) | upgrade to v4 |
| lucide-react | 1.42.0 | upgrade (1.x) |
| shadcn CLI | 4.21.x | refresh components |
| vitest | 5.0 | upgrade (Node ≥22.12 floor) |
| vite-plugin-pwa | 1.3.0 | add |

#### Scenario: Version audit is repeatable

- **WHEN** the framework-upgrade task runs
- **THEN** it re-checks each dependency's dist-tag before pinning, and records the verified version + date in the task notes

### Requirement: Metaframework evaluation recorded with a decision

The implementer SHALL record the metaframework evaluation outcome: **Next.js 16, Remix (now React Router v8 Framework Mode), and SvelteKit were evaluated and rejected** — this app is a private, SEO-less, client-rendered CRUD tool on a Go API; SSR machinery adds a runtime and migration cost without materially improving snappiness. The chosen path: upgraded Vite SPA + React Compiler (via `@vitejs/plugin-react` v6 `reactCompilerPreset`, `babel-plugin-react-compiler@1.x`) + PWA.

#### Scenario: Evaluation is documented, not silent

- **WHEN** the design document for this change is written
- **THEN** it contains the metaframework comparison (Next/Remix/SvelteKit vs Vite SPA) with the decision and rationale

#### Scenario: React Compiler participation

- **WHEN** the toolchain upgrade completes
- **THEN** the app builds with the React Compiler enabled (opt-in preset) and existing components compile without manual `useMemo`/`memo`

### Requirement: PWA installability as mobile-readiness

React Native and Svelte are explicit non-goals for this change (separate UI runtime; shadcn/Tailwind components cannot be reused). The mobile-native path SHALL be an installable PWA: `vite-plugin-pwa` with web app manifest (name, icons, standalone display) and a service worker providing an app-shell cache so the installed app opens reliably offline.

#### Scenario: App is installable

- **WHEN** a user opens the app in a mobile Chrome/Safari on an HTTPS origin and chooses "Add to Home Screen"
- **THEN** an install prompt/app icon is offered with standalone display and correct icons

#### Scenario: Touch targets and responsiveness (mobile-readiness bar)

- **WHEN** any interactive element is rendered at 360px width
- **THEN** it meets a minimum 40×40px touch target and no horizontal overflow occurs
