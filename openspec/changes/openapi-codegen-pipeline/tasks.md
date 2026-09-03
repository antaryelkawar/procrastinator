# Tasks: openapi-codegen-pipeline

Implementation checklist for the `api-contract` capability. Ordered by
dependency. Each task is verifiable; it maps to a requirement/scenario in
`specs/api-contract/spec.md` and an approach in `design.md`.

## 1. OpenAPI source-of-truth document

- [x] 1.1 Add `procrastinator-backend/api/openapi.yaml` as a valid OpenAPI 3.1 document with the shared `/api/users/{userId}` path prefix (design D1). *Verifies: Document validates as OpenAPI 3.1.*
- [x] 1.2 Document all 23 operations (document upload, assets, asset documents, finance accounts, finance movements + link/unlink, finance import-batches + commit/discard, households + add-member) with method, path, parameters, and request/response references. *Verifies: Existing endpoint families are all covered; Routes and operations match exactly.*
- [x] 1.3 Define `components.schemas` for every wire type (asset, document, account, movement, import-batch, import-line, import-source, commit-summary, each request body) using the D7 mapping table (snake_case keys, `nullable` for `omitempty`, exact-decimal string money pattern, `link_conflicting` required boolean, `lines` nullable array, `metadata` non-nullable object).
- [x] 1.4 Define the `Error` envelope schema (`{"error": string}`) and reference it from every non-2xx response in the document. *Verifies: Error envelope is documented.*
- [x] 1.5 Add a validation step that fails if `openapi.yaml` is not valid OpenAPI 3.1 or exceeds 200 KB. *Verifies: Document validates as OpenAPI 3.1; OpenAPI document is within size budget.*

## 2. Pinned codegen tooling

- [x] 2.1 Add `github.com/oapi-codegen/oapi-codegen/v2` to `procrastinator-backend/go.mod` (pinned version).
- [x] 2.2 Add `openapi-typescript` to `ui/package.json` as an exact-pinned devDependency.
- [x] 2.3 Add the pinned static docs generator (redoc-cli or openapi2md) to the build wiring.
- [x] 2.4 Record the pinned tool versions + flags in one place (Makefile / go:generate / npm script) so invocations are reproducible. *Verifies: Generation is deterministic (pinned inputs).*

## 3. Server code generation (design D2, D7)

- [x] 3.1 Add a `//go:generate` directive in `procrastinator-backend/api` running the pinned `oapi-codegen` against `openapi.yaml` into a committed `procrastinator-backend/api/gen/` package.
- [x] 3.2 Generate the Go DTO structs for every documented schema; confirm field names, JSON keys, and optionality match the wire contract (pointer for `omitempty`, string money, non-pointer `link_conflicting`, nullable `lines`). *Verifies: Generated types match the wire contract.*
- [x] 3.3 Generate the per-operation `ServerInterface` and router `RegisterHandlers` wiring.
- [x] 3.4 Refactor the existing handler methods (`handlers.go`, `finance_*.go`, `households.go`) to implement the generated `ServerInterface` as thin adapters; move the entity→DTO `to*` conversion to target the generated types (preserve date formatting, `metadata` `{}` init, and `lines` nil semantics).
- [x] 3.5 Delete the hand-written wire-DTO structs (`assetJSON`, `documentJSON`, `accountJSON`, `movementJSON`, `importBatchJSON`, `importLineJSON`, `importSourceJSON`, `commitSummaryJSON`) from `dto.go` / `finance_dto.go` once handlers use the generated types. *Verifies: Handlers use the generated types (no duplicate wire-DTO struct).*
- [x] 3.6 Confirm the backend compiles and all existing `api` tests pass with the generated types in place.
- [x] 3.7 Add a test that a new operation added to `openapi.yaml` + re-running the generator produces handler scaffolding for it with no hand-written signature edit. *Verifies: New operation yields handler scaffolding.*

## 4. Client TypeScript SDK (design D3, D6)

- [x] 4.1 Add a `ui` npm `codegen` script running the pinned `openapi-typescript` against `openapi.yaml` into a committed `ui/src/lib/api/generated/paths.d.ts`.
- [x] 4.2 Confirm the generated types match the wire contract (snake_case, `?` optional for `omitempty`, string money, required `link_conflicting`, `lines: T[] | null`). *Verifies: Generated TS types match the wire contract.*
- [x] 4.3 Re-type `client.ts` (`apiFetch`/`apiJson`/`apiVoid`) against the generated `Paths` type so documented endpoints are typed with no `any`/untyped bodies. *Verifies: Client is typed against the SDK.*
- [x] 4.4 Replace `hooks.ts` and `upload.ts` usages of the hand-authored `types.ts` interfaces with the generated types; remove `ui/src/lib/api/types.ts`.
- [x] 4.5 Remove the unused `FINANCE_PATH_PREFIX` from `config.ts` and confirm uniform path tenancy (all calls under `/api/users/{userId}`); keep the `X-Tenant-ID` negative assertions in `upload.test.ts` passing. *Verifies: tenancy reconciliation (design D6).*
- [x] 4.6 Confirm the UI builds (`tsc -b`) and all existing `ui` API tests pass against the generated types.
- [x] 4.7 Add a test that adding a field to a response schema + re-running the generator adds it to the generated TS type and the client compiles. *Verifies: Contract field flows to the client.*

## 5. Documentation generation (design D4)

- [x] 5.1 Wire the pinned docs generator into the codegen entrypoint, emitting committed static docs covering every operation (method, path, params, request/response schemas, error responses). *Verifies: Docs cover all operations.*
- [x] 5.2 Strip/pin any volatile metadata (timestamps/version banners) so doc output is byte-for-byte stable. *Verifies: Doc generation is idempotent.*
- [x] 5.3 Add a test that regenerating after a document change reflects the added operation/field. *Verifies: Docs track document changes.*

## 6. Build wiring + drift check (design D5)

- [x] 6.1 Add a top-level `Makefile` with `codegen` (all generators), `docs`, and `codegen-drift-check` targets using the pinned tools/flags.
- [x] 6.2 Implement the drift check: re-run all generators into a scratch location, diff against committed artifacts, and exit non-zero naming each stale generated file on mismatch. *Verifies: Stale generated code fails the build.*
- [x] 6.3 Wire the drift check into the existing test entry points (a `go test` in `api` and a `vitest` case in `ui`) asserting "regen == committed".
- [x] 6.4 Verify determinism: run all generators twice against the same document and assert byte-for-byte identical output. *Verifies: Generation is deterministic.*
- [x] 6.5 Verify the re-run clears drift (edit doc, re-run, commit → drift check passes) and that a fresh clone builds from committed artifacts with the drift check reporting in-sync. *Verifies: Re-running codegen clears the drift; Fresh clone builds from committed artifacts.*

## 7. Budgets (design D5, spec *Codegen performance and size budgets*)

- [x] 7.1 Add a timing check/assertion that a full codegen run (Go + TS + docs) completes under 30 s on a 2 vCPU / 4 GB runner. *Verifies: Full codegen run is within time budget.*
- [x] 7.2 Assert the drift-check step adds no more than 30 s to the pipeline (measured via the same timing harness). *Verifies: Drift check is within CI time budget.*
- [x] 7.3 Re-confirm the committed `openapi.yaml` is under 200 KB in the drift/build gate. *Verifies: OpenAPI document is within size budget.*

## 8. Final verification

- [x] 8.1 Run the full drift check clean; confirm routes↔operations are in exact 1:1 lockstep with `openapi.yaml`. *Verifies: Routes and operations match exactly.*
- [x] 8.2 Run the complete backend + UI test suites green; confirm no observable wire/status/behavior change. *Verifies: PRESERVED contract (proposal).*
- [x] 8.3 Update `openspec status` / validate the change; confirm all `api-contract` scenarios are exercised by at least one task.
