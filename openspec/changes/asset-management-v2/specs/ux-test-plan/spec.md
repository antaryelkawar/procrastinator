# ux-test-plan — Delta

## ADDED Requirements

### Requirement: UX test plan is a mandatory part of the implementation plan

The implementation plan (design + tasks for this change) SHALL contain a **UX-oriented test plan** as a gate: implementation is not complete until every item in the plan has been executed and passes. The plan SHALL cover:

1. **Devices/viewport**: mobile 360×640 and desktop ≥1280 (one mid-size check at 768 optional).
2. **Flows**:
   - chrome: no top bar anywhere; top-left hamburger opens/closes the sheet on every view; sheet content = profile → Home → Review Queue/Assets/Documents/Accounts → theme toggle (light/dark/system, no flash, persisted) + disabled TBD rows; Home entry returns to `/` from every view; wordmark tap targets `/`; landing has no corner [+];
   - routes: deep-link of every pre-reorg legacy route resolves to redirect-or-404; walk-all-links produces no dead links; `/assets/:id` deep-link renders;
   - context [+]: on `/ingest/reviews`, `/assets`, `/documents`, `/finance/accounts` the top-right [+] opens that section's add flow in the composer pattern with a ≥44px target and an action label;
   - unified Add composer: landing [+] → bottom sheet (mobile) / inline (desktop) with describe-input + 📷/📁/⌨ attachment strip + optional directive note; submissions per type (camera, file, paste-text) reach the pipeline; chip add/remove-before-submit; ≥360px no overflow;
   - review & repair: extracted fields render as confirmable chips, correctable items, collapsed "add details" — no blank-field form grid; user can save unedited;
   - landing → composer ingest (with + without directive note) → review; landing search → results → asset detail → edit;
   - documents: list/filter/reprocess(+comment)/delete/edit-asset/delete with confirm; documents corner [+] composer submit incl. duplicate → reprocess/keep prompt;
   - finance accounts/movements/import; reviews queue accept/discard; duplicate upload prompt (both choices); dark mode on every flow.
3. **Quality bar**: no horizontal overflow at 360px; ≥40px touch targets; no unreadable contrast in light or dark; no layout shift on theme/system changes; loading skeletons for async data; keyboard focus visible on desktop; toasts confirm destructive-adjacent actions.

Existing flows whose behavior is not touched by delta 2 keep their previously planned rows; chrome/composer/route rows are added/updated in place so the matrix stays consistent with the delta-2 specs.

Each item SHALL be enumerated as a checklist row (flow × viewport × mode) with pass/fail and evidence noted where done manually; automatable rows SHALL be covered by vitest/browser checks where feasible.

#### Scenario: Plan contains the gate

- **WHEN** the tasks of this change are generated
- **THEN** a dedicated "UX test plan" task group appears with per-flow checklist items covering all rows of the matrix above, before the "apply complete" gate

#### Scenario: Manus runs the audit

- **WHEN** implementation finishes
- **THEN** every checklist row is pass-verified (automated or manually documented with screenshots) before the change is marked applied

#### Scenario: Failure blocks apply

- **WHEN** any checklist row fails (e.g. a 360px horizontal scroll in the documents list)
- **THEN** the apply gate does not pass until that row is fixed and re-verified

#### Scenario: Delta-2 chrome rows are audited

- **WHEN** the audit matrix is executed after delta 2
- **THEN** the route-surface walk (legacy → redirect/404, no dead links), chrome rows (no top bar, top-left hamburger, sheet w/ Home + profile + theme toggle), and composer rows (landing [+] unified composer, context [+] per section, review-chip pattern) are all present and executed

#### Scenario: Audit matrix keeps prior rows honest

- **WHEN** the delta-2 matrix supersedes the old matrix
- **THEN** rows for flows unchanged by delta 2 (duplicate prompt choices, documents actions, finance, reviews, dark mode) are carried over — not silently dropped
