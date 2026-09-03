# multitenancy

## Purpose

This specification describes the owner-model multi-tenancy capability: how a user is identified per request, how data rows are owned (per user and optionally per household), the single visibility rule enforced identically at the application, RLS, and membership-trigger layers, and the household management routes.

## Requirements

### Requirement: User identification via the URL path parameter

All API routes SHALL live under a single `/api/users/{userId}` group. The `{userId}` URL path parameter SHALL be the sole means of identifying the active user on every request; no header-based identity mechanism SHALL exist. Middleware SHALL extract `{userId}` from the request path (e.g. via `chi.URLParam`), SHALL validate its format (non-empty, at most 64 characters drawn from `[A-Za-z0-9_-]`), and SHALL validate that it names a user present in the `users` registry (see "User registry").

A request whose `{userId}` is malformed SHALL be rejected with `400 Bad Request`. A request whose `{userId}` is well-formed but does not name a registered user SHALL be rejected with `404 Not Found`. A registry database error SHALL result in `500 Internal Server Error`. In all rejection cases the rejection SHALL occur before any handler or repository code runs and no data SHALL be read or written. For accepted requests, middleware SHALL place the user ID into the request `context.Context` for downstream use.

#### Scenario: Malformed userId is rejected with 400

- **WHEN** a client sends a request to any `/api/users/{userId}/...` route with a `{userId}` that is empty, longer than 64 characters, or contains a character outside `[A-Za-z0-9_-]`
- **THEN** the response is `400 Bad Request` and no handler or repository code runs and no data is read or written

#### Scenario: Well-formed but unregistered user is rejected with 404

- **WHEN** a client sends a request with a well-formed `{userId}` that does not name a row in the `users` registry
- **THEN** the response is `404 Not Found` and no data is read or written

#### Scenario: Valid registered user reaches the handler via context

- **WHEN** a client sends `GET /api/users/alice/assets` and `alice` is a registered user in the `users` registry
- **THEN** the request is accepted, no `400`/`404`/`500` rejection occurs, and downstream handlers, services, and repositories observe user `alice` from the request context

### Requirement: User registry

The system SHALL maintain a `users` table as the user registry, with columns `id text PRIMARY KEY` (constrained to `^[A-Za-z0-9_-]{1,64}$`) and `created_at`. There SHALL be one row per valid user id. The `owner_id` column of every owned data table (`sources`, `assets`, `documents`, `financial_accounts`, `money_movements`, `import_batches`, `import_lines`) SHALL reference `users.id` via a foreign key. Users SHALL be created only by explicit provisioning; the registry SHALL NOT be populated implicitly by request traffic.

#### Scenario: Owned row with a non-registry owner fails FK

- **WHEN** a row is inserted into `assets` with an `owner_id` that does not exist in the `users` registry
- **THEN** the insert fails with a foreign-key violation

### Requirement: Owner-scoped persistence

Every owned data table (`sources`, `assets`, `documents`, `financial_accounts`, `money_movements`, `import_batches`, `import_lines`) SHALL carry an `owner_id` column (`NOT NULL`, foreign key to `users.id`) and a nullable `owner_household_id` column (foreign key to `households.id`). There SHALL be no `scope_type` column or discriminator on any owned table. The `households` table SHALL carry an `owner_id` column (the owning user) and SHALL NOT carry an `owner_household_id`.

Repository calls SHALL pass the user via the `repo.Owner(id)` option; when the option is absent the repository SHALL fall back to the user carried in the passed `context.Context`. The explicit option SHALL take precedence over the context. A repository method invoked with neither an explicit user option nor a context user SHALL return the sentinel error `ErrNoUser` and SHALL NOT issue any SQL statement.

#### Scenario: Repository writes stamp the bound user

- **WHEN** a Document is created via a repository call whose resolved user is `alice`
- **THEN** the persisted row has `owner_id = 'alice'`

#### Scenario: Repository call without user fails closed

- **WHEN** any repository method is called with neither a `repo.Owner(id)` option nor a context user
- **THEN** it returns `ErrNoUser` and performs no query

### Requirement: Visibility rule

A single visibility rule SHALL govern read access to owned rows, and it SHALL be enforced identically at three layers: the application query predicate, the row-level security policy, and the database membership trigger.

For an owned row `r` and a user `me`, the row is visible to `me` if and only if:

```
owner_id = me
OR (owner_household_id IS NOT NULL
    AND owner_household_id IN (SELECT household_id FROM household_members WHERE user_id = me))
```

Personal rows (`owner_household_id IS NULL`) are owner-only: visible to the owner and to no one else. Household rows (`owner_household_id IS NOT NULL`) are visible to the owner and to every member of the owning household.

Write rules SHALL follow the same vocabulary:

- **Create** — `owner_id` is always stamped to the bound (creating) user. A household row (one whose `owner_household_id` is non-null) MAY be created only if the creating user is a member of that household.
- **Read (Get/List)** — uses the visibility rule.
- **Update** — uses the visibility rule (members may update household rows; the owner may update their own personal and household rows).
- **Delete** — is owner-only (`owner_id = me`); a member of a household SHALL NOT be able to delete a household row they do not own. Delete SHALL be app-enforced; the RLS backstop MAY be coarser on delete (it SHALL NOT itself block a member's delete) because RLS expresses the visibility rule and not the finer owner-only-delete rule.

#### Scenario: User can read their own personal row

- **WHEN** user `alice` requests a row whose `owner_id = 'alice'` and `owner_household_id IS NULL`
- **THEN** the row is returned to `alice`

#### Scenario: Member can read a household row

- **WHEN** user `bob` is a member of household `h1` and requests a row whose `owner_id = 'alice'` and `owner_household_id = 'h1'`
- **THEN** the row is returned to `bob`

#### Scenario: Another owner's personal row is not visible

- **WHEN** user `alice` requests a row whose `owner_id = 'bob'` and `owner_household_id IS NULL` and `alice` is not a member of any household owning that row
- **THEN** the row is not returned and no API response or repository result exposes it

#### Scenario: Member cannot DELETE a household row

- **WHEN** user `bob` (a member of household `h1` but not the owner of a specific row in it) issues a DELETE for that household row whose `owner_id = 'alice'`
- **THEN** the delete is blocked by the application layer (e.g. `404 Not Found`) even though RLS visibility would have permitted it, because delete is owner-only

### Requirement: Cross-owner isolation

No API response or repository result SHALL expose rows belonging to a different owner. Identifiers (asset ids, serial numbers, brand+model pairs) SHALL be scoped per owner (i.e. per owner and optional household): the same normalized serial number MAY exist in one owner scope and not in another, and a lookup in one owner scope SHALL NOT match rows of a different owner scope.

#### Scenario: Same serial in two owner scopes yields two assets

- **WHEN** user `alice` uploads a document extracting serial `SN1` under a personal scope and user `bob` uploads a different document extracting the same serial `SN1` under a different personal scope
- **THEN** two distinct assets exist (one per owner scope) and each owner's reads see only their own asset; a lookup by `SN1` in `alice`'s scope does not return `bob`'s asset

### Requirement: Households and membership

The system SHALL maintain a `households` table with columns `id`, `owner_id text NOT NULL` (foreign key to `users.id`), `display_name`, and `created_at`. The system SHALL maintain a `household_members` table with columns `household_id` (foreign key to `households.id`), `user_id` (foreign key to `users.id`), and `created_at`, with a composite primary key on `(household_id, user_id)`. Membership SHALL be a many-to-many relationship: a user MAY belong to multiple households, and a household MAY have multiple users.

#### Scenario: User is member of multiple households

- **WHEN** user `alice` is a member of households `h1` and `h2`
- **THEN** both memberships are recorded in `household_members` and active for visibility purposes

### Requirement: Household routes

All household routes SHALL live under the single `/api/users/{userId}` group and SHALL use the `{userId}` path parameter as the requester identity.

- `POST /api/users/{userId}/households` — body `{ "display_name": string }` (required, non-empty). Creates a household owned by `{userId}` and auto-adds `{userId}` as a member. `201` on success.
- `POST /api/users/{userId}/households/{householdId}/members` — body `{ "user_id": string }` (required, registered). Adds `user_id` as a member of `householdId`. The requester `{userId}` MUST be a member of `householdId` (otherwise `403`). Unknown household or user → `404`. `204` on success (idempotent; an existing member is left untouched).
- `GET /api/users/{userId}/households` — returns the households visible to `{userId}` (owned ∪ membered), each with its members. `200`.
- `GET /api/users/{userId}/households/{householdId}` — returns one household with its members, or `404` if unknown or not visible.

#### Scenario: Create household auto-adds the creator as a member

- **WHEN** `POST /api/users/alice/households` is called with body `{"display_name":"Main"}`
- **THEN** a new household owned by `alice` is created, `alice` is added to `household_members` for it, and the response is `201`

#### Scenario: Add member by non-member requester is rejected

- **WHEN** user `bob` (not a member of household `h1`) calls `POST /api/users/bob/households/h1/members` with body `{"user_id":"carol"}`
- **THEN** the response is `403` and `carol` is not added as a member of `h1`

### Requirement: Upload scope input

`POST /api/users/{userId}/documents` SHALL accept an optional multipart form field `owner_household_id`. If the field is present, the requester `{userId}` MUST be a member of the named household (otherwise `403`), and the resulting owned rows SHALL be stamped with `owner_household_id` set to that household. If the field is omitted, the upload is personal and the resulting rows SHALL have `owner_household_id IS NULL`.

#### Scenario: Upload with a non-member household is rejected

- **WHEN** user `bob` (not a member of household `h1`) sends `POST /api/users/bob/documents` with multipart field `owner_household_id=h1`
- **THEN** the response is `403` and no owned rows are created

#### Scenario: Personal upload omits the field

- **WHEN** user `alice` sends `POST /api/users/alice/documents` without the `owner_household_id` form field
- **THEN** the upload succeeds and the resulting rows have `owner_id = 'alice'` and `owner_household_id IS NULL`

### Requirement: Row-level security backstop

Row-level security SHALL be enabled and forced on all owned data tables (`sources`, `assets`, `documents`, `financial_accounts`, `money_movements`, `import_batches`, `import_lines`) and on `households` and `household_members`. Policies SHALL reference a `fn_visible_households()` helper (a plpgsql `BYPASSRLS` function that returns the caller's household ids and breaks the members↔households recursion) and the identity GUC `app.user_id`.

RLS SHALL serve as the visibility backstop: a session with no bound `app.user_id` (an "unbound" session) SHALL see and write nothing on any RLS-protected table, and personal rows (`owner_household_id IS NULL`) SHALL never leak across users. The finer owner-only-DELETE rule SHALL be app-enforced and not DB-enforced; RLS MAY be coarser on delete.

#### Scenario: Unbound RLS SELECT returns nothing

- **WHEN** a database session with no `app.user_id` GUC set executes a raw `SELECT` on `assets`
- **THEN** zero rows are returned regardless of the data present

#### Scenario: Bound RLS SELECT returns only the bound user's visible rows

- **WHEN** a database session bound to `app.user_id = 'alice'` executes a raw `SELECT` on `assets` with no explicit `WHERE` clause
- **THEN** only rows visible to `alice` under the visibility rule are returned (rows with `owner_id = 'alice'` plus rows in households `alice` is a member of)

### Requirement: Test user convention

Automated tests SHALL exercise all owner-scoped behavior using explicit, registered users (e.g. `test-user` and `test-user-b`) in the `users` registry. Tests SHALL NOT depend on a default or ambient user.

#### Scenario: Isolation test uses two explicit registered users

- **WHEN** the integration test suite runs cross-owner isolation cases
- **THEN** each case seeds and reads data under explicit, distinct registered user ids from the `users` registry

### Requirement: User-scoped file storage

Stored file content SHALL be laid out under a per-user key prefix of the form `{userId}/{fileID}`. The storage layer SHALL resolve the user from the request context.

#### Scenario: Uploaded file lands under the user prefix

- **WHEN** a document is uploaded in a context carrying user `alice`
- **THEN** the file bytes are stored under a key beginning with `alice/`
