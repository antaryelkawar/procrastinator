# Design — fix-review-cycle-criticals

## Context

The cycle-1 review gate closed the master feature doc FAIL on paper: (a) R1-LOSTUPDATE
needs a testable OCC guardrail baseline now (implementation is change #3 /
`payload-link-model-guardrails`, out of scope here); (b) COH-R5 requires doc
target-vs-built honesty; (c) the OpenSpec `document-ingestion` capability still
documents the pre-confidence upload flow (`201`/`422`/`502`) while live code
implements the confidence-gated flow — `202` hold, review queue, approve/reject
terminalizing with 409-once semantics, and `reprocess` with an in-flight 409 guard
(`procrastinator-backend/api/documents/documents.go`, `core/review/service.go`,
`api/reviews_test.go`, `api/documents_test.go`, `e2e/tenancy_e2e_test.go`).

Live code paths (verified in grounding):
- `commons/entity/review.go` — `IngestReview`, state vocabulary pending/approved/rejected, nullable `DecidedAt`.
- `core/review/service.go` — approve/reject with `ErrConflict` → 409; single transaction.
- `api/documents/documents.go` — reprocess handler: refuses when document `in_review`, in-flight guard via pending review on the source, persists resolved asset.
- `api/deployments`: tests `api/reviews_test.go` (re-approve → 409 at line ~162), `api/documents_test.go` (first reprocess 202, second 409, lines 198–206), `e2e/tenancy_e2e_test.go` (held upload 202 → list → approve).

## Goals / Non-Goals

**Goals:**
- Restate OpenSpec `openspec/specs/document-ingestion/spec.md` upload outcomes to the live confidence-gated flow (via the approved delta) and add the review-lifecycle + reprocess-guard requirements.
- Verify/fix existing backend test coverage so every delta scenario is exercised; fix tests that contradict the delta (these are bugs, per proposal).
- Reconcile `docs/MASTER_FEATURE_DOC.md` with code: target-state markers, MONEY-MOVEMENT EXCEPTION alongside every zero-FK claim, OCC requirement with an honest verification anchor (no phantom test names).
- Establish the testable OCC lost-update guardrail on today's repository surface (baseline for change #3) — a test only, no OCC implementation.

**Non-Goals:**
- No OCC implementation (change #3), no nav move, no new endpoints, no UI, no schema migrations.

## Decisions

1. **Spec-first ordering**: apply the `document-ingestion` delta to `openspec/specs/` before touching tests, so test fixes are judged against the restated spec, not memory. Alternative (tests first) risks encoding the stale flow again.
2. **Coverage strategy — fix, don't re-spec**: for each delta scenario, map to an existing test (`api/reviews_test.go`, `api/documents_test.go`, `core/review/service_test.go`, `e2e/tenancy_e2e_test.go`). Missing scenario → add a focused unit test in the owning package; contradicting test → fix the test. No new endpoints to test against.
3. **Terminal-state immutability** is asserted via the existing `ErrConflict` path in `core/review`; the new test asserts both state and `DecidedAt` unchanged after a refused mutation (service level, no HTTP).
4. **Reprocess asset-replacement guarantee** ("no duplicate assets") is asserted at the `api/documents` handler level using the existing in-memory/pooled test setup, mirroring `documents_test.go` style — reprocess twice-after-terminal must not leave a second asset.
5. **OCC baseline test**: one table-driven test (`commons`/`infra` repo or service test as fitting the existing layout) pins the lost-update behavior currently observable: two fetch-merge-write interleavings on the repository the doc calls load-bearing (JSONB/link writes on documents/entities). It documents present behavior honestly (failing = the bug change #3 fixes, unless a cheap guard exists today). The doc §5.1 anchor names this test file and its status; **no doc sentence may claim OCC tests exist until they pass**.
6. **Doc corrections only in `docs/MASTER_FEATURE_DOC.md`**: add "NOT yet built" markers next to target-state restatements (nav left-align §0/§5.5/§8), append the MONEY-MOVEMENT EXCEPTION to every zero-FK claim, and rewrite the §5.1 OCC paragraph to point at a real test file or the `payload-link-model-guardrails` slug with status.
7. **master-feature-doc verification is executable**: the spot-check scenarios are validated by a task that greps doc claims against the tree (e.g. `side="right"` in nav-sheet, `LinkedDocumentID` in `commons/entity/movement.go`) and records results in the change report — not by an automated CI test (doc markdown; no CI doc-lint infra exists).

## Risks / Trade-offs

- [Test rewrite risks hiding a real behavior bug as a "test fix"] → Each contradicting test is fixed only when the delta spec + live code prove the test expected the stale flow; ambiguous cases are flagged in the apply report instead of silently changed.
- [OCC baseline test pins broken behavior] → The test is explicitly labeled baseline-for-change-#3; if today's code allows lost updates where the doc is load-bearing, the test failure/`t.Skip` with reason is documented, and case #6 doc wording stays "not yet covered".
- [Doc spot-check is manual/greppable, not CI-enforced] → Accepted: scope guard forbids building doc-lint infra; the change report records the verified claims.
- [OpenAPI client pact drift when 202 responses read differently] → No route/schema changes this change; only assertions are added, so generated `api/gen/openapi.gen.go` is untouched.

## Migration Plan

Pure consolidation: apply specs → fix/extend tests → edit doc → validate (`openspec validate`). Rollback = revert the change commit; no data or schema movement.

## Open Questions

None. All target files verified in the working tree.
