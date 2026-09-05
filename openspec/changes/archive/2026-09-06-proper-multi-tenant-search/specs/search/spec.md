# Spec: search

New capability for change `proper-multi-tenant-search`. Adds tenant-scoped, read-only general search across a user's owned resources (assets, finance accounts, money movements, documents, statement import batches), exposed as a fast quick-dropdown endpoint (capped, no pagination) and a paged results-page endpoint. Search returns a shared hit shape with a single deterministic combined ordering, and conforms to the existing owner-model visibility rule, cross-owner isolation, RLS backstop, and OpenAPI source-of-truth conventions. This is an MVP: matching is a literal case-insensitive substring (no full-text search, no relevance ranking) and is expected to evolve.

## ADDED Requirements

### Requirement: Search is a proper multi-tenant, read-only capability

Both search endpoints SHALL live under the single `/api/users/{userId}` path-tenancy group and SHALL be resolved by the existing user middleware; there SHALL be no header-based tenancy for search. The `{userId}` path parameter is the sole identity of the requesting user; a malformed `{userId}` SHALL be rejected with `400 Bad Request` and a well-formed but unregistered `{userId}` SHALL be rejected with `404 Not Found`, in both cases before any search work runs.

Search results SHALL include only rows visible to the requesting user under the existing single owner-model visibility rule (a row is visible iff `owner_id = me` OR the requester is a member of the row's `owner_household_id`). Search SHALL run in the RLS-bound session so the row-level security backstop applies: a session with no bound `app.user_id` SHALL see no rows. Search SHALL be strictly read-only: it SHALL NOT create, update, or delete any row or file.

#### Scenario: Search routes are path-tenant and header-tenancy-free

- **WHEN** a client calls `GET /api/users/alice/search/quick?q=notebook`
- **THEN** the request is handled under the `/api/users/{userId}` path-tenancy group, resolved by the existing user middleware from the path, with no tenancy header consulted

#### Scenario: Malformed userId is rejected with 400

- **WHEN** a client calls either search endpoint with an empty, over-length, or non-`[A-Za-z0-9_-]` `{userId}`
- **THEN** the response is `400 Bad Request` and no search query is issued

#### Scenario: Unregistered userId is rejected with 404

- **WHEN** a client calls either search endpoint with a well-formed `{userId}` that is not a row in the `users` registry
- **THEN** the response is `404 Not Found` and no search query is issued

#### Scenario: Only rows visible to the requester are returned

- **WHEN** user `alice` runs a search whose term matches both an asset she owns (personal) and a household asset in a household she is a member of
- **THEN** both rows are eligible to appear in the results and no row outside her visibility is returned

#### Scenario: Another owner's personal row is never returned

- **WHEN** user `alice` runs a search whose term matches a personal asset owned by user `bob` (a different owner, and `alice` is not a member of any household owning it)
- **THEN** bob's asset does not appear in `alice`'s results and no field of it is exposed

#### Scenario: A member can find a household row

- **WHEN** user `bob` is a member of household `h1` and runs a search whose term matches an asset whose `owner_id = 'alice'` and `owner_household_id = 'h1'`
- **THEN** that asset is returned to `bob`

#### Scenario: A non-member cannot find a household row

- **WHEN** user `carol` is not a member of household `h1` and runs a search whose term matches an asset whose `owner_id = 'alice'` and `owner_household_id = 'h1'`
- **THEN** that asset does not appear in `carol`'s results

#### Scenario: An unbound session sees nothing (RLS backstop)

- **WHEN** a search query is executed in a database session with no bound `app.user_id`
- **THEN** zero rows are returned from any searched table regardless of the data present

#### Scenario: Search is read-only

- **WHEN** either search endpoint is called with a non-empty query
- **THEN** no row in any table is created, updated, or deleted and no file is written

### Requirement: General search matches across resource types

A search query SHALL match a resource if the query is a case-insensitive substring of any of that resource type's searchable fields. The searched resource types and their searchable fields SHALL be: `asset` (brand, model, serial number), `account` (name, type, institution), `movement` (description, external reference), `document` (source filename), and `import_batch` (filename). Matching SHALL treat the query as a literal substring: LIKE metacharacters in the query (`%`, `_`) SHALL be escaped so they match literally rather than acting as wildcards. A query that matches no visible row SHALL yield an empty result set (not an error).

#### Scenario: Case-insensitive substring match

- **WHEN** user `alice` owns an asset with `model = "MacBook Pro 14"` and searches `q=macbook`
- **THEN** that asset is returned for `alice`

#### Scenario: Match on a non-primary field

- **WHEN** user `alice` owns a movement whose `description = "coffee beans"` and `external_reference` is empty, and searches `q=coffee`
- **THEN** that movement is returned (matched on description)

#### Scenario: Match on a secondary field

- **WHEN** user `alice` owns a movement whose `description = "misc"` and `external_reference = "TXN-90210"`, and searches `q=90210`
- **THEN** that movement is returned (matched on external reference)

#### Scenario: Document matches on its source filename

- **WHEN** user `alice` has a document whose source filename is `rent-july.pdf` and searches `q=rent-july`
- **THEN** that document is returned as a `document`-typed hit

#### Scenario: LIKE metacharacters match literally

- **WHEN** user `alice` owns an account named `100% savings` and searches `q=100%`
- **THEN** the query is matched literally (the `%` is escaped), so it matches only the literal `100%` text and does not act as a wildcard that matches every account

#### Scenario: A query with no matches returns an empty set

- **WHEN** user `alice` searches `q=zzz-no-such-thing` and no visible row's searchable field contains it
- **THEN** the response is a `200` with an empty result set (not an error status)

### Requirement: Deterministic combined ordering

Both endpoints SHALL return the same combined result set in a single deterministic order so results are stable across repeated identical requests. The order SHALL be: first by a fixed resource-type priority (`asset`, then `account`, then `movement`, then `document`, then `import_batch`), then by `created_at` descending, then by resource id ascending. The quick endpoint's results SHALL be exactly the top-N of this combined order (for N = its effective limit), so the quick results are a prefix of the results-page ordering for the same query.

#### Scenario: Ordering is stable across identical requests

- **WHEN** user `alice` sends the same query to the results-page endpoint twice with the same page and page size
- **THEN** both responses return the same hits in the same order

#### Scenario: Type priority is applied before recency

- **WHEN** a query matches an `account` created more recently than a matching `asset`
- **THEN** the `asset` still appears before the `account` in the combined result set (type priority dominates)

#### Scenario: Quick results are a prefix of the results-page ordering

- **WHEN** user `alice` searches a term with `limit=5` on the quick endpoint and `page_size>=5, page=1` on the results-page endpoint for the same query
- **THEN** the quick endpoint's five hits equal the first five hits of the results page, in the same order

### Requirement: Quick dropdown endpoint

The system SHALL expose `GET /api/users/{userId}/search/quick` that returns a fast, capped set of hits for typeahead. It SHALL accept an optional `q` query parameter and an optional `limit` query parameter. `limit` SHALL default to 10 and SHALL be an integer in `[1, 50]`; a non-integer or out-of-range `limit` SHALL be rejected with `400 Bad Request`. The endpoint SHALL return at most the effective `limit` hits from the top of the combined ordering and SHALL return no pagination metadata (no `page`, `page_size`, or `total`). An absent or blank `q` (after trimming) SHALL return `200` with an empty result set rather than an error.

#### Scenario: Quick returns the top hits

- **WHEN** user `alice` searches `GET /api/users/alice/search/quick?q=note` and there are matching visible rows
- **THEN** the response is `200` with `results` containing the top matching hits in combined order

#### Scenario: Quick respects the limit

- **WHEN** there are 30 matching visible rows and `limit=10` (the default)
- **THEN** the response's `results` contains at most 10 hits and no `total`/`page`/`page_size` fields

#### Scenario: Quick honors an explicit limit

- **WHEN** there are 30 matching visible rows and `limit=5`
- **THEN** the response's `results` contains at most 5 hits (the top 5 of the combined order)

#### Scenario: Blank query returns empty results

- **WHEN** user `alice` calls the quick endpoint with `q` absent or `q=` (blank)
- **THEN** the response is `200` with `results` equal to an empty list

#### Scenario: limit above the maximum is rejected

- **WHEN** the quick endpoint is called with `limit=51`
- **THEN** the response is `400 Bad Request` and no hits are returned

#### Scenario: limit below the minimum is rejected

- **WHEN** the quick endpoint is called with `limit=0` or a non-integer `limit`
- **THEN** the response is `400 Bad Request` and no hits are returned

### Requirement: Paged results-page endpoint

The system SHALL expose `GET /api/users/{userId}/search` that returns a page of the combined, ordered result set together with pagination metadata. It SHALL accept an optional `q`, an optional `page`, and an optional `page_size`. `page` SHALL be a 1-based integer (default 1); `page_size` SHALL be an integer in `[1, 100]` (default 20). A non-integer or out-of-range `page`/`page_size` SHALL be rejected with `400 Bad Request`. The `200` response SHALL include `results` (the hits for that page, never `null`), `page`, `page_size`, and `total` (the total number of matching visible hits across all resource types). An absent or blank `q` SHALL return `200` with an empty `results` list and `total = 0`. A page number beyond the last populated page SHALL return `200` with an empty `results` list and the unchanged `total`.

#### Scenario: Returns the first page with total

- **WHEN** there are 25 matching visible rows and the results-page endpoint is called with `q=<term>` and no page parameters
- **THEN** the response is `200` with `results` containing the first 20 hits, `page = 1`, `page_size = 20`, and `total = 25`

#### Scenario: Returns a subsequent page

- **WHEN** there are 25 matching visible rows and the results-page endpoint is called with `page=2&page_size=20`
- **THEN** the response is `200` with `results` containing the remaining 5 hits, `page = 2`, `page_size = 20`, and `total = 25`

#### Scenario: A page beyond the last returns empty results with valid metadata

- **WHEN** there are 25 matching visible rows and the results-page endpoint is called with `page=99&page_size=20`
- **THEN** the response is `200` with `results` equal to an empty list, `page = 99`, `page_size = 20`, and `total = 25`

#### Scenario: total reflects all resource types

- **WHEN** a query matches one visible asset, one visible account, and one visible movement
- **THEN** the results page reports `total = 3`

#### Scenario: Blank query returns an empty page

- **WHEN** the results-page endpoint is called with `q` absent or blank
- **THEN** the response is `200` with `results` equal to an empty list, `total = 0`, and the effective `page`/`page_size`

#### Scenario: Invalid page is rejected

- **WHEN** the results-page endpoint is called with `page=0`, a negative page, or a non-integer page
- **THEN** the response is `400 Bad Request`

#### Scenario: Invalid page_size is rejected

- **WHEN** the results-page endpoint is called with `page_size=0`, `page_size=101`, or a non-integer page_size
- **THEN** the response is `400 Bad Request`

### Requirement: Search hit shape

Every hit returned by either endpoint SHALL be a `search_hit` with: a `type` naming the resource type (exactly one of `asset`, `account`, `movement`, `document`, `import_batch`); an `id` that is that resource's own identifier and that names a row of that type visible to the requesting user (no dangling ids); a non-blank `title` suitable for display (derived from the resource, e.g. an asset's brand+model or serial, an account's name, a movement's description, a document/import batch's filename); an optional `subtitle` (may be absent) carrying a secondary display line; and an optional `confidence` (may be absent) in the closed interval `[0.0, 1.0]` carrying the resolution confidence of the underlying record where one exists. The `type` and `id` SHALL be sufficient for the client to navigate to the resource.

#### Scenario: Asset hit carries its resolution confidence

- **WHEN** a hit is returned for a matching asset whose resolution confidence is `0.82`
- **THEN** the hit's `confidence` is `0.82`

#### Scenario: Confidence is absent when the record has none

- **WHEN** a hit is returned for a record that has no resolution confidence (e.g. a record created before confidence existed, or a non-asset type that carries none)
- **THEN** the hit is still valid with `confidence` absent (or empty)

#### Scenario: Hit carries a valid type, id, and non-blank title

- **WHEN** a hit is returned for a matching asset
- **THEN** its `type` is `asset`, its `id` is a non-empty asset id, and its `title` is non-blank

#### Scenario: Hit type is one of the searched types

- **WHEN** any hit is returned by either endpoint
- **THEN** its `type` is exactly one of `asset`, `account`, `movement`, `document`, `import_batch`

#### Scenario: Hit id names a real visible resource

- **WHEN** a hit of a given `type` is returned to a user
- **THEN** a row of that type with that `id` exists and is visible to that user (the id does not dangle or point at another owner's invisible row)

#### Scenario: subtitle is optional

- **WHEN** a hit has no meaningful secondary line
- **THEN** the hit is still valid with `subtitle` absent (or empty)

### Requirement: Empty and error response behavior

Search SHALL use the standard `{"error": string}` JSON error envelope for every non-2xx response. A search that matches nothing SHALL be a `200` with an empty result set, never a `404`. A `q` value longer than 200 characters SHALL be rejected with `400 Bad Request`. Invalid pagination or limit parameters SHALL be rejected with `400 Bad Request`.

#### Scenario: No matches is a 200, not a 404

- **WHEN** a search query matches no visible row
- **THEN** the response status is `200` with an empty result set, not `404`

#### Scenario: Overly long query is rejected

- **WHEN** either endpoint is called with a `q` of 201 or more characters
- **THEN** the response is `400 Bad Request`

#### Scenario: Boundary-length query is accepted

- **WHEN** an endpoint is called with a `q` of exactly 200 characters
- **THEN** the `400` length rule is not triggered (the query is accepted and searched)

#### Scenario: Errors use the standard envelope

- **WHEN** a search endpoint returns a non-2xx response (for example `400`)
- **THEN** the response body is the `{"error": string}` envelope

### Requirement: OpenAPI source-of-truth coverage

Both search operations and the search response/hit schemas SHALL be added to the source-of-truth OpenAPI 3.1 document (`procrastinator-backend/api/openapi.yaml`) and SHALL be reflected in the generated server types, preserving the existing 1:1 routes↔operations lockstep. The document SHALL define `search_hit` (with the `type` enum, `id`, `title`, optional `subtitle`, and optional nullable `confidence`), `search_quick_response` (a capped `results` array), and `search_results_page` (`results` plus `page`, `page_size`, `total`), and every non-2xx response of the two operations SHALL reference the standard `error` envelope.

#### Scenario: Both search operations are documented

- **WHEN** the OpenAPI document is inspected for the search endpoints
- **THEN** both `GET /api/users/{userId}/search/quick` and `GET /api/users/{userId}/search` are present as operations

#### Scenario: The new search schemas are present

- **WHEN** the OpenAPI document's `components.schemas` is inspected
- **THEN** `search_hit`, `search_quick_response`, and `search_results_page` are defined and referenced by the two operations' responses

#### Scenario: Routes and operations stay in lockstep

- **WHEN** the backend router's registered routes are compared against the operations in the document
- **THEN** the two new search routes each correspond to exactly one documented operation and no undocumented search route exists

#### Scenario: Generated types reflect the search schemas

- **WHEN** the server code generator is run against the updated document
- **THEN** Go types for `search_hit`, `search_quick_response`, and `search_results_page` are generated and the backend compiles with the two new handlers implementing them
