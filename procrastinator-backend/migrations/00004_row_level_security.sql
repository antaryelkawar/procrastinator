-- +goose Up
-- Owner-model RLS backstop + membership trigger (change: owner-model-rework).
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

-- +goose Down
DROP TRIGGER IF EXISTS sources_household_membership ON sources;
DROP TRIGGER IF EXISTS assets_household_membership ON assets;
DROP TRIGGER IF EXISTS documents_household_membership ON documents;
DROP TRIGGER IF EXISTS financial_accounts_household_membership ON financial_accounts;
DROP TRIGGER IF EXISTS money_movements_household_membership ON money_movements;
DROP TRIGGER IF EXISTS import_batches_household_membership ON import_batches;
DROP TRIGGER IF EXISTS import_lines_household_membership ON import_lines;
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
