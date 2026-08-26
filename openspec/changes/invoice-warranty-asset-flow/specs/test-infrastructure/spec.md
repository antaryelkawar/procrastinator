# Spec: test-infrastructure

Delta for change `invoice-warranty-asset-flow` (v3). Modifies the baseline in `openspec/specs/test-infrastructure`. Only the interface-based unit testing requirement changes (faked interfaces now live in `commons/repo` and `commons/service`); Docker PostgreSQL and gated real-LLM requirements are unchanged.

## MODIFIED Requirements

### Requirement: Interface-based unit testing

Application services (`ingest`, `identity`) SHALL be unit-testable purely with in-memory fakes of the `commons/` boundary interfaces (`repo.Repository[T]`, `repo.TxFactory`, `service.Extractor`, `service.FileStorage`) — no Docker, database, or network required — because all dependencies are injected as interfaces.

#### Scenario: Ingest service tests run without PostgreSQL

- **WHEN** the ingest service unit tests run on a machine with no Docker daemon
- **THEN** they pass using in-memory fake repositories, a fake extractor, and a fake file-storage
