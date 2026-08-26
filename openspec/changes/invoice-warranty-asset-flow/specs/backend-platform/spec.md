# Spec: backend-platform

Delta for change `invoice-warranty-asset-flow` (v3). Modifies the baseline in `openspec/specs/backend-platform`.

## MODIFIED Requirements

### Requirement: Procrastinator naming

The backend SHALL use `procrastinator` naming consistently: the binary SHALL be `procrastinator` (built from `api/cmd/procrastinator`), the configuration struct SHALL be `Config`, all environment variables SHALL use the `PROCRASTINATOR_` prefix, and the PostgreSQL database SHALL be `procrastinator`. No production code, configuration file, or migration SHALL contain the identifiers `life-manager`, `lifemanager`, or the `LM_` env-var prefix.

#### Scenario: Startup aborts naming the missing PROCRASTINATOR_ variable

- **WHEN** the application starts with `PROCRASTINATOR_DATABASE_URL` unset
- **THEN** startup aborts with an error naming `PROCRASTINATOR_DATABASE_URL`

#### Scenario: No legacy identifiers remain

- **WHEN** the backend source tree (excluding archived openspec artifacts and historical docs) is searched for `life-manager`, `lifemanager`, or `LM_` env vars
- **THEN** no matches exist

#### Scenario: No superseded package roots remain

- **WHEN** the backend source tree is searched for references to the package roots `commons-data`, `commons-server`, `procrastinator-core`, `procrastinator-infra`, or `procrastinator-api`
- **THEN** no matches exist

### Requirement: Clean architecture layering

The backend SHALL be organized as a single Go module (`procrastinator-backend`) with flat top-level package roots mapped to clean-architecture layers, each with subpackages for grouping: `commons/` (shared kernel — `entity`, `repo`, `service`, `tenant`, `parse`), `core/` (domain and application services — `identity`, `ingest`), `infra/` (PostgreSQL repositories, LLM client, file storage — `postgres`, `llm`, `filestorage`), and `api/` (HTTP handlers, DTOs, `httpx` utilities, and the binary under `cmd/procrastinator`). Domain/application code SHALL NOT import `net/http`, `pgx`, or LLM client packages; no infrastructure package SHALL import another infrastructure package; `commons/` SHALL contain only entities, interfaces, and pure helpers (no concrete implementations); concrete implementations SHALL be wired together only in the binary's `main` package.

#### Scenario: Domain package has no infrastructure imports

- **WHEN** the import graph of `core/` is inspected
- **THEN** it depends only on the standard library and `commons/` packages

#### Scenario: Shared kernel has no implementations

- **WHEN** the import graph of `commons/` is inspected
- **THEN** it depends only on the standard library (and `commons/entity` from `commons/repo`) and contains no PostgreSQL, HTTP, or LLM implementation code

#### Scenario: Infrastructure implementations are wired in main

- **WHEN** the application binary is constructed
- **THEN** `api/` handlers and `core/` services receive their dependencies as interfaces defined in `commons/`, with concrete `infra/` implementations injected from `main` (`api/cmd/procrastinator`)

### Requirement: Boundary interfaces with compile-time guards

All repository and external-service boundaries SHALL be defined as Go interfaces in `commons/`: a single generic `repo.Repository[T]` (`Get`, `List`, `Create`, `Update`, `Delete` with functional options `Tenant`, `Where`, `Limit`, `Offset`, `OrderBy`) in `commons/repo`, the `service.Extractor` and `service.FileStorage` interfaces in `commons/service`, and a transactional `repo.TxFactory` (`InTransaction` passing an explicit transaction-bound `*repo.Repos`). Entity-specific persistence operations (such as race-safe serial insertion and the document+source join) SHALL NOT appear on the generic interface; they SHALL be concrete implementation methods consumed through narrow consumer-side interfaces. Every implementation in `infra/` SHALL carry a compile-time interface guard of the form `var _ <Interface> = (*<Impl>)(nil)`.

#### Scenario: Broken conformance fails the build

- **WHEN** an infrastructure implementation's method set no longer satisfies its boundary interface
- **THEN** the package fails to compile at the guard declaration

#### Scenario: Services consume interfaces, not implementations

- **WHEN** the ingest service and identity resolver are constructed in tests
- **THEN** they accept in-memory fakes of the `commons/repo` and `commons/service` interfaces without any PostgreSQL or LLM dependency

#### Scenario: Entity-specific queries use options, not interface methods

- **WHEN** identity resolution looks up an asset by normalized serial or by normalized brand+model
- **THEN** it issues `List` with `Where` options on `repo.Repository[entity.Asset]` rather than calling a dedicated finder method on the shared interface

#### Scenario: Transaction body uses the explicit transaction-bound repositories

- **WHEN** `InTransaction` executes its callback
- **THEN** the callback receives a transaction-bound `*repo.Repos` and all writes inside the callback go through it (no pool-bound repository is used inside the transaction)
