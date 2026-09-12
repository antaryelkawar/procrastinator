# Tasks — fix-review-cycle-criticals

## 1. Spec application (backend/docs-neutral)

- [x] 1.1 [backend] Apply the MODIFIED upload-outcome requirement + ADDED review-lifecycle and reprocess-guard requirements from `specs/document-ingestion/spec.md` into `openspec/specs/document-ingestion/spec.md` (replace the stale 201/422/502 flow with 201/202/415/413/502 per delta; add review lifecycle + reprocess guard requirements verbatim from delta). Verify: `openspec validate document-ingestion`.
- [x] 1.2 [backend] Create the new capability spec `openspec/specs/master-feature-doc/spec.md` from `specs/master-feature-doc/spec.md` (3 ADDED requirements). Verify: `openspec validate master-feature-doc`.

## 2. Review-cycle test coverage (backend)

- [x] 2.1 [backend] Audit existing coverage for each `document-ingestion` delta scenario; record a scenario→test map (including gaps + contradicting tests) in `.autopilot/change-fix-review-cycle-criticals/coverage-map.md`. Owns: no product files.
- [x] 2.2 [backend] Fix contradicting tests in `procrastinator-backend/api/reviews_test.go` / `procrastinator-backend/api/documents_test.go` that expect the pre-confidence flow (22-unidentifiable/201-always, missing 202-hold assertions). Only fix where the delta proves the old expectation stale; ambiguous findings go to 2.1 report instead of edits.
- [x] 2.3 [backend] Add missing terminal-immutability coverage in `procrastinator-backend/core/review/service_test.go`: after approve/reject, a further decision mutation returns `ErrConflict`, with both state and `DecidedAt` byte-equal to pre-mutation snapshot. Additionally assert the "Reject terminalizes without creating an Asset" invariant: after reject the document resolves to exactly zero Assets and becomes reprocessable, and a subsequent reprocess restores exactly one Asset.
- [x] 2.4 [backend] Add missing "pending review blocks reprocess" and "reprocess does not duplicate assets" coverage in `procrastinator-backend/api/documents_test.go` (mirroring the existing 202/409 reprocess style): duplicate-asset assertion checks the document resolves to exactly one Asset after reprocess-success.
- [x] 2.5 [backend] Add the 202-hold + `prompt.reprocess_uri` + outage-502-no-partial-writes assertions where not already covered: `procrastinator-backend/api/documents_test.go` (unit) and `procrastinator-backend/e2e/tenancy_e2e_test.go` (e2e 202→list→approve path already exists; extend only if a scenario gap remains).
- [x] 2.6 [backend] OCC baseline lost-update test on the current repository surface (JSONB/link write path): `procrastinator-backend/infra/postgres/` (new `_lostupdate_baseline_test.go`, table-driven, labeled "baseline for payload-link-model-guardrails"). Interleave two fetch-merge-writes; assert/document present behavior honestly. No production-code changes.

## 3. Master doc correctness (docs)

- [x] 3.1 [backend] Target-state markers: in `docs/MASTER_FEATURE_DOC.md`, add the explicit "Resolved target design — NOT yet built in code" marker to every section restating a not-built state (§0, §5.5, any §8 reiteration), and confirm §8 rows match live code (e.g. nav `side="right"` per `procrastinator-backend` UI, `ui/src/components/nav-sheet/nav-sheet.tsx`).
- [x] 3.2 [backend] MONEY-MOVEMENT EXCEPTION consistency: append/point-to §5.1's exception at every zero-FK-columns claim in `docs/MASTER_FEATURE_DOC.md`; remove every unqualified "ALL links embedded"-voice sentence.
- [x] 3.3 [backend] OCC anchor honesty in `docs/MASTER_FEATURE_DOC.md` §5.1: name the governed repository write surface and the exact test/coverage status — `infra/postgres/*_lostupdate_baseline_test.go` (added in 2.6) with its honest outcome, or the `payload-link-model-guardrails` slug if the baseline test cannot assert a passing guard. No phantom test names anywhere in the doc.
- [x] 3.4 [backend] Spot-check verification pass: sample ≥10 factual doc claims (paths, columns, constants) in `docs/MASTER_FEATURE_DOC.md` against the working tree (e.g. `MoneyMovement.LinkedDocumentID` typed in `commons/entity/movement.go:49`, nav `side="right"`); fix each confirmed mismatch in the doc or annotate with its owning change slug; list verified claims in the apply report.

## 4. Validation

- [x] 4.1 [backend] Run `openspec validate --all`; run the touched Go test packages (`go test ./core/review/... ./api/... ./infra/postgres/...` in `procrastinator-backend`) and confirm green; record results in the apply report.
