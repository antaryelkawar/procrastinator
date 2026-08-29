-- +goose Up
CREATE TABLE tenants (
    id text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9_-]{1,64}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Backfill: every distinct pre-existing tenant_id becomes a registry row,
-- before FK enforcement.
INSERT INTO tenants (id)
    SELECT DISTINCT tenant_id FROM sources
    UNION
    SELECT DISTINCT tenant_id FROM assets
    UNION
    SELECT DISTINCT tenant_id FROM documents
    ON CONFLICT (id) DO NOTHING;

-- Explicit provisioning of dev/test tenants (never implicit from a header).
INSERT INTO tenants (id) VALUES ('test-tenant'), ('test-tenant-b')
    ON CONFLICT (id) DO NOTHING;

-- Tenant-owned rows must reference a registered tenant (default ON DELETE RESTRICT).
ALTER TABLE sources   ADD CONSTRAINT sources_tenant_id_fkey   FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE assets    ADD CONSTRAINT assets_tenant_id_fkey    FOREIGN KEY (tenant_id) REFERENCES tenants (id);
ALTER TABLE documents ADD CONSTRAINT documents_tenant_id_fkey FOREIGN KEY (tenant_id) REFERENCES tenants (id);

-- +goose Down
ALTER TABLE sources   DROP CONSTRAINT sources_tenant_id_fkey;
ALTER TABLE assets    DROP CONSTRAINT assets_tenant_id_fkey;
ALTER TABLE documents DROP CONSTRAINT documents_tenant_id_fkey;

DROP TABLE IF EXISTS tenants;
