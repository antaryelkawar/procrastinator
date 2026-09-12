# api-contract delta — basic auth security scheme

## ADDED Requirements

### Requirement: Contract declares the Basic Auth security scheme

The OpenAPI document SHALL declare a `basicAuth` security scheme of type `http` with scheme `basic`, and SHALL apply it globally to the API (or to every documented operation). The scheme declaration SHALL be visible in generated client and documentation artifacts so that generated code and docs reflect that the API requires Basic Auth. The document's existing error-envelope invariant for non-2xx responses is unchanged and extends to the newly documented `401` responses (see the api-basic-auth delta): they SHALL also use the `{"error": string}` envelope. Non-API transport-level responses (nothing else) violate this only insofar as they are documented — there SHALL be no separately documented empty-body 401.

#### Scenario: Security scheme present in the document

- **WHEN** the committed `openapi.yaml` is inspected
- **THEN** it contains a `basicAuth` `http` security scheme with scheme `basic` applied to the documented operations
