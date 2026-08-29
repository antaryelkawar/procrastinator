-- Household/scope schema (change: multi-tenant-isolation).
-- RLS is intentionally UNCHANGED in this migration: the isolation policy stays
-- at the tenant level (00003) and household-based scope filtering is enforced
-- at the application level, per the spec's "Row-level security
-- defense-in-depth" requirement. This migration only adds the household and
-- scope schema; it neither enables nor alters any RLS policy.
-- Households are provisioned explicitly by the application; no seeding here.
-- +goose Up

-- A household id is a user-shaped id: same pattern as tenants.id, so id
-- formats stay consistent with the user registry.
CREATE TABLE households (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    -- The household's owning user/tenant.
    tenant_id text NOT NULL REFERENCES tenants (id),
    display_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Many-to-many membership: a user may belong to multiple households and a
-- household may have multiple users; membership is unique per (household, user).
CREATE TABLE household_members (
    household_id text NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    -- Members are users from the registry.
    user_id text NOT NULL REFERENCES tenants (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);

-- Scope dimension on tenant-owned rows. Pre-existing rows are backfilled
-- implicitly: scope_type DEFAULT 'personal' applies to existing rows and
-- owner_household_id is nullable, so no separate backfill UPDATE is needed.
ALTER TABLE sources ADD COLUMN scope_type text NOT NULL DEFAULT 'personal';
ALTER TABLE sources ADD CONSTRAINT sources_scope_type_chk CHECK (scope_type IN ('personal','household'));
ALTER TABLE sources ADD COLUMN owner_household_id text REFERENCES households (id);
-- Mutual exclusion: personal rows have no household owner; household rows name one.
ALTER TABLE sources ADD CONSTRAINT sources_scope_exclusive_chk
    CHECK ((scope_type = 'personal' AND owner_household_id IS NULL)
         OR (scope_type = 'household' AND owner_household_id IS NOT NULL));

ALTER TABLE assets ADD COLUMN scope_type text NOT NULL DEFAULT 'personal';
ALTER TABLE assets ADD CONSTRAINT assets_scope_type_chk CHECK (scope_type IN ('personal','household'));
ALTER TABLE assets ADD COLUMN owner_household_id text REFERENCES households (id);
ALTER TABLE assets ADD CONSTRAINT assets_scope_exclusive_chk
    CHECK ((scope_type = 'personal' AND owner_household_id IS NULL)
         OR (scope_type = 'household' AND owner_household_id IS NOT NULL));

ALTER TABLE documents ADD COLUMN scope_type text NOT NULL DEFAULT 'personal';
ALTER TABLE documents ADD CONSTRAINT documents_scope_type_chk CHECK (scope_type IN ('personal','household'));
ALTER TABLE documents ADD COLUMN owner_household_id text REFERENCES households (id);
ALTER TABLE documents ADD CONSTRAINT documents_scope_exclusive_chk
    CHECK ((scope_type = 'personal' AND owner_household_id IS NULL)
         OR (scope_type = 'household' AND owner_household_id IS NOT NULL));

-- A household-scoped row's owning household must belong to the same tenant as
-- the row. A pure CHECK constraint cannot reference another table, so this is
-- enforced by a BEFORE INSERT OR UPDATE trigger function (DB-enforced,
-- consistent with this change's RLS/FK defense-in-depth). The alternative
-- considered was a documented application-level rule; the trigger keeps the
-- invariant in the database.
-- goose splits statements on ';' and does not parse plpgsql $$ bodies, so the
-- function is wrapped in StatementBegin/End and applied as a single statement.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION enforce_scope_matching_tenant()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    -- Personal rows are the common path: no cross-table check needed.
    IF NEW.scope_type = 'household' AND NEW.owner_household_id IS NOT NULL THEN
        IF NOT EXISTS (SELECT 1 FROM households WHERE id = NEW.owner_household_id) THEN
            -- The FK would catch this too; be explicit.
            RAISE EXCEPTION 'household "%" does not exist', NEW.owner_household_id
                USING ERRCODE = 'foreign_key_violation';
        ELSIF (SELECT tenant_id FROM households WHERE id = NEW.owner_household_id) IS DISTINCT FROM NEW.tenant_id THEN
            RAISE EXCEPTION 'household "%" is owned by a different tenant', NEW.owner_household_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER sources_scope_matching_tenant
    BEFORE INSERT OR UPDATE ON sources
    FOR EACH ROW EXECUTE FUNCTION enforce_scope_matching_tenant();
CREATE TRIGGER assets_scope_matching_tenant
    BEFORE INSERT OR UPDATE ON assets
    FOR EACH ROW EXECUTE FUNCTION enforce_scope_matching_tenant();
CREATE TRIGGER documents_scope_matching_tenant
    BEFORE INSERT OR UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION enforce_scope_matching_tenant();

-- Indexes for the spec's access patterns.
CREATE INDEX idx_households_tenant ON households (tenant_id);
-- The composite PK covers the household_id prefix; "which households is user X
-- in" needs its own index.
CREATE INDEX idx_household_members_user ON household_members (user_id);
-- Household-scoped rows are looked up by their owning household.
CREATE INDEX idx_sources_owner_household ON sources (owner_household_id) WHERE owner_household_id IS NOT NULL;
CREATE INDEX idx_assets_owner_household ON assets (owner_household_id) WHERE owner_household_id IS NOT NULL;
CREATE INDEX idx_documents_owner_household ON documents (owner_household_id) WHERE owner_household_id IS NOT NULL;

-- +goose Down
DROP TRIGGER IF EXISTS sources_scope_matching_tenant ON sources;
DROP TRIGGER IF EXISTS assets_scope_matching_tenant ON assets;
DROP TRIGGER IF EXISTS documents_scope_matching_tenant ON documents;
DROP FUNCTION IF EXISTS enforce_scope_matching_tenant();

ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_scope_type_chk;
ALTER TABLE sources DROP CONSTRAINT IF EXISTS sources_scope_exclusive_chk;
ALTER TABLE sources DROP COLUMN IF EXISTS scope_type;
ALTER TABLE sources DROP COLUMN IF EXISTS owner_household_id;

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_scope_type_chk;
ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_scope_exclusive_chk;
ALTER TABLE assets DROP COLUMN IF EXISTS scope_type;
ALTER TABLE assets DROP COLUMN IF EXISTS owner_household_id;

ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_scope_type_chk;
ALTER TABLE documents DROP CONSTRAINT IF EXISTS documents_scope_exclusive_chk;
ALTER TABLE documents DROP COLUMN IF EXISTS scope_type;
ALTER TABLE documents DROP COLUMN IF EXISTS owner_household_id;

DROP INDEX IF EXISTS idx_households_tenant;
DROP INDEX IF EXISTS idx_household_members_user;
DROP INDEX IF EXISTS idx_sources_owner_household;
DROP INDEX IF EXISTS idx_assets_owner_household;
DROP INDEX IF EXISTS idx_documents_owner_household;

DROP TABLE IF EXISTS household_members;
DROP TABLE IF EXISTS households;
