# Life Manager — Database Schema Design

> This document defines the full PostgreSQL schema for Life Manager. It is the persistence
> translation of the canonical model described in `Life_Manager_Design.md`. This is a
> PoC-quality design — fields, tables, and relationships will evolve as stress tests
> are executed.

---

## 1. Key Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| **ID strategy** | UUID v7 | Time-sortable, globally unique, native in PG 18+ |
| **Multitenancy** | `tenant_id` on every table | Row-level isolation, consistent with existing approach |
| **Canonical object pattern** | Base `canonical_objects` + typed child tables | Shared infrastructure without one giant table |
| **Assertions** | Versioned (`version_number` per fact key) | Corrections create new versions; old ones become `superseded` |
| **Relationships** | Typed directional junction table | Small controlled vocabulary + metadata; not arbitrary graph |
| **Impacts** | First-class table, not event bus | Domain consequences only; infrastructure reactions use jobs |
| **Derived state** | Materialized summary tables | Reconstructable from authoritative data; not source of truth |
| **Money** | `numeric` (decimal), never float | Financial correctness |
| **Search** | pgvector + pg full-text + pg_trgm | Semantic + keyword + fuzzy |
| **Extensions** | JSONB on canonical objects | Controlled extensibility; promote to columns when stable |
| **Time** | Multiple semantic timestamps per type | `occurred_at`, `recorded_at`, `effective_at` where needed |
| **Graph** | Not a graph DB; typed relationships with indexes | Sufficient traversal without operational complexity |

---

## 2. Schema Layers

The schema is organized into six layers:

```
Layer 1: Infrastructure     (tenants, sources, actors, scopes, canonical_objects,
                             assertions, relationships, impacts, derived_state)
Layer 2: People & Household (persons, households, household_members, users)
Layer 3: Documents          (documents)
Layer 4: Tasks              (tasks, task_assignments)
Layer 5: Finance            (financial_accounts, money_movements, allocations,
                             debts, settlements)
Layer 6: Inventory & Assets (product_definitions, purchases, purchase_lines,
                             inventory_holdings, physical_assets, warranties,
                             maintenance_events)
```

---

## 3. Layer 1: Infrastructure

### 3.1 `tenants`

Isolation boundary. Every table carries `tenant_id` referencing this.

```sql
CREATE TABLE tenants (
    id              text PRIMARY KEY,
    display_name    text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
```

### 3.2 `sources`

Raw evidence. Retained for fidelity and provenance. Survives downstream failures.

```sql
CREATE TABLE sources (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    filename        text NOT NULL,
    content_type    text NOT NULL,
    byte_size       bigint NOT NULL,
    storage_path    text NOT NULL,
    sha256          text NOT NULL,
    uploaded_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_sources_tenant ON sources(tenant_id);
CREATE INDEX idx_sources_sha256 ON sources(tenant_id, sha256);
```

### 3.3 `actors`

Who or what performed an action or made an assertion. Separate from Person/User.

```sql
CREATE TABLE actors (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    actor_type      text NOT NULL CHECK (actor_type IN (
                        'person', 'system', 'llm', 'external', 'scheduled'
                    )),
    person_id       uuid REFERENCES persons(canonical_id),
    display_name    text NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_actors_tenant ON actors(tenant_id);
CREATE INDEX idx_actors_person ON actors(tenant_id, person_id)
    WHERE person_id IS NOT NULL;
```

### 3.4 `scopes`

Ownership and access boundary. Personal or household.

```sql
CREATE TABLE scopes (
    id                  uuid PRIMARY KEY,
    tenant_id           text NOT NULL REFERENCES tenants(id),
    scope_type          text NOT NULL CHECK (scope_type IN ('personal', 'household')),
    owner_person_id     uuid,
    owner_household_id  uuid,
    display_name        text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_scope_owner CHECK (
        (owner_person_id IS NOT NULL AND owner_household_id IS NULL)
        OR
        (owner_person_id IS NULL AND owner_household_id IS NOT NULL)
    )
);

CREATE INDEX idx_scopes_tenant ON scopes(tenant_id);

-- FK constraints added after persons/households tables exist (Layer 2).
-- Use ALTER TABLE in a later migration to avoid circular dependency.
-- ALTER TABLE scopes ADD CONSTRAINT fk_scope_owner_person
--     FOREIGN KEY (owner_person_id) REFERENCES persons(canonical_id);
-- ALTER TABLE scopes ADD CONSTRAINT fk_scope_owner_household
--     FOREIGN KEY (owner_household_id) REFERENCES households(canonical_id);
```

### 3.5 `canonical_objects`

Base table for every entity in the system. Typed child tables provide domain-specific fields via 1:1 FK.

```sql
CREATE TABLE canonical_objects (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    entity_kind     text NOT NULL,
    scope_id        uuid REFERENCES scopes(id),
    status          text NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'archived', 'deleted')),
    metadata        jsonb NOT NULL DEFAULT '{}',
    embedding       vector(1536),
    search_vector   tsvector,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    created_by_actor_id uuid REFERENCES actors(id),
    updated_by_actor_id uuid REFERENCES actors(id)
);

CREATE INDEX idx_co_tenant_kind ON canonical_objects(tenant_id, entity_kind);
CREATE INDEX idx_co_tenant_scope ON canonical_objects(tenant_id, scope_id);
CREATE INDEX idx_co_status ON canonical_objects(tenant_id, status);
CREATE INDEX idx_co_search ON canonical_objects USING gin(search_vector);
CREATE INDEX idx_co_embedding ON canonical_objects
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);
```

### 3.6 `assertions`

Provenance and confidence for every fact. Versioned per canonical object + fact key.

```sql
CREATE TABLE assertions (
    id                  uuid PRIMARY KEY,
    tenant_id           text NOT NULL REFERENCES tenants(id),
    canonical_id        uuid NOT NULL REFERENCES canonical_objects(id),
    fact_key            text NOT NULL,
    version_number      int NOT NULL,
    asserted_value      jsonb NOT NULL,
    source_id           uuid REFERENCES sources(id),
    actor_id            uuid REFERENCES actors(id),
    confidence          numeric(3,2),
    status              text NOT NULL DEFAULT 'proposed'
                            CHECK (status IN (
                                'proposed', 'accepted', 'rejected', 'superseded'
                            )),
    validation_method   text,
    model_id            text,
    model_version       text,
    superseded_by       uuid REFERENCES assertions(id),
    created_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_assertion_version
        UNIQUE (tenant_id, canonical_id, fact_key, version_number)
);

CREATE INDEX idx_assertions_canonical ON assertions(tenant_id, canonical_id);
CREATE INDEX idx_assertions_fact_chain ON assertions(tenant_id, canonical_id, fact_key);
CREATE INDEX idx_assertions_source ON assertions(tenant_id, source_id);
CREATE INDEX idx_assertions_pending ON assertions(tenant_id, status, created_at)
    WHERE status = 'proposed';
```

### 3.7 `relationships`

Typed, directional connections between canonical objects.

```sql
CREATE TABLE relationships (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    rel_type        text NOT NULL,
    source_id       uuid NOT NULL REFERENCES canonical_objects(id),
    target_id       uuid NOT NULL REFERENCES canonical_objects(id),
    metadata        jsonb NOT NULL DEFAULT '{}',
    valid_from      date,
    valid_until     date,
    assertion_id    uuid REFERENCES assertions(id),
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_relationship UNIQUE (tenant_id, rel_type, source_id, target_id)
);

CREATE INDEX idx_rel_source ON relationships(tenant_id, source_id, rel_type);
CREATE INDEX idx_rel_target ON relationships(tenant_id, target_id, rel_type);
CREATE INDEX idx_rel_type ON relationships(tenant_id, rel_type);
```

**Initial relationship vocabulary:**

| rel_type | source entity | target entity | semantics |
|---|---|---|---|
| `contains` | Purchase | Purchase Line | composition |
| `refers_to` | Purchase Line | Product | acquisition reference |
| `funds` | Financial Account | Money Movement | source of funds |
| `caused_by` | Money Movement | Purchase | what triggered it |
| `owes` | Person | Debt | debtor |
| `creditor_of` | Person | Debt | creditor |
| `settled_by` | Settlement | Debt | settlement record |
| `covers` | Warranty | Physical Asset | warranty coverage |
| `assigned_to` | Task | Person | task assignment |
| `participates_in` | Person | Purchase | involvement |
| `located_at` | Physical Asset | Place | physical location |
| `household_member` | Person | Household | membership |
| `produced` | Purchase Line | Inventory Holding | inventory creation |
| `produced` | Purchase Line | Money Movement | financial impact |
| `reduces` | Settlement | Debt | debt reduction |

### 3.8 `impacts`

Domain consequences of accepted canonical information. Not an event bus.

```sql
CREATE TABLE impacts (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    impact_type     text NOT NULL CHECK (impact_type IN (
                        'money_movement', 'inventory_change', 'debt_change',
                        'asset_status_change', 'task_created',
                        'warranty_expiry', 'other'
                    )),
    cause_type      text NOT NULL CHECK (cause_type IN (
                        'assertion', 'relationship', 'manual_correction', 'system'
                    )),
    cause_assertion_id   uuid REFERENCES assertions(id),
    cause_relationship_id uuid REFERENCES relationships(id),
    affected_id     uuid NOT NULL REFERENCES canonical_objects(id),
    amount          numeric,
    currency        char(3),
    metadata        jsonb NOT NULL DEFAULT '{}',
    reconciled      boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_impact_cause CHECK (
        (cause_type = 'assertion' AND cause_assertion_id IS NOT NULL AND cause_relationship_id IS NULL)
        OR
        (cause_type = 'relationship' AND cause_relationship_id IS NOT NULL AND cause_assertion_id IS NULL)
        OR
        (cause_type IN ('manual_correction', 'system')
            AND cause_assertion_id IS NULL AND cause_relationship_id IS NULL)
    )
);

CREATE INDEX idx_impacts_affected ON impacts(tenant_id, affected_id);
CREATE INDEX idx_impacts_type ON impacts(tenant_id, impact_type);
CREATE INDEX idx_impacts_unreconciled ON impacts(tenant_id, reconciled)
    WHERE NOT reconciled;
```

### 3.9 `derived_state`

Materialized current state. Reconstructable from authoritative data.

```sql
CREATE TABLE derived_state (
    id                  uuid PRIMARY KEY,
    tenant_id           text NOT NULL REFERENCES tenants(id),
    canonical_id        uuid NOT NULL REFERENCES canonical_objects(id),
    state_key           text NOT NULL,
    state_value         jsonb NOT NULL,
    last_computed_at    timestamptz NOT NULL DEFAULT now(),
    source_impact_ids   uuid[],

    CONSTRAINT uq_derived_state UNIQUE (tenant_id, canonical_id, state_key)
);

CREATE INDEX idx_derived_state_canonical ON derived_state(tenant_id, canonical_id);
```

---

## 4. Layer 2: People & Household

### 4.1 `persons`

```sql
CREATE TABLE persons (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    display_name    text NOT NULL,
    given_name      text,
    family_name     text,
    nickname        text,
    email           text,
    phone           text,
    notes           text
);

CREATE INDEX idx_persons_tenant ON persons(tenant_id);
CREATE INDEX idx_persons_name ON persons(tenant_id, display_name);
```

### 4.2 `households`

```sql
CREATE TABLE households (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    name            text NOT NULL,
    notes           text
);

CREATE INDEX idx_households_tenant ON households(tenant_id);
```

### 4.3 `household_members`

Optimized for the common "who's in this household" query. Redundant with `relationships` but avoids traversal for this高频 access pattern.

```sql
CREATE TABLE household_members (
    household_id    uuid NOT NULL REFERENCES households(canonical_id),
    person_id       uuid NOT NULL REFERENCES persons(canonical_id),
    role            text NOT NULL DEFAULT 'member'
                        CHECK (role IN ('owner', 'member', 'guest')),
    joined_at       date,
    left_at         date,

    PRIMARY KEY (household_id, person_id)
);
```


### 4.4 `users`

Authenticated Life Manager accounts. Distinct from persons — a person may exist
without being a user and may become a user later without recreating their identity.

```sql
CREATE TABLE users (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    person_id       uuid UNIQUE REFERENCES persons(canonical_id),
    email           text NOT NULL,
    password_hash   text NOT NULL,
    status          text NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'inactive')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_users_tenant_email UNIQUE (tenant_id, email)
);

CREATE INDEX idx_users_tenant ON users(tenant_id);
CREATE INDEX idx_users_person ON users(tenant_id, person_id)
    WHERE person_id IS NOT NULL;
```

---

## 5. Layer 3: Documents

### 5.1 `documents`

Links a raw Source to a Canonical Object with classification and extraction data.

```sql
CREATE TABLE documents (
    id                        uuid PRIMARY KEY,
    tenant_id                 text NOT NULL REFERENCES tenants(id),
    source_id                 uuid NOT NULL UNIQUE REFERENCES sources(id),
    canonical_id              uuid NOT NULL REFERENCES canonical_objects(id),
    doc_type                  text NOT NULL CHECK (doc_type IN (
                                  'invoice', 'warranty', 'amc', 'receipt',
                                  'email', 'sms', 'photo', 'other'
                              )),
    classification_confidence numeric(3,2),
    extracted_fields          jsonb NOT NULL,
    raw_extraction            jsonb NOT NULL,
    created_at                timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_documents_tenant ON documents(tenant_id);
CREATE INDEX idx_documents_canonical ON documents(tenant_id, canonical_id);
CREATE INDEX idx_documents_type ON documents(tenant_id, doc_type);
```

---

## 6. Layer 4: Tasks

### 6.1 `tasks`

```sql
CREATE TABLE tasks (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    title           text NOT NULL,
    description     text,
    priority        text NOT NULL DEFAULT 'medium'
                        CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    due_date        date,
    due_time        time,
    recurrence_rule text,
    status          text NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'in_progress', 'completed', 'cancelled')),
    completed_at    timestamptz
);

CREATE INDEX idx_tasks_tenant ON tasks(tenant_id);
CREATE INDEX idx_tasks_status ON tasks(tenant_id, status);
CREATE INDEX idx_tasks_due ON tasks(tenant_id, due_date) WHERE due_date IS NOT NULL;
```

### 6.2 `task_assignments`

```sql
CREATE TABLE task_assignments (
    task_id         uuid NOT NULL REFERENCES tasks(canonical_id),
    person_id       uuid NOT NULL REFERENCES persons(canonical_id),
    assigned_at     timestamptz NOT NULL DEFAULT now(),
    assigned_by     uuid REFERENCES actors(id),

    PRIMARY KEY (task_id, person_id)
);
```

---

## 7. Layer 5: Finance

### 7.1 `financial_accounts`

Where money is held or tracked.

```sql
CREATE TABLE financial_accounts (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    account_name    text NOT NULL,
    account_type    text NOT NULL CHECK (account_type IN (
                        'bank', 'credit_card', 'cash', 'wallet',
                        'investment', 'other'
                    )),
    currency        char(3) NOT NULL,
    institution     text,
    is_active       boolean NOT NULL DEFAULT true,

    CONSTRAINT uq_account_tenant_name UNIQUE (tenant_id, account_name)
);

CREATE INDEX idx_financial_accounts_tenant ON financial_accounts(tenant_id);
```

### 7.2 `money_movements`

An actual movement of money. Not the same as a purchase.

```sql
CREATE TABLE money_movements (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    amount          numeric NOT NULL,
    currency        char(3) NOT NULL,
    movement_type   text NOT NULL CHECK (movement_type IN ('debit', 'credit', 'transfer')),
    occurred_at     timestamptz NOT NULL,
    recorded_at     timestamptz NOT NULL DEFAULT now(),
    description     text,
    counterparty    text,
    account_id      uuid REFERENCES financial_accounts(canonical_id)
);

CREATE INDEX idx_money_movements_tenant ON money_movements(tenant_id);
CREATE INDEX idx_money_movements_account ON money_movements(tenant_id, account_id);
CREATE INDEX idx_money_movements_date ON money_movements(tenant_id, occurred_at);
```

### 7.3 `allocations`

How a money movement or expense is attributed across people, purchases, or categories.

```sql
CREATE TABLE allocations (
    id              uuid PRIMARY KEY,
    tenant_id       text NOT NULL REFERENCES tenants(id),
    movement_id     uuid NOT NULL REFERENCES money_movements(canonical_id),
    canonical_id    uuid NOT NULL REFERENCES canonical_objects(id),
    person_id       uuid REFERENCES persons(canonical_id),
    amount          numeric NOT NULL,
    percentage      numeric(5,2),
    allocation_type text NOT NULL CHECK (allocation_type IN (
                        'purchase', 'debt', 'subscription', 'other'
                    )),
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_allocations_tenant ON allocations(tenant_id);
CREATE INDEX idx_allocations_movement ON allocations(tenant_id, movement_id);
CREATE INDEX idx_allocations_canonical ON allocations(tenant_id, canonical_id);
```

### 7.4 `debts`

An outstanding obligation between parties.

```sql
CREATE TABLE debts (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    debtor_id       uuid NOT NULL REFERENCES persons(canonical_id),
    creditor_id     uuid NOT NULL REFERENCES persons(canonical_id),
    original_amount numeric NOT NULL,
    currency        char(3) NOT NULL,
    description     text,
    incurred_at     timestamptz,
    status          text NOT NULL DEFAULT 'outstanding'
                        CHECK (status IN (
                            'outstanding', 'partially_settled', 'settled', 'written_off'
                        ))
);

CREATE INDEX idx_debts_tenant ON debts(tenant_id);
CREATE INDEX idx_debts_debtor ON debts(tenant_id, debtor_id);
CREATE INDEX idx_debts_creditor ON debts(tenant_id, creditor_id);
CREATE INDEX idx_debts_outstanding ON debts(tenant_id, status)
    WHERE status IN ('outstanding', 'partially_settled');
```

### 7.5 `settlements`

A payment or action that reduces or closes a debt.

```sql
CREATE TABLE settlements (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    debt_id         uuid NOT NULL REFERENCES debts(canonical_id),
    movement_id     uuid REFERENCES money_movements(canonical_id),
    amount          numeric NOT NULL,
    settled_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_settlements_tenant ON settlements(tenant_id);
CREATE INDEX idx_settlements_debt ON settlements(tenant_id, debt_id);
```

---

## 8. Layer 6: Inventory & Assets

### 8.1 `product_definitions`

What a kind of thing is, independent of any particular purchase or household holding.

```sql
CREATE TABLE product_definitions (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    name            text NOT NULL,
    brand           text,
    model           text,
    norm_name       text,
    norm_brand      text,
    norm_model      text,
    category        text,
    unit            text,
    description     text,

    CONSTRAINT uq_product_tenant_name UNIQUE (tenant_id, name)
);

CREATE INDEX idx_products_tenant ON product_definitions(tenant_id);
CREATE INDEX idx_products_name ON product_definitions(tenant_id, name);
CREATE INDEX idx_products_brand_model ON product_definitions(tenant_id, brand, model);
CREATE INDEX idx_products_norm_brand_model ON product_definitions(tenant_id, norm_brand, norm_model);
```

### 8.2 `purchases`

The real-world acquisition event.

```sql
CREATE TABLE purchases (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    total_amount    numeric,
    currency        char(3),
    purchase_date   date,
    merchant        text,
    notes           text
);

CREATE INDEX idx_purchases_tenant ON purchases(tenant_id);
CREATE INDEX idx_purchases_date ON purchases(tenant_id, purchase_date);
```

### 8.3 `purchase_lines`

The acquisition-specific bridge between a purchase and what was acquired. First-class canonical object.

```sql
CREATE TABLE purchase_lines (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    purchase_id     uuid NOT NULL REFERENCES purchases(canonical_id),
    product_id      uuid REFERENCES product_definitions(canonical_id),
    quantity        numeric NOT NULL DEFAULT 1,
    unit_price      numeric,
    total_price     numeric,
    currency        char(3),
    description     text
);

CREATE INDEX idx_purchase_lines_tenant ON purchase_lines(tenant_id);
CREATE INDEX idx_purchase_lines_purchase ON purchase_lines(tenant_id, purchase_id);
CREATE INDEX idx_purchase_lines_product ON purchase_lines(tenant_id, product_id)
    WHERE product_id IS NOT NULL;
```

### 8.4 `inventory_holdings`

Quantity-based household holdings of a product. Current quantity is derived/materialized state.

```sql
CREATE TABLE inventory_holdings (
    canonical_id        uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id           text NOT NULL REFERENCES tenants(id),
    product_id          uuid NOT NULL REFERENCES product_definitions(canonical_id),
    current_quantity    numeric NOT NULL DEFAULT 0,
    unit                text NOT NULL,
    location            text,
    reorder_threshold   numeric,
    notes               text
);

CREATE INDEX idx_inventory_tenant ON inventory_holdings(tenant_id);
CREATE INDEX idx_inventory_product ON inventory_holdings(tenant_id, product_id);
CREATE INDEX idx_inventory_low ON inventory_holdings(tenant_id, current_quantity, reorder_threshold)
    WHERE reorder_threshold IS NOT NULL;
```

### 8.5 `physical_assets`

Individually identifiable persistent objects (appliances, electronics, etc.).

```sql
CREATE TABLE physical_assets (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    product_id      uuid REFERENCES product_definitions(canonical_id),
    serial_number   text,
    norm_serial     text,
    brand           text,
    model           text,
    purchase_date   date,
    price           numeric,
    currency        char(3),
    status          text NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'disposed', 'lost', 'retired'))
);

CREATE UNIQUE INDEX uniq_pa_tenant_norm_serial
    ON physical_assets(tenant_id, norm_serial)
    WHERE norm_serial IS NOT NULL;

CREATE INDEX idx_pa_tenant ON physical_assets(tenant_id);
CREATE INDEX idx_pa_status ON physical_assets(tenant_id, status);
```

### 8.6 `warranties`

Coverage for a physical asset.

```sql
CREATE TABLE warranties (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    asset_id        uuid NOT NULL REFERENCES physical_assets(canonical_id),
    provider        text,
    start_date      date NOT NULL,
    end_date        date NOT NULL,
    terms           text,
    document_id     uuid REFERENCES documents(id),

    CONSTRAINT chk_warranty_dates CHECK (end_date >= start_date)
);

CREATE INDEX idx_warranties_tenant ON warranties(tenant_id);
CREATE INDEX idx_warranties_asset ON warranties(tenant_id, asset_id);
CREATE INDEX idx_warranties_expiry ON warranties(tenant_id, end_date)
    WHERE end_date >= CURRENT_DATE;
```

### 8.7 `maintenance_events`

Service or maintenance on a physical asset.

```sql
CREATE TABLE maintenance_events (
    canonical_id    uuid PRIMARY KEY REFERENCES canonical_objects(id),
    tenant_id       text NOT NULL REFERENCES tenants(id),
    asset_id        uuid NOT NULL REFERENCES physical_assets(canonical_id),
    event_type      text NOT NULL CHECK (event_type IN (
                        'service', 'repair', 'inspection', 'cleaning', 'other'
                    )),
    performed_at    date NOT NULL,
    performed_by    text,
    cost            numeric,
    currency        char(3),
    description     text,
    next_due        date
);

CREATE INDEX idx_maintenance_tenant ON maintenance_events(tenant_id);
CREATE INDEX idx_maintenance_asset ON maintenance_events(tenant_id, asset_id);
CREATE INDEX idx_maintenance_next_due ON maintenance_events(tenant_id, next_due)
    WHERE next_due IS NOT NULL;
```

---

## 9. Search Architecture

Three complementary search mechanisms, all PostgreSQL-native:

### 9.1 Semantic Search (pgvector)

- `canonical_objects.embedding` — vector(1536), populated asynchronously after creation/update
- HNSW index for approximate nearest-neighbor with cosine distance
- Use case: "find things similar to this receipt" / semantic queries via LLM

### 9.2 Full-Text Search (tsvector)

- `canonical_objects.search_vector` — maintained via triggers from child table text fields
- GIN index for fast full-text queries
- Use case: keyword search across entity names, descriptions, notes

### 9.3 Fuzzy Search (pg_trgm)

- Trigram indexes on key text columns (brand, model, name, merchant, serial_number)
- Use case: "find the washing machne" matching "Washing Machine"

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Example trigram indexes
CREATE INDEX idx_pa_brand_trgm ON physical_assets
    USING gin (brand gin_trgm_ops);
CREATE INDEX idx_products_name_trgm ON product_definitions
    USING gin (name gin_trgm_ops);
```

### 9.4 Search Strategy

```
User question
    |
    v
LLM / parser interpretation
    |
    v
Query plan (structured + full-text + semantic + trigram)
    |
    v
PostgreSQL execution
    |
    v
Results ranked by relevance
    |
    v
Optional natural-language explanation via LLM
```

---

## 10. Migration Strategy

### Current state

The existing `00001_init.sql` migration creates `sources`, `assets`, and `documents` with a simpler model. This needs to be replaced.

### Recommended approach

1. **Rewrite migration 00001** to create the full schema above (this is a PoC, clean break is fine)
2. **Use goose** (already in the project) for all future migrations
3. **One migration per layer** for initial implementation:
   - `00001_infrastructure.sql` — Layer 1 tables
   - `00002_people.sql` — Layer 2 tables
   - `00003_documents.sql` — Layer 3 tables
   - `00004_tasks.sql` — Layer 4 tables
   - `00005_finance.sql` — Layer 5 tables
   - `00006_inventory.sql` — Layer 6 tables
4. **Extensions** (`pgvector`, `pg_trgm`) in the first migration
5. **All future changes** as incremental goose migrations

### ID generation

All UUIDs are generated in Go application code using `google/uuid` with `NewV7()`.
No SQL `DEFAULT` is used for primary keys — the application is responsible for ID
generation on every INSERT.

---

## 11. Relationship to Current PoC

The current `00001_init.sql` has:
- `sources` — maps directly to the new `sources` table
- `assets` — will be replaced by `canonical_objects` + `physical_assets` child table
- `documents` — maps to the new `documents` table with expanded fields

### Migration path

Since this is a PoC that can be discarded:

1. Drop the existing database
2. Run the new migrations from scratch
3. Re-implement the ingest service to use the new canonical object pattern
4. The existing identity resolution logic (`core/identity/resolve.go`) maps naturally to the new model:
   - Serial number matching → `physical_assets.norm_serial` lookup
   - Brand+model matching → `physical_assets.norm_brand + norm_model` lookup
   - The `canonical_objects.entity_kind = 'physical_asset'` pattern replaces the flat `assets` table

---


## 11.1 Review Fixes Applied

The following fixes were applied after architecture review:

- **FK constraints**: `actors.person_id` now references `persons(canonical_id)`
- **Polymorphic cause eliminated**: `impacts.cause_id` replaced with typed `cause_assertion_id` / `cause_relationship_id` columns + CHECK constraint
- **UUID v7 enforced**: Removed all `DEFAULT gen_random_uuid()` — app generates v7 IDs
- **Audit columns**: `canonical_objects` now tracks `created_by_actor_id` and `updated_by_actor_id`
- **Product identity resolution**: `product_definitions` now has `norm_name`, `norm_brand`, `norm_model` columns + unique constraint on `(tenant_id, name)`
- **Account uniqueness**: `financial_accounts` now has unique constraint on `(tenant_id, account_name)`
- **Users table**: Added `users` table to Layer 2 for authenticated accounts distinct from persons

## 12. Open Questions

These remain open and should be resolved during implementation:

1. **Scope owner FKs**: `scopes.owner_person_id` / `owner_household_id` intentionally lack FK constraints to avoid circular dependency between Layer 1 and Layer 2. Add them via ALTER TABLE in a later migration, or enforce at the application layer.
2. **Soft delete**: The `status = 'deleted'` approach means child tables are never physically deleted. Is this sufficient?
3. **Search vector triggers**: Which child table fields populate `canonical_objects.search_vector`? Likely a per-kind trigger.
4. **Embedding population**: Should embeddings be populated synchronously (slower upload) or asynchronously (eventual consistency)?
5. **Allocations**: The current model has allocations referencing both a movement and a canonical object. Is this sufficient for shared expenses?
6. **Place/Location**: Mentioned in the design doc as a missing cross-cutting primitive. Deferred for now.
7. **Recurring patterns**: Tasks have `recurrence_rule` but recurring purchases, bills, and expected patterns are not yet modeled.
8. **User authentication**: The `users` table stores `password_hash` but the auth mechanism (local, OIDC) is not yet decided.
