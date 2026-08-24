-- +goose Up
CREATE TABLE sources (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    filename text NOT NULL,
    content_type text NOT NULL,
    byte_size bigint NOT NULL,
    storage_path text NOT NULL,
    sha256 text NOT NULL,
    uploaded_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    brand text,
    model text,
    serial_number text,
    norm_serial text,
    norm_brand text,
    norm_model text,
    purchase_date date,
    price numeric,
    currency char(3),
    warranty_start date,
    warranty_end date,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_assets_norm_serial ON assets (norm_serial) WHERE norm_serial IS NOT NULL;

CREATE TABLE documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id uuid NOT NULL REFERENCES assets(id),
    source_id uuid NOT NULL UNIQUE REFERENCES sources(id),
    doc_type text NOT NULL CHECK (doc_type IN ('invoice','warranty','other')),
    extracted_fields jsonb NOT NULL,
    raw_extraction jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS assets;
DROP TABLE IF EXISTS sources;
