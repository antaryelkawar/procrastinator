# Spec: backend-platform

## Purpose

Establish a robust, clean-architecture-compliant backend for the Procrastinator system. This includes defining the project structure, naming conventions, layering, and interface boundaries to ensure maintainability, testability, and adherence to best practices.
## Requirements
### Requirement: Procrastinator naming

The backend SHALL use `procrastinator` naming consistently: the binary SHALL be `procrastinator-api`, the configuration struct SHALL be `Config`, all environment variables SHALL use the `PROCRASTINATOR_` prefix, and the PostgreSQL database SHALL be `procrastinator`. No production code, configuration file, or migration SHALL contain the identifiers `life-manager`, `lifemanager`, or the `LM_` env-var prefix.

#### Scenario: Startup aborts naming the missing PROCRASTINATOR_ variable

- **WHEN** the application starts with `PROCRASTINATOR_DATABASE_URL` unset
- **THEN** startup aborts with an error naming `PROCRASTINATOR_DATABASE_URL`

#### Scenario: No legacy identifiers remain

- **WHEN** the backend source tree (excluding archived openspec artifacts and historical docs) is searched for `life-manager`, `lifemanager`, or `LM_` env vars
- **THEN** no matches exist

### Requirement: Clean architecture layering

The backend SHALL be organized as a single Go module (`procrastinator-backend`) with package roots mapped to clean-architecture layers: `commons-data` (shared entities, boundary interfaces, pure helpers), `commons-server` (configuration, HTTP/middleware utilities), `procrastinator-core` (domain and application services), `procrastinator-infra` (PostgreSQL repositories, LLM client, file storage), and `procrastinator-api` (HTTP handlers and the binary). Domain/application code SHALL NOT import `net/http`, `pgx`, or LLM client packages; no infrastructure package SHALL import another infrastructure package; concrete implementations SHALL be wired together only in the binary's `main` package.

#### Scenario: Domain package has no infrastructure imports

- **WHEN** the import graph of `procrastinator-core` is inspected
- **THEN** it depends only on the standard library and `commons-data`

#### Scenario: Infrastructure implementations are wired in main

- **WHEN** the application binary is constructed
- **THEN** `procrastinator-api` handlers and `procrastinator-core` services receive their dependencies as interfaces defined in `commons-data`, with concrete `procrastinator-infra` implementations injected from `main`

### Requirement: Boundary interfaces with compile-time guards

All repository and external-service boundaries SHALL be defined as Go interfaces in `commons-data` (at minimum: `AssetRepository`, `SourceRepository`, `DocumentRepository`, `Extractor`, `FileStorage`, and a transactional `RepoFactory`). Every implementation in `procrastinator-infra` SHALL carry a compile-time interface guard of the form `var _ <Interface> = (*<Impl>)(nil)`.

#### Scenario: Broken conformance fails the build

- **WHEN** an infrastructure implementation's method set no longer satisfies its boundary interface
- **THEN** the package fails to compile at the guard declaration

#### Scenario: Services consume interfaces, not implementations

- **WHEN** the ingest service and identity resolver are constructed in tests
- **THEN** they accept in-memory fakes of the `commons-data` interfaces without any PostgreSQL or LLM dependency

### Requirement: Safe dynamic query construction

Wherever the backend builds SQL from caller-influenced names — repository filter fields, filter operators, and ordering columns — the names SHALL be validated against an explicit per-entity whitelist before use. Only whitelisted fields, operators, and columns SHALL appear in generated SQL; all values SHALL be passed as bind parameters, never string-interpolated. An unknown field, operator, or ordering column SHALL be rejected with an error before any query is issued.

#### Scenario: Unknown filter field is rejected

- **WHEN** a repository query is built with a filter field outside the entity's whitelist
- **THEN** an error is returned and no SQL statement is issued

#### Scenario: Unknown filter operator is rejected

- **WHEN** a repository query is built with an operator outside the supported set (`=`, `!=`, `<`, `<=`, `>`, `>=`, `LIKE`, `IN`)
- **THEN** an error is returned and no SQL statement is issued

#### Scenario: Injection attempt in a field name cannot reach SQL

- **WHEN** a repository query is built with a filter field or ordering column containing SQL metacharacters (e.g. `1; DROP TABLE assets--`)
- **THEN** the name fails whitelist validation, an error is returned, and no SQL statement containing the input is issued

#### Scenario: Whitelisted filter uses bind parameters

- **WHEN** a repository query is built with a whitelisted field, operator, and a caller-supplied value
- **THEN** the generated SQL references the whitelisted column name only and carries the value as a positional bind parameter

