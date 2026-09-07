-- +goose Up
-- Asset-management rework (change: asset-management-rework). The asset gains an
-- intrinsic identity (name + category) and soft-delete / merge lifecycle, while
-- the document keeps its own doc_type. The two composite indexes back the
-- reworked identity-resolution lookup: stage 2 resolves on brand+model, stage 3
-- on name+model, both scoped to the owner (and optional household).

-- Intrinsic identity + lifecycle columns on the asset. asset_category replaces
-- the old doc_type (the document keeps its own doc_type); category_user_set
-- marks a user-corrected category that the extractor must not overwrite.
ALTER TABLE assets ADD COLUMN name text;
ALTER TABLE assets ADD COLUMN norm_name text;
ALTER TABLE assets ADD COLUMN asset_category text;
ALTER TABLE assets ADD COLUMN category_confidence real;
ALTER TABLE assets ADD COLUMN category_user_set boolean NOT NULL DEFAULT false;

-- Soft-delete + merge lifecycle. merged_into points at the surviving asset
-- (SET NULL so a merge target can itself be purged without dangling FKs).
ALTER TABLE assets ADD COLUMN deleted_at timestamptz;
ALTER TABLE assets ADD COLUMN merged_into uuid REFERENCES assets(id) ON DELETE SET NULL;
ALTER TABLE assets ADD COLUMN merged_at timestamptz;

-- doc_type is replaced by the intrinsic asset_category (see above); drop it.
ALTER TABLE assets DROP COLUMN doc_type;

-- doc_type vocabulary grows: a receipt/statement is now a first-class document
-- type alongside invoice/warranty/amc. Re-apply the same named constraint with
-- the expanded 6-value set (keeping the column NOT NULL).
ALTER TABLE documents DROP CONSTRAINT documents_doc_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_doc_type_check
    CHECK (doc_type IN ('invoice','receipt','warranty','amc','statement','other'));

-- asset_id becomes nullable so a purge can detach documents from a purged asset
-- rather than cascade-deleting them.
ALTER TABLE documents ALTER COLUMN asset_id DROP NOT NULL;

-- Same 6-value doc_type expansion on the review queue (same named constraint).
ALTER TABLE ingest_reviews DROP CONSTRAINT ingest_reviews_doc_type_check;
ALTER TABLE ingest_reviews ADD CONSTRAINT ingest_reviews_doc_type_check
    CHECK (doc_type IN ('invoice','receipt','warranty','amc','statement','other'));

-- provenance stores the per-worker extraction results / the candidate set that
-- produced this review.
ALTER TABLE ingest_reviews ADD COLUMN provenance jsonb NOT NULL DEFAULT '{}';

-- Identity-resolution indexes: stage 2 (brand+model) and stage 3 (name+model),
-- each scoped to the owner and optional household.
CREATE INDEX idx_assets_owner_norm_brand_model ON assets (owner_id, owner_household_id, norm_brand, norm_model);
CREATE INDEX idx_assets_owner_norm_name_model ON assets (owner_id, owner_household_id, norm_name, norm_model);

-- +goose Down
-- Reverse every step above, in reverse order.
DROP INDEX IF EXISTS idx_assets_owner_norm_name_model;
DROP INDEX IF EXISTS idx_assets_owner_norm_brand_model;

ALTER TABLE ingest_reviews DROP COLUMN IF EXISTS provenance;

-- Restore the 4-value doc_type check on the review queue.
ALTER TABLE ingest_reviews DROP CONSTRAINT ingest_reviews_doc_type_check;
ALTER TABLE ingest_reviews ADD CONSTRAINT ingest_reviews_doc_type_check
    CHECK (doc_type IN ('invoice','warranty','amc','other'));

-- Restore the 4-value doc_type check on documents.
ALTER TABLE documents DROP CONSTRAINT documents_doc_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_doc_type_check
    CHECK (doc_type IN ('invoice','warranty','amc','other'));

-- Restore asset_id NOT NULL.
ALTER TABLE documents ALTER COLUMN asset_id SET NOT NULL;

-- Re-add assets.doc_type exactly as 00002 originally defined it.
ALTER TABLE assets ADD COLUMN doc_type text NOT NULL DEFAULT 'other';
ALTER TABLE assets ADD CONSTRAINT assets_doc_type_check
    CHECK (doc_type IN ('invoice','warranty','amc','other'));

-- Drop the new asset columns.
ALTER TABLE assets DROP COLUMN IF EXISTS merged_at;
ALTER TABLE assets DROP COLUMN IF EXISTS merged_into;
ALTER TABLE assets DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE assets DROP COLUMN IF EXISTS category_user_set;
ALTER TABLE assets DROP COLUMN IF EXISTS category_confidence;
ALTER TABLE assets DROP COLUMN IF EXISTS asset_category;
ALTER TABLE assets DROP COLUMN IF EXISTS norm_name;
ALTER TABLE assets DROP COLUMN IF EXISTS name;
