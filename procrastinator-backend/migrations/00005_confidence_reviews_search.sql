-- +goose Up
-- P2: confidence columns on owned tables + ingest_reviews (change: proper-multi-tenant-search).
--
-- Adds a nullable confidence column to assets and documents, and creates the
-- ingest_reviews table: the human-in-the-loop review queue for extracted
-- documents. Each review row is owned (owner_id + nullable owner_household_id)
-- and follows the same RLS / membership-trigger pattern as the other owned
-- tables introduced in 00002/00004.

ALTER TABLE assets ADD COLUMN confidence real;
ALTER TABLE documents ADD COLUMN confidence real;

CREATE TABLE ingest_reviews (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    owner_household_id text REFERENCES households (id),
    source_id uuid NOT NULL REFERENCES sources (id),
    doc_type text NOT NULL CHECK (doc_type IN ('invoice','warranty','amc','other')),
    candidate_fields jsonb NOT NULL,
    raw_extraction jsonb NOT NULL DEFAULT '{}',
    confidence real,
    best_matched_asset_id uuid REFERENCES assets (id) ON DELETE SET NULL,
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','approved','rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    decided_at timestamptz,
    decided_by text REFERENCES users (id)
);

-- Composite index for the list endpoint (filter by owner + state, order by created_at).
CREATE INDEX idx_ingest_reviews_owner_state_created ON ingest_reviews (owner_id, state, created_at);

-- Partial index for household scoping (matches the pattern on other owned tables).
CREATE INDEX idx_ingest_reviews_owner_household ON ingest_reviews (owner_household_id) WHERE owner_household_id IS NOT NULL;

-- RLS: identical visibility policy to the other owned tables (USING = WITH CHECK).
ALTER TABLE ingest_reviews ENABLE ROW LEVEL SECURITY;
ALTER TABLE ingest_reviews FORCE ROW LEVEL SECURITY;
CREATE POLICY ingest_reviews_owner_isolation ON ingest_reviews
    USING (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())))
    WITH CHECK (owner_id = current_setting('app.user_id', true)
        OR (owner_household_id IS NOT NULL AND owner_household_id IN (SELECT fn_visible_households())));

-- Membership trigger: the bound user must be a member of any household they
-- attach a row to (DB-enforced, matching the RLS/FK defense-in-depth).
CREATE TRIGGER ingest_reviews_household_membership
    BEFORE INSERT OR UPDATE ON ingest_reviews
    FOR EACH ROW EXECUTE FUNCTION enforce_household_membership();

-- +goose Down
DROP TRIGGER IF EXISTS ingest_reviews_household_membership ON ingest_reviews;
DROP POLICY IF EXISTS ingest_reviews_owner_isolation ON ingest_reviews;
ALTER TABLE ingest_reviews NO FORCE ROW LEVEL SECURITY;
ALTER TABLE ingest_reviews DISABLE ROW LEVEL SECURITY;
DROP TABLE IF EXISTS ingest_reviews;
ALTER TABLE documents DROP COLUMN IF EXISTS confidence;
ALTER TABLE assets DROP COLUMN IF EXISTS confidence;
