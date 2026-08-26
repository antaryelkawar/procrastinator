# Spec: test-infrastructure

Delta for change `multi-tenant-isolation`. Modifies the baseline in `openspec/specs/test-infrastructure`.

Adds one requirement covering the tenant registry in test setups. No existing requirement is changed; this does not overlap with the open `invoice-warranty-asset-flow` v3 deltas on this capability.

## ADDED Requirements

### Requirement: Test tenant registration

Tests that exercise tenant-scoped behavior SHALL register their explicit test tenants (e.g., `test-tenant`, `test-tenant-b`) in the tenant registry as part of test setup, before seeding tenant-owned rows. Test helpers SHALL make this registration a single, reusable step so no test bypasses the registry. Tests SHALL continue to use only explicit, distinct tenant identifiers — never a default or ambient tenant.

#### Scenario: Integration test setup registers its tenants

- **WHEN** an integration test seeds data under `test-tenant` and `test-tenant-b`
- **THEN** both identifiers exist in the `tenants` registry before the first tenant-owned row is inserted

#### Scenario: Cross-tenant isolation tests remain two-tenant

- **WHEN** the integration suite runs cross-tenant isolation cases
- **THEN** each case registers, seeds, and reads data under two explicit, distinct tenant ids

#### Scenario: Registry enforcement is itself tested

- **WHEN** the test suite runs
- **THEN** at least one test asserts that inserting a tenant-owned row for an unregistered tenant fails with a foreign-key violation
