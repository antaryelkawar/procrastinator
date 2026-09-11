# asset-registry — Delta

## MODIFIED Requirements

### Requirement: Canonical asset model

The system SHALL persist assets alongside all entities in the uniform jsonb shape (see `jsonb-entity-storage`), scoped per owner (and optional household) by key columns `id`, `owner_id`, `owner_household_id`, plus audit `created_at`/`updated_at` and a `payload` jsonb column. The payload SHALL hold all asset data — brand, model, serial number, purchase date, warranty end, price, currency, name, normalized identity fields (`norm_name`, `norm_brand`, `norm_model`, `norm_serial`), intrinsic category (+ confidence, `category_user_set`), confidence, lifecycle state (`deleted_at`, `merged_into`, `merged_at`) and raw/derived extraction metadata. *(Previously: the asset had a structured core of columns — brand, model, serial, purchase date, warranty end, price, currency — plus a separate `metadata` jsonb object whose fields were excluded from identity resolution; this change folds everything but keys/audit into the payload.)* All extracted fields remain optional except owner scoping; lifecycle fields use domain defaults where semantics require columns (`deleted_at IS NULL` active state).

Uniqueness of the normalized serial number (when present) SHALL be guarded by a partial unique jsonb expression index: `CREATE UNIQUE INDEX ... ON assets ((payload #>> '{data,norm_serial}')) WHERE payload #>> '{data,norm_serial}' IS NOT NULL AND deleted_at IS NULL` expressed over the lifecycle rule (a real `deleted_at` column may host the predicate; implementation choice documented at design time). *(Previously: norm-serial uniqueness was guarded by a unique index on a physical `norm_serial` column.)* The asset no longer carries a `doc_type` column or check constraint — that vocabulary stays intrinsic to the document (per the archived asset_rework lineage); category is the asset's intrinsic taxonomy inside the payload. Identity-resolution lookups over payload paths SHALL use jsonb indexes (per `jsonb-entity-storage`).

#### Scenario: Asset persists the full extracted field set

- **WHEN** an asset is created from a complete extraction
- **THEN** reading the asset back (API + repo) returns the same brand, model, serial number, purchase date, price, currency, name, and category — all from the payload

#### Scenario: Partially known data still forms an asset

- **WHEN** an extraction yields only brand and model
- **THEN** an asset can be created with every other payload field absent (no entries or explicit nulls)

#### Scenario: Duplicate serial does not create a second asset (carried over)

- **WHEN** two uploads extract the same normalized serial number and no asset with that serial existed before either upload
- **THEN** exactly one asset with that serial exists after both uploads complete — enforced without a physical `norm_serial` column

#### Scenario: Money representation round-trips (carried over)

- **WHEN** an asset is created with price `39999.99` and currency `INR`
- **THEN** reading the asset returns `39999.99` and `INR` exactly, without floating-point drift

### Requirement: Asset creation input is directive-first

Asset creation SHALL be reachable from the `/assets` corner [+] (per `app-chrome`) and SHALL use the intuitive input pattern of `intuitive-input` — directive-first composer with progressive-disclosure review chips — with all resulting values persisted into the payload exactly as extraction-produced values today. The legacy field-by-field asset form SHALL NOT be the primary creation input. Nothing about the asset's stored/persisted shape changes: payload contents, identity indexes, prices round-trip all as specified above.

#### Scenario: Composer-derived asset round-trips like an extracted one

- **WHEN** the user describes an asset in the composer ("vacuum cleaner, brand Philips, paid 3200"), confirms the review chips, and proceeds
- **THEN** the asset is created via the same asset creation path with payload values exactly as described (price 3200 including correct currency default), and subsequent identity-resolution and search behave identically to an extraction-created asset

#### Scenario: Composer asset participates in the duplicate-serial rule

- **WHEN** a composer-created asset's description yields a serial number matching an existing asset's
- **THEN** the duplicate-serial rule above (single asset per serial) applies unchanged — composer-created assets are first-class
