# Design: openapi-codegen-pipeline

> HOW to implement the `api-contract` capability defined in `proposal.md` and
> `specs/api-contract/spec.md`. This is the architectural approach and the key
> tooling/build decisions; line-by-line work is decomposed in `tasks.md`.

## Context

The Procrastinator HTTP API is currently defined in three places that must be
kept in sync by hand:

- **Server (Go)** — `procrastinator-backend/api/`. The chi router is built in
  `server.go` `Routes()` (23 operations, all under `/api/users/{userId}` via
  path tenancy). Wire DTOs are hand-authored structs in `dto.go`
  (`assetJSON`, `documentJSON`) and `finance_dto.go` (`accountJSON`,
  `movementJSON`, `importBatchJSON`, `importLineJSON`, `importSourceJSON`,
  `commitSummaryJSON`), each with a `to*JSON` converter and duplicate field
  lists. Handlers (`handlers.go`, `finance_*.go`, `households.go`) encode/decode
  these structs and call `httpx.WriteJSON` / `httpx.WriteError` (the
  `{"error": string}` envelope).
- **Client (TypeScript)** — `ui/src/lib/api/`. `types.ts` holds hand-authored
  DTO interfaces mirroring the wire contract; `client.ts` provides generic
  `apiFetch`/`apiJson<T>`/`apiVoid` helpers; `hooks.ts` and `upload.ts` call them
  with resource strings. `config.ts` still exports an unused
  `FINANCE_PATH_PREFIX`. The `X-Tenant-ID` header referenced in the proposal is
  **not** actually sent today — it only appears in negative test assertions
  (`upload.test.ts`). The client already uses path tenancy uniformly.
- **Docs** — informal only.

Build tooling today: the backend is plain `go` (no `Makefile`, no
`go:generate`); the UI is Vite + `tsc` + npm scripts. **There is no CI pipeline,
no `Makefile`, and no Dockerfile** in the repo. So the drift check must be
self-contained in the repo and wired into the existing local/`go test`/`npm test`
entry points rather than a hosted CI job.

Constraints carried into the design:
- The observable HTTP contract (routes, methods, status codes, wire field
  names, `{"error": string}` envelope, snake_case keys, `omitempty`→optional,
  exact-decimal string money, `link_conflicting` always present, `lines` null in
  list responses) is **preserved unchanged**.
- New tooling must be **pinned and deterministic** (byte-for-byte reproducible).
- Budgets: full codegen < 30 s on 2 vCPU/4 GB; document < 200 KB; drift-check
  step < 30 s.

## Goals / Non-Goals

**Goals:**
- Author one committed, valid **OpenAPI 3.1** document that is the sole source of
  truth for every route/operation/schema/error envelope, and keep it in 1:1 lock
  step with the chi router (every registered route ↔ exactly one operation).
- **Generate** the server transport DTOs + route/operation scaffolding from the
  document; replace the hand-written `*JSON` structs (entity→DTO conversion stays,
  now targeting generated types).
- **Generate** the client TypeScript types and type the existing thin fetch
  client against them (no `any`/untyped documented-endpoint bodies).
- **Generate** API documentation from the same document.
- Provide **pinned, reproducible build wiring** (a codegen entrypoint + a drift
  check that fails when committed generated code no longer matches the document).
- Reconcile the stale `FINANCE_PATH_PREFIX` so the client models uniform path
  tenancy.

**Non-Goals:**
- No new endpoints, no behavior/status/wire-shape changes (see proposal
  *Out of Scope*).
- No changes to domain entities, persistence, or business logic — only the HTTP
  transport layer.
- No auth/middleware changes; no real-time/WebSocket/SS-E support; no
  multi-language SDKs; no data migration.

## Decisions

### D1 — Single source of truth: one committed OpenAPI 3.1 YAML document

- **Location:** `procrastinator-backend/api/openapi.yaml`.
- **Format:** YAML (readable, diff-friendly). It must validate against the
  OpenAPI 3.1 schema (spec requirement *Single source-of-truth OpenAPI
  contract*).
- **Content:** 23 operations under a shared server/path prefix
  `/api/users/{userId}`; `components.schemas` for every documented wire type
  (asset, document, account, movement, import-batch, import-line, import-source,
  commit-summary, each request body, and the `Error` envelope); every non-2xx
  response references the `{"error": string}` envelope.
- **Wire-shape fidelity (critical):** the document must encode the exact wire
  conventions so generated code reproduces them:
  - snake_case property names (verbatim JSON keys).
  - `omitempty`/nullable fields → JSON Schema `nullable: true` (3.1) → Go pointer
    / TS optional `?`.
  - Money/price → `type: string` with an exact-decimal `pattern`
    (`^[0-9]+(\.[0-9]+)?$`), never a number.
  - `link_conflicting` → required non-nullable `boolean` (always present).
  - `lines` on the import-batch → nullable array (`type: ["array","null"]`), so
    list responses serialize `null`.
- **Why this over alternatives:** a single committed doc (vs. generating the doc
  from annotations) makes the contract the explicit, reviewable source of truth
  and the drift anchor. Generating the doc from Go annotations would invert the
  flow and still leave the TS SDK + docs to be derived elsewhere; the proposal
  and spec commit to the document as the source of truth.

### D2 — Server codegen: `oapi-codegen` (Go module, pinned)

- **Tool:** `github.com/oapi-codegen/oapi-codegen/v2`, pinned via `go.mod` (Go
  module pinning is inherently reproducible). Invoked via a `go:generate`
  directive in `procrastinator-backend/api` plus a `Makefile` target.
- **What it generates** (into a committed `procrastinator-backend/api/gen/`
  package): the request/response DTO structs (replacing the hand-written
  `*JSON` structs), a `ServerInterface` with one method per operation, and
  `RegisterHandlers(...)` that wires the operations to a router.
- **Router integration:** `RegisterHandlers` drives the existing chi router
  (chi's `Router` satisfies the interface `RegisterHandlers` needs). The current
  `Server` struct's handler methods are refactored to implement the generated
  `ServerInterface` — each generated method is a thin adapter that resolves the
  operation's path/query params from the generated params struct and delegates to
  the existing business logic. `to*JSON` converters become `to<GeneratedType>`
  converters.
- **Multipart:** `POST /documents` and `POST /finance/import-batches` use
  OpenAPI `multipart/formData` request bodies; `oapi-codegen` generates the
  binding types, and the adapters keep the existing size-limit / multipart
  handling.
- **Why oapi-codegen:** it is the standard Go OpenAPI generator that produces
  *both* typed DTOs and per-operation route/operation scaffolding with a router
  registration hook — exactly what the spec's *Server-side code generation*
  requirement demands (a new operation yields handler scaffolding with no manual
  signature edit). Alternatives (hand-rolling types, `swaggo` reflection-based
  annotations) either don't give per-operation scaffolding or generate the
  contract from code, which conflicts with the document-as-source-of-truth goal.

### D3 — Client codegen: `openapi-typescript` (npm devDependency, pinned) + typed wrapper

- **Tool:** `openapi-typescript`, pinned as an npm `devDependency` (exact
  version, not a range, so output is reproducible). Invoked via a `ui` npm
  `codegen` script.
- **What it generates:** a committed `ui/src/lib/api/generated/paths.d.ts`
  exporting the `Paths`, `Operations`, and `Components["schemas"]` types derived
  from the same `openapi.yaml`.
- **Typed fetch client:** the existing `apiFetch`/`apiJson`/`apiVoid` helpers are
  re-typed against the generated `Paths` type so documented endpoints are typed
  (request body + response schema), eliminating `any`/untyped bodies while
  preserving the current thin-client shape and its tests. `hooks.ts` calls become
  typed against the generated response/request types in place of the
  hand-authored `types.ts` interfaces, which are then removed.
- **Why over orval / openapi-fetch:** the UI already has a bespoke, tested thin
  fetch client. `openapi-typescript` (types-only) slots in with the smallest
  change and keeps the existing client + its test suite intact, while still
  producing a "typed fetch client" (generated types + typed wrapper). A full
  client generator (orval / openapi-fetch) would force replacing the working
  client and its tests for marginal benefit.

### D4 — Docs codegen: static, committed, deterministic generator

- **Tool:** a static OpenAPI→Markdown/HTML docs generator (default: `redoc-cli`
  to a single-file static HTML, or `openapi2md` to Markdown), pinned (exact
  version) and invoked from the `codegen` entrypoint.
- **Output:** committed static docs under `docs/api/` (or
  `procrastinator-backend/api/docs/`), covering every operation (method, path,
  params, request/response schemas, error responses).
- **Determinism:** the generator output must be byte-for-byte stable — the
  design pins the tool version and, if the tool embeds a build timestamp or
  version banner, the wiring strips/pins it so re-runs are identical (spec
  scenario *Doc generation is idempotent*).
- **Why static committed docs (vs. a served Swagger UI):** a served UI is not a
  committed artifact and can't be drift-checked or rendered offline; static
  committed docs satisfy *Documentation generated from the contract* and are
  reviewable in diff.

### D5 — Build wiring + drift check (no hosted CI exists)

- **Single entrypoint:** a top-level `Makefile` with targets:
  - `make codegen` — runs all generators (Go types, TS types, docs) with the
    pinned tools/flags and writes into their committed locations.
  - `make codegen-drift-check` — re-runs the generators into a scratch/temp
    location, diffs against the committed artifacts, and exits non-zero with a
    message naming each stale generated file if they differ.
  - `make docs` — docs only.
- **Local/`go`/`npm` hooks:**
  - `procrastinator-backend/api` carries a `//go:generate` directive running the
    Go generator.
  - `ui/package.json` gains a `codegen` npm script (TS types + docs) and a
    `codegen:check` script for the TS/docs drift check.
  - The drift check is also reachable from the existing test entry points — a
    backend `go test` (in the `api` package) and a UI `vitest` case each assert
    "regen == committed" — so the check runs wherever the project's tests run.
- **Why:** with no CI, the drift check must live in the repo and be reachable
  from the commands developers/agents already run. The scratch-regen-and-diff
  approach is what the spec's *Reproducible generation and drift detection*
  scenarios require (stale → fail naming the file; re-run → pass; fresh clone
  builds from committed artifacts).
- **Fresh-clone guarantee:** because generated artifacts are committed, a clean
  clone builds without running codegen; the drift check then reports them in
  sync (spec scenario *Fresh clone builds from committed artifacts*).

### D6 — Tenancy reconciliation (minimal)

- The client already routes every call under `/api/users/{userId}`. The only
  stale artifact is the unused `FINANCE_PATH_PREFIX` const in `config.ts`.
- **Action:** remove `FINANCE_PATH_PREFIX`; confirm the generated contract models
  all operations under the `/api/users/{userId}` path prefix with no
  header-tenancy special-case. No wire change; the negative `X-Tenant-ID` test
  assertions in `upload.test.ts` remain valid.

### D7 — Wire-shape mapping table (contract fidelity)

A fixed mapping is used when authoring the document so generated Go/TS types
reproduce the current wire exactly:

| Wire convention (today) | OpenAPI 3.1 in doc | Generated Go | Generated TS |
|---|---|---|---|
| snake_case JSON key | property name verbatim | struct field w/ json tag | property verbatim |
| `omitempty` / nullable | `nullable: true` | pointer (`*T`) | optional (`?`) |
| money/price (exact decimal string) | `type: string` + decimal `pattern` | `string` | `string` |
| `link_conflicting` always present | required non-nullable `boolean` | `bool` (non-pointer) | required `boolean` |
| `lines` null in list responses | `type: ["array","null"]` | nullable slice (`*[]T` / nil-able) | `T[] \| null` |
| `metadata` `{}` never null | non-nullable object | non-pointer map | `Record<string,string>` |
| date `occurred_on` | `type: string`, `format: date` | `string` (formatted in converter) | `string` |
| RFC3339 timestamps | `type: string`, `format: date-time` | `time.Time` | `string` |

Entity→DTO conversion (`to*`) remains hand-written but targets the generated
types; this is where the date formatting and `metadata`-init / `lines`-nil
semantics are preserved.

## Risks / Trade-offs

- **[Generated types don't exactly reproduce the wire contract → UI breaks]** →
  Mitigate with the D7 mapping table enforced at document authoring, plus the
  drift check and the existing UI/backend test suites (which assert exact wire
  shapes) as the guard. Any mismatch fails the build before it ships.
- **[oapi-codegen changes handler signatures → large refactor surface]** →
  Mitigate by making each generated method a thin adapter over the existing
  handler logic (logic unchanged, only the entry signature + DTO target change),
  and doing it per-endpoint with tests green after each.
- **[A docs generator embeds timestamps/versions → non-deterministic output]** →
  Pin the exact tool version and strip/pin any volatile banner so re-runs are
  byte-identical (verified by the *Doc generation is idempotent* scenario).
- **[No CI to enforce the drift check automatically]** → The drift check is
  wired into `go test`/`npm test`/`make codegen-drift-check` so it runs in the
  existing local test flow; if CI is introduced later, it is a one-line addition.
- **[Full codegen could exceed the 30 s budget]** → The document is small
  (< 200 KB, 23 operations); the tools are fast single-pass generators. The
  budget is asserted by a task/CI timing check; if a tool is slow, the
  alternative is a lighter TS generator (types-only is already the choice).
- **[Two Go/TS tools must stay version-stable]** → Versions are pinned (go.mod +
  exact npm devDependency), so output is reproducible across machines and time;
  upgrades are explicit, deliberate changes that regenerate + commit.

## Migration Plan

1. Author `openapi.yaml` by transcribing the current 23 operations + schemas
   (using the D7 mapping table) and validate it as OpenAPI 3.1.
2. Add pinned tooling: `oapi-codegen` (go.mod + `go:generate`),
   `openapi-typescript` (npm devDependency), and the pinned docs generator.
3. Generate + commit the server `gen/` package; refactor handlers to implement
   the generated `ServerInterface` and target generated DTOs; remove the
   hand-written `*JSON` structs. Keep tests green.
4. Generate + commit the client `paths.d.ts`; re-type `client.ts` and `hooks.ts`
   against generated types; remove `types.ts` and the unused `FINANCE_PATH_PREFIX`.
5. Generate + commit the docs.
6. Add the `Makefile` entrypoints + the drift check (scratch regen + diff) and
   wire it into `go test` / `npm test`.
7. Run the drift check clean; confirm a fresh clone builds from committed
   artifacts.

**Rollback:** each phase is independently reversible — the document, generated
packages, and build wiring are additive; reverting to the hand-written DTOs /
`types.ts` and removing the codegen targets restores the prior state. No data
migration is involved.

## Open Questions

- **Exact pinned versions** of `oapi-codegen`, `openapi-typescript`, and the docs
  generator will be fixed at implementation (first release that produces
  deterministic output); recorded in the `Makefile`/`go.mod`/`package.json` and
  asserted by the determinism scenarios.
- **Docs generator choice** (`redoc-cli` HTML vs `openapi2md` Markdown) — decide
  by whichever yields clean byte-stable output for the current document; both are
  within scope.
- **Whether the drift check should hard-fail `go test`/`npm test` or be a
  separate explicit target** — default: hard-fail in tests (strongest guarantee),
  with the explicit `make codegen-drift-check` target for developers.
