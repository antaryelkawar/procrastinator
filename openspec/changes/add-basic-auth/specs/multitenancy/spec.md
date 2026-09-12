# multitenancy delta — basic auth ordering

## ADDED Requirements

### Requirement: Authentication precedes tenancy resolution

User identification via the `{userId}` path parameter SHALL only run for requests that first passed backend transport authentication (HTTP Basic Auth, see the `api-basic-auth` capability). A request failing transport authentication SHALL be rejected (`401`) before the user-identification middleware and SHALL NOT trigger user-registry lookup, visibility evaluation, or any owned-data query. The `Authorization` credentials are transport-level authentication only and SHALL NOT be treated as a user identity source: the `{userId}` path parameter remains the sole means of identifying the active user for domain purposes.

#### Scenario: Failed transport auth never reaches user middleware

- **WHEN** a request with invalid Basic Auth credentials targets `/api/users/<registered-user-id>/assets`
- **THEN** the response is `401` and the `users` registry is not consulted

#### Scenario: Tenant resolution behavior unchanged for authenticated requests

- **WHEN** a request with valid Basic Auth credentials sends a well-formed but unregistered `{userId}`
- **THEN** the response is `404` exactly as specified by the multitenancy user-identification requirement
