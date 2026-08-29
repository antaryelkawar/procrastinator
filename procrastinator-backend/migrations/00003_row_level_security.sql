-- RLS defense-in-depth on tenant-owned tables (design D4).
-- app.tenant_id is a transaction-scoped GUC set by the application; unset, it is NULL.
-- +goose Up
ALTER TABLE sources ENABLE ROW LEVEL SECURITY;
-- FORCE applies the policies to the table owner as well
ALTER TABLE sources FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS sources_tenant_isolation ON sources;
CREATE POLICY sources_tenant_isolation ON sources USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
ALTER TABLE assets ENABLE ROW LEVEL SECURITY;
-- FORCE applies the policies to the table owner as well
ALTER TABLE assets FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS assets_tenant_isolation ON assets;
CREATE POLICY assets_tenant_isolation ON assets USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;
-- FORCE applies the policies to the table owner as well
ALTER TABLE documents FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS documents_tenant_isolation ON documents;
CREATE POLICY documents_tenant_isolation ON documents USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true));

-- +goose Down
DROP POLICY IF EXISTS sources_tenant_isolation ON sources;
ALTER TABLE sources NO FORCE ROW LEVEL SECURITY;
ALTER TABLE sources DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS assets_tenant_isolation ON assets;
ALTER TABLE assets NO FORCE ROW LEVEL SECURITY;
ALTER TABLE assets DISABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS documents_tenant_isolation ON documents;
ALTER TABLE documents NO FORCE ROW LEVEL SECURITY;
ALTER TABLE documents DISABLE ROW LEVEL SECURITY;
