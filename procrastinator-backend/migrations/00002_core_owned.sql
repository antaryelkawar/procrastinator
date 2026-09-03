-- +goose Up
-- Core owned tables (change: owner-model-rework). Every row carries owner_id
-- (NOT NULL, FK -> users.id) and a nullable owner_household_id (FK ->
-- households.id). There is NO scope_type discriminator: a personal row has
-- owner_household_id IS NULL; a household row names the owning household.

CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    filename text NOT NULL,
    content_type text NOT NULL,
    byte_size bigint NOT NULL,
    storage_path text NOT NULL,
    sha256 text NOT NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now(),
    owner_household_id text REFERENCES households (id)
);
CREATE INDEX idx_sources_owner_household ON sources (owner_household_id) WHERE owner_household_id IS NOT NULL;

CREATE TABLE assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    brand text,
    model text,
    serial_number text,
    norm_serial text,
    norm_brand text,
    norm_model text,
    purchase_date date,
    warranty_end date,
    price numeric,
    currency char(3),
    doc_type text NOT NULL DEFAULT 'other' CHECK (doc_type IN ('invoice','warranty','amc','other')),
    metadata jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    owner_household_id text REFERENCES households (id)
);
CREATE UNIQUE INDEX uniq_assets_owner_norm_serial ON assets (owner_id, owner_household_id, norm_serial) WHERE norm_serial IS NOT NULL;
CREATE INDEX idx_assets_owner_household ON assets (owner_household_id) WHERE owner_household_id IS NOT NULL;

CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id text NOT NULL REFERENCES users (id),
    asset_id uuid NOT NULL REFERENCES assets(id),
    source_id uuid NOT NULL UNIQUE REFERENCES sources(id),
    doc_type text NOT NULL CHECK (doc_type IN ('invoice','warranty','amc','other')),
    extracted_fields jsonb NOT NULL,
    raw_extraction jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    owner_household_id text REFERENCES households (id)
);
CREATE INDEX idx_documents_owner_household ON documents (owner_household_id) WHERE owner_household_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS sources;
