# Spec: test-infrastructure

Delta for change `multi-tenant-isolation`. Modifies the baseline in `openspec/specs/test-infrastructure`.

## ADDED Requirements

### Requirement: Test user registration

Supersedes "Test tenant convention": test tenants SHALL be test users, and the registry SHALL be the `tenants` table whose id space is user ids.

Tests that exercise tenant-scoped behavior SHALL register their explicit test users (e.g., `test-user`, `test-user-b`) in the user registry (`tenants` table) as part of test setup, before seeding tenant-owned rows. Test helpers SHALL make this registration a single, reusable step.

#### Scenario: Integration test setup registers its users

- **WHEN** an integration test seeds data under `test-user` and `test-user-b`
- **THEN** both identifiers exist in the user registry before the first tenant-owned row is inserted
