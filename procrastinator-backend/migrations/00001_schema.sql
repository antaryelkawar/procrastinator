-- +goose Up
-- Fresh uniform-jsonb schema (change: asset-management-v2, design D1+D2).
--
-- This replaces the old 00001_identity .. 00006_asset_rework chain. The database
-- is WIP and contains no data to preserve, so the whole chain is collapsed into
-- this single fresh file. Every entity table now carries the uniform physical
-- shape: key/FK/RLS/lifecycle columns + created_at + updated_at + payload jsonb.
-- All queryable data besides keys/audit/lifecycle lives in payload.data.*.
--
-- pg_trgm is enabled for the ILIKE/trigram search indexes (D2). It is NOT
-- dropped in Down (an extension is a cluster-level object; dropping it here
-- would be lossy for other databases and is not part of a schema rollback).

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ---------------------------------------------------------------------------
-- Identity substrate (old 00001_identity). `users` is the user registry (no
-- RLS: the API middleware queries it unbound). `households` +
-- `household_members` carry the household-sharing model; RLS on them is added
-- in the RLS section below.
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);

-- Explicit provisioning of dev/test users (never implicit from a request).
INSERT INTO users (id) VALUES ('test-user'), ('test-user-b');

CREATE TABLE households (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    -- The household's owning user (the creator).
    owner_id text NOT NULL REFERENCES users (id),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_households_owner ON households (owner_id);

-- Many-to-many membership: a user may belong to multiple households and a
-- household may have multiple users; membership is unique per (household, user).
-- Pure association table — exempt from the payload shape (the real FK pair is
-- what the at-least-one-membership predicate in scope_access.go keys on).
CREATE TABLE household_members (
    household_id text NOT NULL REFERENCES households (id) ON DELETE CASCADE,
    -- Members are users from the registry.
    user_id text NOT NULL REFERENCES users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);
CREATE INDEX idx_household_members_user ON household_members (user_id);

-- ---------------------------------------------------------------------------
-- Core owned tables (old 00002_core_owned). Every row carries owner_id
-- (NOT NULL, FK -> users.id) and a nullable owner_household_id (FK ->
-- households.id). There is NO scope_type discriminator: a personal row has
-- owner_household_id IS NULL; a household row names the owning household.
-- All identity data moved into payload.data.*; lookups are backed by the
-- jsonb expression / trigram indexes below (design D2).
-- ---------------------------------------------------------------------------
CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_sources_owner_household ON sources (owner_household_id) WHERE owner_household_id IS NOT NULL;
-- Content-hash dedupe: per-scope lookup on payload.data.sha256 (no global
-- uniqueness — a hash collision is a dupe only within an owner scope).
CREATE INDEX idx_sources_owner_sha256 ON sources (owner_id, (payload #>> '{data,sha256}'));
-- ILIKE filename search (documents-section search-by-filename): a btree
-- expression cannot serve a leading-wildcard ILIKE, so use a pg_trgm GIN index.
CREATE INDEX idx_sources_data_filename_trgm ON sources USING gin ((payload #>> '{data,filename}') gin_trgm_ops);

CREATE TABLE assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    -- deleted_at stays a real column (design D1): it backs the soft-delete
    -- `IS NULL` filter AND hosts the WHERE predicate of the partial unique
    -- norm-serial index below, keeping RLS/tenancy tests uniform.
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_assets_owner_household ON assets (owner_household_id) WHERE owner_household_id IS NOT NULL;
-- Identity-resolution stage 1: exact normalized-serial match, owner/household
-- scoped. The predicate (deleted_at IS NULL AND norm_serial IS NOT NULL) is what
-- makes the index a true uniqueness guarantee for live rows — soft-deleted
-- assets and serial-less assets fall out of the constraint.
--
-- DEVIATION from the literal spec text: the spec lists owner_household_id as a
-- raw btree key column, but Postgres treats rows where ALL key columns are NULL
-- as distinct — so two personal (NULL-household) assets with the same
-- owner+norm_serial would NOT collide, silently breaking the uniqueness
-- guarantee for the common personal-asset case. COALESCE(owner_household_id,
-- '') maps the NULL household to '' so personal assets share a single
-- household key and the constraint actually fires. Household-scoped rows are
-- unaffected (non-NULL values pass through COALESCE unchanged), and '' cannot
-- collide with a real household id (the id CHECK forbids empty strings).
CREATE UNIQUE INDEX uniq_assets_owner_norm_serial ON assets (owner_id, COALESCE(owner_household_id, ''), (payload #>> '{data,norm_serial}')) WHERE deleted_at IS NULL AND (payload #>> '{data,norm_serial}') IS NOT NULL;
-- Stage-1 lookup index for the resolver's actual query shape (owner_id =,
-- owner_household_id IS NULL / = hh, norm_serial expr =). The unique index
-- above cannot serve it: the planner does not rewrite owner_household_id
-- predicates to the COALESCE expression, so the equality lookup gets its own
-- (non-unique) composite expression index — the same shape the old column
-- index had.
CREATE INDEX idx_assets_owner_norm_serial ON assets (owner_id, owner_household_id, (payload #>> '{data,norm_serial}')) WHERE (payload #>> '{data,norm_serial}') IS NOT NULL;
-- Identity-resolution stage 2 (brand+model) and stage 3 (name+model), each
-- scoped to the owner and optional household.
CREATE INDEX idx_assets_owner_norm_brand_model ON assets (owner_id, owner_household_id, (payload #>> '{data,norm_brand}'), (payload #>> '{data,norm_model}'));
CREATE INDEX idx_assets_owner_norm_name_model ON assets (owner_id, owner_household_id, (payload #>> '{data,norm_name}'), (payload #>> '{data,norm_model}'));
-- ILIKE search over the four identity text fields (landing search / list
-- filters): pg_trgm GIN expression indexes back the leading-wildcard ILIKE.
CREATE INDEX idx_assets_data_name_trgm ON assets USING gin ((payload #>> '{data,name}') gin_trgm_ops);
CREATE INDEX idx_assets_data_brand_trgm ON assets USING gin ((payload #>> '{data,brand}') gin_trgm_ops);
CREATE INDEX idx_assets_data_model_trgm ON assets USING gin ((payload #>> '{data,model}') gin_trgm_ops);
CREATE INDEX idx_assets_data_serial_trgm ON assets USING gin ((payload #>> '{data,serial}') gin_trgm_ops);

CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    -- Nullable so a purge can detach documents from a purged asset rather than
    -- cascade-deleting them (SET NULL keeps referential integrity on purge).
    asset_id uuid REFERENCES assets(id) ON DELETE SET NULL,
    -- At most one document per source.
    source_id uuid NOT NULL UNIQUE REFERENCES sources(id),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_documents_owner_household ON documents (owner_household_id) WHERE owner_household_id IS NOT NULL;
-- Documents-section list filter: owner + status (status lives in payload).
CREATE INDEX idx_documents_owner_status ON documents (owner_id, (payload #>> '{data,status}'));

-- ---------------------------------------------------------------------------
-- Finance tables (old 00003_finance). owner_id is the identity column;
-- owner_household_id is nullable so finance rows can be household-shared.
-- ---------------------------------------------------------------------------
CREATE TABLE financial_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_fin_accounts_owner ON financial_accounts (owner_id);
CREATE INDEX idx_fin_accounts_owner_household ON financial_accounts (owner_household_id) WHERE owner_household_id IS NOT NULL;
-- ILIKE search over account name/type/institution (landing search / list
-- filters): pg_trgm GIN expression indexes.
CREATE INDEX idx_accounts_data_name_trgm ON financial_accounts USING gin ((payload #>> '{data,name}') gin_trgm_ops);
CREATE INDEX idx_accounts_data_type_trgm ON financial_accounts USING gin ((payload #>> '{data,account_type}') gin_trgm_ops);
CREATE INDEX idx_accounts_data_institution_trgm ON financial_accounts USING gin ((payload #>> '{data,institution}') gin_trgm_ops);

CREATE TABLE import_batches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    account_id uuid NOT NULL REFERENCES financial_accounts(id),
    source_id uuid NOT NULL REFERENCES sources(id),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_import_batches_owner ON import_batches (owner_id);
CREATE INDEX idx_import_batches_owner_household ON import_batches (owner_household_id) WHERE owner_household_id IS NOT NULL;
-- Batch list filter: owner + state (state lives in payload).
CREATE INDEX idx_import_batches_owner_state ON import_batches (owner_id, (payload #>> '{data,state}'));

CREATE TABLE money_movements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    source_account_id uuid REFERENCES financial_accounts(id),
    destination_account_id uuid REFERENCES financial_accounts(id),
    -- import provenance (NULL for manual)
    import_batch_id uuid REFERENCES import_batches(id),
    -- document link (at most one movement per document): NO FK column to
    -- documents (matches the old schema) — uniqueness is enforced by the
    -- partial unique index below, not referential integrity.
    linked_document_id uuid,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX idx_movements_owner ON money_movements (owner_id);
CREATE INDEX idx_movements_src_acct ON money_movements (owner_id, source_account_id) WHERE source_account_id IS NOT NULL;
CREATE INDEX idx_movements_dst_acct ON money_movements (owner_id, destination_account_id) WHERE destination_account_id IS NOT NULL;
CREATE INDEX idx_movements_owner_household ON money_movements (owner_household_id) WHERE owner_household_id IS NOT NULL;
-- At most one movement per document (owner-scoped; linked_document_id has no FK).
CREATE UNIQUE INDEX uq_movement_linked_doc ON money_movements (owner_id, linked_document_id) WHERE linked_document_id IS NOT NULL;
-- ILIKE search over description / external_reference: pg_trgm GIN indexes.
CREATE INDEX idx_movements_data_description_trgm ON money_movements USING gin ((payload #>> '{data,description}') gin_trgm_ops);
CREATE INDEX idx_movements_data_reference_trgm ON money_movements USING gin ((payload #>> '{data,external_reference}') gin_trgm_ops);

CREATE TABLE import_lines (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    import_batch_id uuid NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    line_ref int NOT NULL,
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}',
    CONSTRAINT uq_line_ref UNIQUE (import_batch_id, line_ref)
);
CREATE INDEX idx_import_lines_owner ON import_lines (owner_id);
CREATE INDEX idx_import_lines_batch ON import_lines (owner_id, import_batch_id, line_ref);
CREATE INDEX idx_import_lines_owner_household ON import_lines (owner_household_id) WHERE owner_household_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Ingest review queue (old 00005_confidence_reviews_search). Human-in-the-loop
-- review queue for extracted documents; owned like the other owned tables.
-- ---------------------------------------------------------------------------
CREATE TABLE ingest_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    source_id uuid NOT NULL REFERENCES sources (id),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    payload jsonb NOT NULL DEFAULT '{}'
);
-- Composite index for the list endpoint: owner + state (state in payload).
CREATE INDEX idx_ingest_reviews_owner_state ON ingest_reviews (owner_id, (payload #>> '{data,state}'));
CREATE INDEX idx_ingest_reviews_owner_household ON ingest_reviews (owner_household_id) WHERE owner_household_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Owner-model RLS backstop + membership trigger (old 00004 + ingest_reviews
-- part of 00005).
--
-- Visibility rule (identical at app level, RLS, and the membership trigger):
--   visible(row, me) := owner_id = me
--                     OR (owner_household_id IS NOT NULL AND owner_household_id IN households(me))
-- where households(me) = the households of which `me` is a member.
--
-- fn_visible_households() returns households(me). It is created UNQUALIFIED,
-- i.e. in the schema resolved by search_path (`public` in the app deployment,
-- a per-test schema in the suite), so each schema owns its own copy and no
-- shared `public` object is mutated. It runs as the BYPASSRLS role rls_bypass
-- (SECURITY DEFINER) so its internal scan of household_members does not
-- re-fire the household_members policy (a raw subquery there = infinite
-- recursion). rls_bypass is NOLOGIN with exactly one member (the app role),
-- so the exception is bounded.
-- ---------------------------------------------------------------------------
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION fn_visible_households() RETURNS SETOF text
LANGUAGE plpgsql STABLE SECURITY DEFINER AS $$
BEGIN
    RETURN QUERY
        SELECT household_id FROM household_members
        WHERE user_id = current_setting('app.user_id', true);
END;
$$;
-- +goose StatementEnd

-- Run the helper as the dedicated NOLOGIN BYPASSRLS role (SECURITY DEFINER) so
-- its scan of household_members does not re-fire the household_members policy
-- (a raw subquery there = infinite recursion). SET ROLE inside a policy
-- subquery is rejected (it perturbs the outer table's RLS check), so a
-- SECURITY DEFINER function owned by a BYPASSRLS role is the recursion-free
-- form.
--
-- The function is created UNQUALIFIED, i.e. in the schema resolved by
-- search_path: `public` in the app deployment, a per-test schema in the suite.
-- It must be owned by rls_bypass (BYPASSRLS) for the recursion-free scan, and
-- to own an object in a schema the new owner needs CREATE on it. So grant
-- rls_bypass USAGE + CREATE on the schema that holds household_members (the
-- same schema the function and its table live in). to_regclass() resolves the
-- table via search_path, so each schema is granted independently and NO
-- shared `public` object is created or mutated (this is what removes the
-- parallel-migration "tuple concurrently updated" race).
GRANT SELECT ON household_members TO rls_bypass;
GRANT SELECT ON households TO rls_bypass;
-- +goose StatementBegin
DO $$
DECLARE
    cur text;
BEGIN
    SELECT n.nspname INTO cur
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE c.oid = to_regclass('household_members');
    IF cur IS NOT NULL THEN
        EXECUTE format('GRANT USAGE, CREATE ON SCHEMA %I TO rls_bypass', cur);
    END IF;
END;
$$;
-- +goose StatementEnd
ALTER FUNCTION fn_visible_households() OWNER TO rls_bypass;

-- Owned tables: identical visibility policy (USING = WITH CHECK).
ALTER TABLE sources ENABLE ROW LEVEL SECURITY;
ALTER TABLE sources FORCE ROW LEVEL SECURITY;
CREATE POLICY sources_owner_isolation ON sources
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE assets FORCE ROW LEVEL SECURITY;
CREATE POLICY assets_owner_isolation ON assets
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE documents FORCE ROW LEVEL SECURITY;
CREATE POLICY documents_owner_isolation ON documents
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE financial_accounts ENABLE ROW LEVEL SECURITY;
ALTER TABLE financial_accounts FORCE ROW LEVEL SECURITY;
CREATE POLICY financial_accounts_owner_isolation ON financial_accounts
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE money_movements ENABLE ROW LEVEL SECURITY;
ALTER TABLE money_movements FORCE ROW LEVEL SECURITY;
CREATE POLICY money_movements_owner_isolation ON money_movements
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE import_batches ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_batches FORCE ROW LEVEL SECURITY;
CREATE POLICY import_batches_owner_isolation ON import_batches
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE import_lines ENABLE ROW LEVEL SECURITY;
ALTER TABLE import_lines FORCE ROW LEVEL SECURITY;
CREATE POLICY import_lines_owner_isolation ON import_lines
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

ALTER TABLE ingest_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingest_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY ingest_reviews_owner_isolation ON ingest_reviews
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

-- households: visible to the owner AND to every member of the household.
ALTER TABLE households ENABLE ROW LEVEL SECURITY;
ALTER TABLE households FORCE ROW LEVEL SECURITY;
CREATE POLICY households_owner_isolation ON households
    USING (owner_id = current_setting('app.user_id', true)
        OR id IN (SELECT fn_visible_households()))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR id IN (SELECT fn_visible_households()));

-- household_members: visible to the member (their own rows) AND to members of
-- that household (full co-member list) AND to the household's owner (who
-- manages membership: adding a member must not require the owner to first be
-- a member of their own household). The owner disjunct reads households, whose
-- policy calls fn_visible_households(); that helper scans household_members as
-- the BYPASSRLS role, so the members<->households cycle does not recurse.
ALTER TABLE household_members ENABLE ROW LEVEL SECURITY;
ALTER TABLE household_members FORCE ROW LEVEL SECURITY;
CREATE POLICY household_members_isolation ON household_members
    USING (user_id = current_setting('app.user_id', true)
        OR household_id IN (SELECT fn_visible_households())
        OR household_id IN (SELECT id FROM households WHERE owner_id = current_setting('app.user_id', true)))
    WITH CHECK (user_id = current_setting('app.user_id', true)
        OR household_id IN (SELECT fn_visible_households())
        OR household_id IN (SELECT id FROM households WHERE owner_id = current_setting('app.user_id', true)));

-- Membership trigger: the bound user must be a member of any household they
-- attach a row to. A pure CHECK cannot reference another table, so this is a
-- BEFORE INSERT OR UPDATE trigger (DB-enforced, matching the RLS/FK
-- defense-in-depth). goose does not parse plpgsql $$ bodies, so the function
-- is wrapped in StatementBegin/End.
-- +goose StatementBegin
CREATE FUNCTION enforce_household_membership() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER AS $$
BEGIN
    IF NEW.owner_household_id IS NOT NULL THEN
        IF NOT EXISTS (
            SELECT 1 FROM household_members
            WHERE household_id = NEW.owner_household_id
              AND user_id = current_setting('app.user_id', true)
        ) AND NOT EXISTS (
            SELECT 1 FROM households
            WHERE id = NEW.owner_household_id
              AND owner_id = current_setting('app.user_id', true)
        ) THEN
            RAISE EXCEPTION 'user is not a member of household %', NEW.owner_household_id
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
ALTER FUNCTION enforce_household_membership() OWNER TO rls_bypass;

CREATE TRIGGER sources_household_membership
    BEFORE INSERT OR UPDATE ON sources
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER assets_household_membership
    BEFORE INSERT OR UPDATE ON assets
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER documents_household_membership
    BEFORE INSERT OR UPDATE ON documents
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER financial_accounts_household_membership
    BEFORE INSERT OR UPDATE ON financial_accounts
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER money_movements_household_membership
    BEFORE INSERT OR UPDATE ON money_movements
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER import_batches_household_membership
    BEFORE INSERT OR UPDATE ON import_batches
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER import_lines_household_membership
    BEFORE INSERT OR UPDATE ON import_lines
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();
CREATE TRIGGER ingest_reviews_household_membership
    BEFORE INSERT OR UPDATE ON ingest_reviews
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();

-- +goose Down
-- Reverse in order: triggers, RLS (policies + NO FORCE/DISABLE), functions,
-- grants/revokes, then tables in reverse dependency order. The pg_trgm
-- extension is intentionally NOT dropped (cluster-level object, not a schema
-- rollback target).
DROP TRIGGER IF EXISTS sources_household_membership ON sources;
DROP TRIGGER IF EXISTS assets_household_membership ON assets;
DROP TRIGGER IF EXISTS documents_household_membership ON documents;
DROP TRIGGER IF EXISTS financial_accounts_household_membership ON financial_accounts;
DROP TRIGGER IF EXISTS money_movements_household_membership ON money_movements;
DROP TRIGGER IF EXISTS import_batches_household_membership ON import_batches;
DROP TRIGGER IF EXISTS import_lines_household_membership ON import_lines;
DROP TRIGGER IF EXISTS ingest_reviews_household_membership ON ingest_reviews;
DROP FUNCTION IF EXISTS enforce_household_membership();

DROP POLICY IF EXISTS sources_owner_isolation ON sources;
ALTER TABLE sources NO FORCE ROW LEVEL SECURITY;
ALTER TABLE sources DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS assets_owner_isolation ON assets;
ALTER TABLE assets NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assets DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS documents_owner_isolation ON documents;
ALTER TABLE documents NO FORCE ROW LEVEL SECURITY;
ALTER TABLE documents DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS financial_accounts_owner_isolation ON financial_accounts;
ALTER TABLE financial_accounts NO FORCE ROW LEVEL SECURITY;
ALTER TABLE financial_accounts DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS money_movements_owner_isolation ON money_movements;
ALTER TABLE money_movements NO FORCE ROW LEVEL SECURITY;
ALTER TABLE money_movements DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS import_batches_owner_isolation ON import_batches;
ALTER TABLE import_batches NO FORCE ROW LEVEL SECURITY;
ALTER TABLE import_batches DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS import_lines_owner_isolation ON import_lines;
ALTER TABLE import_lines NO FORCE ROW LEVEL SECURITY;
ALTER TABLE import_lines DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ingest_reviews_owner_isolation ON ingest_reviews;
ALTER TABLE ingest_reviews NO FORCE ROW LEVEL SECURITY;
ALTER TABLE ingest_reviews DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS households_owner_isolation ON households;
ALTER TABLE households NO FORCE ROW LEVEL SECURITY;
ALTER TABLE households DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS household_members_isolation ON household_members;
ALTER TABLE household_members NO FORCE ROW LEVEL SECURITY;
ALTER TABLE household_members DISABLE ROW LEVEL SECURITY;

REVOKE SELECT ON household_members FROM rls_bypass;
REVOKE SELECT ON households FROM rls_bypass;
-- +goose StatementBegin
DO $$
DECLARE
    cur text;
BEGIN
    SELECT n.nspname INTO cur
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      WHERE c.oid = to_regclass('household_members');
    IF cur IS NOT NULL THEN
        EXECUTE format('REVOKE USAGE, CREATE ON SCHEMA %I FROM rls_bypass', cur);
    END IF;
END;
$$;
-- +goose StatementEnd
DROP FUNCTION IF EXISTS fn_visible_households();

DROP TABLE IF EXISTS import_lines;
-- money_movements before import_batches: money_movements.import_batch_id is a
-- real FK (unlike the old chain), so the dependent table drops first.
DROP TABLE IF EXISTS money_movements;
DROP TABLE IF EXISTS import_batches;
DROP TABLE IF EXISTS financial_accounts;
DROP TABLE IF EXISTS ingest_reviews;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS sources;
DROP TABLE IF EXISTS household_members;
DROP TABLE IF EXISTS households;
DROP TABLE IF EXISTS users;
