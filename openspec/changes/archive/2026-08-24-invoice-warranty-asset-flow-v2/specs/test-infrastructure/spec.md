# Spec: test-infrastructure

Delta for change `invoice-warranty-asset-flow`. Modifies the archived baseline in `openspec/specs/test-infrastructure`.

## MODIFIED Requirements

### Requirement: Docker PostgreSQL integration tests

Persistence- and API-level integration tests SHALL run against a real PostgreSQL 18 instance provided as a local Docker container. Tests SHALL obtain the database location from the environment variable `PROCRASTINATOR_TEST_DATABASE_URL`, apply migrations before running, and isolate each test in a clean schema state (per-package private schemas to avoid cross-package truncate races). All integration tests SHALL operate under explicit test tenant identifiers.

#### Scenario: Integration tests exercise the real tenant-scoped schema

- **WHEN** integration tests run with the test database environment variable set and the Docker PostgreSQL container running
- **THEN** migrations are applied and the tests execute real tenant-scoped queries against the Source, Asset, and Document tables

#### Scenario: Missing test database is reported, not silently skipped into false confidence

- **WHEN** integration tests run without the test database available
- **THEN** the affected tests skip with an explicit message — they never pass without touching PostgreSQL

### Requirement: Opt-in real LLM integration test

Exactly one integration test SHALL exercise the real OpenAI-compatible LLM endpoint. It SHALL be skipped unless the explicit opt-in environment variable `PROCRASTINATOR_TEST_LLM_INTEGRATION=1` is set together with real endpoint credentials, and SHALL be excluded from the default test run. The test SHALL upload the real sample documents from `sample-data/` (the LG AMC invoice PDF and the IFB AMC contract PDF) and assert correct classification (`invoice`/`amc`), at least one usable identity field, and a non-empty metadata object.

#### Scenario: Real LLM test is skipped by default

- **WHEN** the test suite runs without the opt-in environment variable
- **THEN** the real LLM integration test reports as skipped and no network call to an LLM provider is attempted

#### Scenario: Real LLM test runs against sample documents when enabled

- **WHEN** the test suite runs with the opt-in variable set and valid real endpoint credentials configured
- **THEN** the test sends each `sample-data/` PDF to the real endpoint and asserts classification, usable identity fields, and populated metadata

## ADDED Requirements

### Requirement: Interface-based unit testing

Application services (`ingest`, `identity`) SHALL be unit-testable purely with in-memory fakes of the `commons-data` boundary interfaces — no Docker, database, or network required — because all dependencies are injected as interfaces.

#### Scenario: Ingest service tests run without PostgreSQL

- **WHEN** the ingest service unit tests run on a machine with no Docker daemon
- **THEN** they pass using in-memory fake repositories, a fake extractor, and a fake file-storage
