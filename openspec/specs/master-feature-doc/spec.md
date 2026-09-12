# master-feature-doc

## Purpose

The authoritative master feature document (`docs/MASTER_FEATURE_DOC.md`) is the project's single source of truth for architecture and behavior. This capability pins the document's correctness: every stated-fact claim about live code must be verifiable against the working tree, target states that are not yet built must be explicitly marked as targets rather than asserted as current fact, the schema and link-model decisions (including the MONEY-MOVEMENT EXCEPTION) must be stated consistently in every section, and the OCC guardrail must be documented alongside verifiable test coverage rather than phantom test references.

## Requirements

### Requirement: No target state stated as current fact

`docs/MASTER_FEATURE_DOC.md` SHALL never describe a resolved architectural *target*
state in the same factual voice as verified current behavior without an explicit
target marker (e.g. "Resolved target design, §5.5 — NOT yet built in code") in the
same section, cross-referenced where §0/§2 restates it. Every factual claim about
live code in the doc (column existence, control flow, API shape, UI structure) SHALL
be verifiable against the working tree; a claim that no longer holds SHALL be
corrected or explicitly marked as the target of a tracked change with its change
slug.

#### Scenario: Target state is marked as such next to every restatement

- **WHEN** a resolved decision describes a state that does not exist in live code
  (e.g. left-aligned nav sheet)
- **THEN** every section that asserts that state (including §0 and any summary
  section) carries the explicit "not yet built" marker, and the §8 log row
  describing the actual current code matches what the code shows

#### Scenario: Verified-fact claims survive a spot-check

- **WHEN** a reviewer spot-checks a sample of doc claims written in the factual
  voice (file, path, constant, column, or behavior) against the live tree
- **THEN** each sampled claim is confirmed; any confirmed mismatch is treated as a
  correctness bug in the change and fixed in the doc or flagged to a owning change

### Requirement: Structured-link decisions stated without contradiction

The master feature doc SHALL state its link-model decision consistently: links on
new/derived entities are denormalized inside JSONB payloads with zero FK columns
or link tables for **new** entities, while the MONEY-MOVEMENT EXCEPTION keeps
`MoneyMovement.LinkedDocumentID` as a live typed column that is NOT collapsed into
JSONB. This exception SHALL be stated in every section that describes the
"zero FK columns" rule, and no section SHALL assert "all links are embedded" in an
unqualified voice.

#### Scenario: MONEY-MOVEMENT EXCEPTION accompanies every zero-FK claim

- **WHEN** the doc states the denormalized-JSONB link model or the "zero FK
  columns" rule
- **THEN** the statement either names the MoneyMovement exception or points to the
  monetized exception paragraph in §5.1, and no unqualified "ALL links embedded"
  sentence remains

#### Scenario: Documented column facts match the schema

- **WHEN** the doc asserts the presence/absence of a live column
  (e.g. `MoneyMovement.LinkedDocumentID` kept; `Document.AssetID` deprecated)
- **THEN** the entity definition and the migration schema in the working tree
  agree with the assertion

### Requirement: OCC guardrail documented with verifiable coverage anchor

The master feature doc SHALL state the OCC hard requirement (no silent lost
updates on JSONB/link writes) alongside an explicit pointer to the testability of
that requirement: which entity repository surface it governs and which existing
test coverage (or, if none exists yet, which named change will add it — the
`payload-link-model-guardrails` change) demonstrates it. The doc SHALL NOT
describe OCC tests as existing when they do not.

#### Scenario: OCC claim names its verification

- **WHEN** the doc's §5.1 guardrail names mandatory OCC
- **THEN** a reader can find, in the same or a referenced section, the name of the
  repository write surface it governs and the exact status of current test
  coverage (present tests, or the change slug that adds them and its status)

#### Scenario: No phantom test references

- **WHEN** any doc, checklist, or spec claims that OCC behavior is covered by
  named tests
- **THEN** each named test file exists in the working tree and actually exercises
  a lost-update-style scenario; a claim that fails verification is corrected in
  this change
