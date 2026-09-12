# Proposal — fix-review-cycle-criticals

## Why

Change #1 in `docs/OPENSPEC_CHANGES_CHECKLIST.md`. The reviewer gate that resolved
`docs/MASTER_FEATURE_DOC.md` (cycle-1 FAIL, closed 2026-09-11) surfaced three
critical findings; two of them were closed **on paper only**, and the review-cycle
is itself drifting from its governing spec:

1. **R1-LOSTUPDATE (critical)** — the repository's `Update` is a
   fetch-then-merge-then-write with no OCC. The master doc now *declares* OCC a
   hard requirement, but nothing pins that declaration to code behavior; change #3
   (`payload-link-model-guardrails`) will implement mandatory OCC. This change
   establishes the **testable guardrail now** on whatever replacement/OCC coverage
   exists today, so #3 starts from a verified baseline rather than a wish.
2. **COH-R5 (critical)** — the nav-sheet "target ≠ built" distinction must stay
   honest in documentation: no doc section may describe a target state as current
   fact. The cycle-1 closure edits were applied hastily (a mid-edit UTF-8 corruption
   was already repaired); the coherence of every doc claim about live code is
   unverified.
3. **Spec/code drift in the review cycle itself (discovered in this change's
   grounding):** the main `document-ingestion` OpenSpec capability still specifies
   the pre-confidence upload flow (`201` commit / `422` unidentifiable / `502`
   outage), while live code implements the confidence-gated flow — uploads held for
   review respond `202`, low-confidence extractions land in the review queue, and
   the documents section exposes `reprocess`/`keep`/`delete` with in-flight
   (409) guards. There is **no capability spec for the ingest review cycle**
   (hold → list → approve / reject / reprocess → terminal states) anywhere in
   `openspec/specs/`. Behavior parity for approve-once-only, re-approve conflict,
   and reprocess in-flight conflict is enforced only by scattered unit tests.

## What Changes

- **Delta `document-ingestion` (MODIFIED):** restate upload outcomes to match the
  live confidence-gated flow (commit `201`, held-for-review `202`, still `415`/`413`
  validation) and add the missing review-cycle requirements: review hold + queue
  listing, approve-once-only with conflict, reject, decision immutability after a
  terminal state, and document `reprocess` with its in-flight 409 guard. Behavior
  correctness only — no new endpoints, no UI.
- **Delta `master-feature-doc` (NEW capability: openspec/specs does not yet
  document the doc itself — this change creates it):** require that the
  authoritative master feature doc never contradicts live code in a stated-fact
  voice, that target-not-yet-built states are explicitly marked, that the
  schema decisions (MONEY-MOVEMENT EXCEPTION: `MoneyMovement.LinkedDocumentID`
  stays a typed column; zero FK columns for *new* entities) are stated
  consistently everywhere, and that the OCC guardrail is documented alongside test
  coverage proving it is honored where load-bearing today.
- **Test/verification coverage baseline:** scenarios below are exercised by the
  existing backend tests where present; any existing coverage that contradicts the
  restated scenarios is a bug and gets fixed, not spec'ed around.

## Impact

- **Scope guard:** correctness consolidation only. **No** OCC implementation
  (that is change #3's subject via `payload-link-model-guardrails`), **no** nav
  move, **no** new endpoints, **no** new features.
- Affected: `openspec/specs/document-ingestion` (via delta),
  `docs/MASTER_FEATURE_DOC.md` (doc-side corrections only),
  existing backend tests around `core/review`, `api/documents`, `api/reviews`
  (fix mismatches, add missing 409/terminal-state coverage if absent).
- Downstream consumers: change #3 inherits a verified OCC baseline; changes #10–#12
  build on a correctly-spec'ed review queue.

## Grounding

- `.tmp/reviewer-subspec-master-feature-doc-20260911.md` (3 CRITICAL: COH-R5,
  R1-JSONB-SCHEMA, R1-LOSTUPDATE)
- `docs/MASTER_FEATURE_DOC.md` §5.1 (MONEY-MOVEMENT EXCEPTION, OCC hard
  requirement), §5.5 + §8 (nav target-vs-built)
- Live code checks (2026-09-11): `ui/src/components/nav-sheet/nav-sheet.tsx:141`
  `side="right"` (doc §8 correctly says not-built); `@procrastinator-backend`
  repository `Update` has no version/OCC marker; `MoneyMovement.LinkedDocumentID`
  live and typed (`commons/entity/movement.go:49`, `payload_codec.go:713`,
  `core/ledger/service.go:294-330`); review endpoints `'.../ingest/reviews/{id}/approve'`
  re-approve → 409 (`api/reviews_test.go:162`); reprocess first-call `202`,
  second-call `409` (`api/documents_test.go:198-206`).
- `openspec/specs/document-ingestion/spec.md` (stale flow described above).
