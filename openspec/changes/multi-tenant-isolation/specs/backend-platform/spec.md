# Spec: backend-platform

Delta for change `multi-tenant-isolation`. Modifies the baseline in `openspec/specs/backend-platform`.

Adds one requirement on safe dynamic query construction. No existing requirement is changed; this does not overlap with the open `invoice-warranty-asset-flow` v3 deltas on this capability.

## ADDED Requirements

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
