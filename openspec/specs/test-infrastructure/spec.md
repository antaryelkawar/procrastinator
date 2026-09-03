# test-infrastructure

## Purpose

Define the testing standards and infrastructure requirements for the project, emphasizing a test-driven development (TDD) workflow, isolation between unit and integration tests, and the use of fakes/containers to ensure reliable, reproducible tests.
## Requirements
### Requirement: Test-driven development workflow

Implementation SHALL proceed test-first: each behavioral requirement in this change's specs SHALL have at least one failing automated test written before the production code that satisfies it, and the full test suite SHALL pass before the change is considered complete.

#### Scenario: Every spec scenario maps to an executable test

- **WHEN** the implementation phase completes
- **THEN** every WHEN/THEN scenario in this change's delta specs is covered by at least one automated test that fails if the behavior is removed

### Requirement: Pure unit tests

Unit tests for extraction parsing, identity normalization, matching, and field-merge logic SHALL run without Docker, a database, or any network dependency, so they remain fast and runnable anywhere.

#### Scenario: Unit tests run without external services

- **WHEN** the unit test suite runs on a machine with no Docker daemon and no network
- **THEN** all unit tests pass

### Requirement: Docker PostgreSQL integration tests

Persistence- and API-level integration tests SHALL run against a real PostgreSQL 18 instance provided as a local Docker container. Tests SHALL obtain the database location from an environment variable (e.g., `LM_TEST_DATABASE_URL`), apply migrations before running, and isolate each test in a clean schema state.

#### Scenario: Integration tests exercise the real schema

- **WHEN** integration tests run with the test database environment variable set and the Docker PostgreSQL container running
- **THEN** migrations are applied and the tests execute real queries against the Source, Asset, and Document tables

#### Scenario: Missing test database is reported, not silently skipped into false confidence

- **WHEN** integration tests run without the test database available
- **THEN** the affected tests either skip with an explicit message or fail — they never pass without touching PostgreSQL

### Requirement: Fake LLM server in tests

Automated tests SHALL NOT require a real LLM. Tests SHALL stand up a fake HTTP server that emulates the OpenAI-compatible chat-completions endpoint and returns canned extraction JSON, and SHALL point the LLM client at it via the standard `LM_LLM_BASE_URL` configuration — with no test-only code paths in the client.

#### Scenario: Classification and extraction are tested against canned responses

- **WHEN** a test configures the LLM client with the fake server's base URL and uploads a fixture document
- **THEN** the fake server receives a well-formed chat-completions request and its canned extraction JSON drives the resulting classification, fields, and asset outcome

#### Scenario: Failure modes are tested against the fake server

- **WHEN** the fake server is programmed to return malformed JSON, a non-2xx status, or to hang past the timeout
- **THEN** the corresponding tests observe the specified extraction-failure behavior

### Requirement: Opt-in real LLM integration test

Exactly one integration test SHALL exercise a real OpenAI-compatible LLM endpoint. It SHALL be skipped unless an explicit opt-in environment variable (e.g., `LM_TEST_LLM_INTEGRATION=1`) is set together with real endpoint credentials, and SHALL be excluded from the default test run.

#### Scenario: Real LLM test is skipped by default

- **WHEN** the test suite runs without the opt-in environment variable
- **THEN** the real LLM integration test reports as skipped and no network call to an LLM provider is attempted

#### Scenario: Real LLM test runs when explicitly enabled

- **WHEN** the test suite runs with the opt-in variable set and valid real endpoint credentials configured
- **THEN** the test uploads a fixture document to the real endpoint and asserts a successful classification and extraction

### Requirement: Test user registration

Supersedes the prior test-identity convention: the test identities SHALL be registered users, and the registry SHALL be the `users` table whose id space is user ids.

Tests that exercise owner-scoped behavior SHALL register their explicit test users (e.g., `test-user`, `test-user-b`) in the user registry (`users` table) as part of test setup, before seeding owner-owned rows. Test helpers SHALL make this registration a single, reusable step.

#### Scenario: Integration test setup registers its users

- **WHEN** an integration test seeds data under `test-user` and `test-user-b`
- **THEN** both identifiers exist in the user registry before the first owner-owned row is inserted

