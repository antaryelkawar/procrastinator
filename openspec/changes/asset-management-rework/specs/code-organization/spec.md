## ADDED Requirements

### Requirement: Backend api/ organized into domain subpackages

The flat `api/` handler layer SHALL be split into domain subpackages grouped by
resource/flow (at minimum: `api/assets`, `api/add`, `api/search`, `api/lifecycle`; and —
where the existing surface warrants it — `api/documents`, `api/reviews`, `api/finance`,
`api/household`). Handlers SHALL NOT remain as a flat bag of files under the root
package; the root `api/` package SHALL retain only the composition/server scaffolding,
the generated surface (`api/gen`), and the contract documents.

#### Scenario: Handler files live in domain subpackages

- **WHEN** the modular rework is complete
- **THEN** `api/<domain>` packages carry the handlers for their domain
- **AND** `api/` root contains no domain handler file left in the flat layout

#### Scenario: Behavior is preserved through the restructure

- **WHEN** the restructure is applied
- **THEN** the existing operation-to-route mapping and responses are unchanged
  (the OpenAPI contract stays the source of truth)

### Requirement: core/ subpackage boundaries verified

The `core/` subpackages SHALL be verified as bounded (`identity`, `processing`,
`lifecycle`, `search`, `review`, `statement`) — each SHALL own its domain behavior with
no cross-domain behavioral code smeared into a sibling package (imports of shared types/
ports notwithstanding). The verification SHALL be recorded as part of the change
(evidence: package responsibility statements + the existing tests that pin behavior).

#### Scenario: Every core/ subpackage documents its behavior

- **WHEN** the verification runs
- **THEN** each core/ subpackage has a recorded responsibility and the behavior it owns
  is attributable to it (no behavior moved between subpackages without being noticed)

#### Scenario: Boundaries hold after modular reorganization

- **WHEN** cycle-2 views/actions land
- **THEN** domain logic stays in its owning `core/` subpackage rather than leaking into
  `api/` handlers or a new package

### Requirement: UI organized by feature folders with co-located hooks/components

The UI SHALL reorganize `src/pages/` into **feature folders** under `src/features/`
(at minimum: `features/add`, `features/search`, `features/assets`, `features/reviews`,
`features/finance`), each containing that feature's page(s) **plus its feature-specific
hooks and components co-located inside the folder**. `src/components/` SHALL hold only
shared primitives/design-system pieces (buttons, dialogs, table, upload, feedback,
layout); a feature-specific component SHALL NOT live at the shared level. Route bindings
SHALL point at the feature folders.

#### Scenario: Feature code is co-located

- **WHEN** the reorganization lands
- **THEN** a feature's page, its feature-specific hooks, and its feature-specific
  components all live in that feature's folder

#### Scenario: Shared level holds primitives only

- **WHEN** the reorganization lands
- **THEN** `src/components/` contains shared primitives and the layout/design system
  with no feature-specific page/update code left there

### Requirement: Restructure leaves the workflow verifiable

The modular reorganization SHALL be executed without dropping existing verification:
backend (`go build ./... && go test ./...`) and UI (`npm run test && npm run build`) SHALL
pass after the restructure, and the OpenAPI/codegen drift gates SHALL still apply to the
contract (file location changes notwithstanding).

#### Scenario: Checks still pass post-restructure

- **WHEN** the restructure completes
- **THEN** the backend test suite, the UI suite, and the codegen drift gates all pass
