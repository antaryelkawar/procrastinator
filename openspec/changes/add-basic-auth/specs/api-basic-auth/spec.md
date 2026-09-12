# Spec: api-basic-auth (delta)

All registered API routes live under `/api/users/{userId}` (single namespace). "The API" below means every route registered by the backend HTTP router.

## ADDED Requirements

### Requirement: Generated client sends Basic Auth credentials

The UI client SHALL attach the `Authorization: Basic <credentials>` header to every API request it makes to the backend. Credentials SHALL come from two build-time environment variables in the frontend build: `VITE_API_BASIC_AUTH_USER` and `VITE_API_BASIC_AUTH_PASSWORD` (only `VITE_`-prefixed variables are inlined by Vite). These are an application-level shared secret baked into the shipped bundle; they shall not be individual user secrets, and the acceptable exposure model (any person who can load the UI bundle can read them) is an accepted consequence of this change, stated here rather than silently assumed. The header injection SHALL happen at a single, centralized point in the UI client layer (the fetch mutator and the multipart upload helper), not per call site, and SHALL NOT rely on the browser's native Basic Auth dialog (no bare `window.location` navigation or 401-prompt flow is involved).

#### Scenario: Read request carries authorization header

- **WHEN** the UI issues a GET request to a documented API endpoint
- **THEN** the outgoing request includes an `Authorization` header with a valid `Basic <credentials>` scheme token built from `VITE_API_BASIC_AUTH_USER` / `VITE_API_BASIC_AUTH_PASSWORD`

#### Scenario: Multipart upload carries authorization header

- **WHEN** the UI uploads a document via the multipart upload helper
- **THEN** the upload request includes the same `Authorization: Basic` header used by fetch-based calls

#### Scenario: Frontend credentials are absent

- **WHEN** the UI is built without `VITE_API_BASIC_AUTH_USER` or `VITE_API_BASIC_AUTH_PASSWORD` set
- **THEN** the UI fails fast at startup with a clear error naming the missing variable, and makes no API requests with empty credentials

#### Scenario: 401 surfaces through existing error handling

- **WHEN** the backend rejects a UI request with `401 Unauthorized` (bad or missing credentials)
- **THEN** the client surfaces an `ApiError` with that status via the existing error handling, without crashing or hanging

### Requirement: Basic Auth is configurable via environment

Valid Basic Auth credentials SHALL be configured exclusively through the environment variable `PROCRASTINATOR_BASIC_AUTH_USERS`, whose value is a JSON array of objects `{"user": <string>, "pass": <string>}`, e.g. `[{"user":"app","pass":"s3cret!"}]`. This encoding allows passwords containing `:` or `,` without escaping. The array SHALL contain at least one and at most 16 credential pairs; the username SHALL be 1–64 UTF-8 characters, the password SHALL be 8–128 UTF-8 characters. The variable SHALL be required: the server SHALL fail fast at startup (with an error naming the variable and the violated constraint) if it is absent, malformed JSON, empty, exceeds 16 pairs, or violates the length bounds. The exact accepted/required syntax SHALL be documented in `.env.example`. Credentials as provisioned via environment variables are plaintext at rest in the deployment environment (IOC-style secret managers are not in scope).

#### Scenario: Server fails fast without configured credentials

- **WHEN** the backend starts with `PROCRASTINATOR_BASIC_AUTH_USERS` unset
- **THEN** startup fails with an error naming `PROCRASTINATOR_BASIC_AUTH_USERS` and the server process does not begin listening

#### Scenario: Server fails fast on malformed credential configuration

- **WHEN** the backend starts with `PROCRASTINATOR_BASIC_AUTH_USERS` set to a value that is not a valid JSON array of `{"user","pass"}` objects, or violates the 1–16 pair / 1–64 / 8–128 character bounds
- **THEN** startup fails with an error describing the violated constraint and the server does not begin listening

#### Scenario: Configured credentials are accepted

- **WHEN** valid credentials are set via `PROCRASTINATOR_BASIC_AUTH_USERS` and a request carries `Authorization: Basic` with those exact credentials
- **THEN** the credentials are accepted and the request proceeds normally

### Requirement: API requests require valid Basic Auth

Every API request SHALL be required to carry an `Authorization: Basic <base64(user:pass)>` header whose decoded user/password exactly match one pair in `PROCRASTINATOR_BASIC_AUTH_USERS`. The comparison SHALL run in constant time (e.g. `subtle.ConstantTimeCompare` over the byte slices) so credential validation does not leak timing information. Requests that do not match SHALL be rejected with `401 Unauthorized`, a `WWW-Authenticate: Basic` response header, and a body of the standard JSON error envelope `{"error": string}` (consistent with every other documented non-2xx response of the API). No database access and no handler execution SHALL occur for rejected requests. Valid requests proceed through the rest of the middleware chain and handlers unchanged in every other respect.

#### Scenario: Valid credentials are accepted

- **WHEN** a request carries `Authorization: Basic <credentials>` matching one configured pair
- **THEN** the request proceeds through the middleware chain to the handler and the response is the normal, authenticated response for that route

#### Scenario: Missing Authorization header is rejected

- **WHEN** a request is sent without an `Authorization` header
- **THEN** the response is `401 Unauthorized` with a `WWW-Authenticate: Basic` response header and the `{"error": string}` envelope body

#### Scenario: Wrong credentials are rejected — even on registered user paths

- **WHEN** a request to `/api/users/<registered-user-id>/assets` carries Basic Auth credentials that do not match the configured set
- **THEN** the response is `401 Unauthorized` with the envelope body before user identification runs, even though the `{userId}` is valid

#### Scenario: Other authorization schemes are rejected

- **WHEN** a request carries an `Authorization` header using a scheme other than `Basic` (e.g. `Bearer`, `Digest`)
- **THEN** the response is `401 Unauthorized` with the same rejection behavior (header, envelope body) as missing/invalid credentials

#### Scenario: Credential check does not leak timing

- **WHEN** the same request is issued repeatedly, once with credentials matching a configured pair exactly and once differing only in a final password character
- **THEN** no observable difference in response behavior (status, headers, body) exists between the two, and the rejection path does not reveal which character or field mismatched

### Requirement: Authentication precedes tenancy resolution

Basic Auth validation MUST run before user identification/tenancy resolution in the middleware chain. A request failing Basic Auth SHALL be rejected with the 401 behavior described in "API requests require valid Basic Auth" and SHALL NOT perform tenant/user lookup or reach any owned-data query. Passing Basic Auth alone SHALL NOT confer any tenant access: user scoping and ownership isolation continue to be governed entirely by the multitenancy capability (the `Authorization` credentials are transport authentication and convey no user identity within the domain model).

#### Scenario: Failed Basic Auth does not leak tenant data

- **WHEN** a request with invalid Basic Auth credentials targets a route belonging to a registered user
- **THEN** the response is `401`, the user registry is not consulted, and no data is read or written

#### Scenario: Identity semantics are unchanged by Basic Auth

- **WHEN** a request with valid Basic Auth credentials accesses `/api/users/{userId}/...`
- **THEN** user identity, visibility, and isolation are determined exactly as before by the multitenancy rules; the Basic Auth credentials grant no additional or different user identity
