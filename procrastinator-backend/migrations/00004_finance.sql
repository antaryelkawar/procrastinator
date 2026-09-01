-- +goose Up
CREATE TABLE financial_accounts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         text NOT NULL,
    name              text NOT NULL,
    account_type      text NOT NULL CHECK (account_type IN ('bank','wallet','cash','credit_card')),
    currency          char(3) NOT NULL,
    institution       text,
    external_descriptor text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_fin_accounts_tenant ON financial_accounts (tenant_id);

CREATE TABLE money_movements (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              text NOT NULL,
    kind                   text NOT NULL CHECK (kind IN ('expense','income','transfer')),
    amount                 numeric NOT NULL CHECK (amount > 0),
    currency               char(3) NOT NULL,
    occurred_on            date NOT NULL,
    recorded_at            timestamptz NOT NULL DEFAULT now(),
    description            text NOT NULL,
    norm_description       text NOT NULL,
    origin                 text NOT NULL CHECK (origin IN ('manual','import')),
    source_account_id      uuid REFERENCES financial_accounts(id),
    destination_account_id uuid REFERENCES financial_accounts(id),
    -- import provenance (NULL for manual)
    import_batch_id        uuid,
    import_line            int,
    external_reference     text,
    -- document link (at most one document per movement, at most one movement per document)
    linked_document_id     uuid,
    link_creator           text CHECK (link_creator IN ('manual','auto')),
    link_conflicting       boolean NOT NULL DEFAULT false,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT chk_kind_accounts CHECK (
        (kind = 'expense'  AND source_account_id IS NOT NULL AND destination_account_id IS NULL)
        OR (kind = 'income' AND source_account_id IS NULL AND destination_account_id IS NOT NULL)
        OR (kind = 'transfer' AND source_account_id IS NOT NULL AND destination_account_id IS NOT NULL
            AND source_account_id <> destination_account_id)
    )
);
CREATE INDEX idx_movements_tenant      ON money_movements (tenant_id);
CREATE INDEX idx_movements_src_acct    ON money_movements (tenant_id, source_account_id)     WHERE source_account_id IS NOT NULL;
CREATE INDEX idx_movements_dst_acct    ON money_movements (tenant_id, destination_account_id) WHERE destination_account_id IS NOT NULL;
CREATE INDEX idx_movements_occurred    ON money_movements (tenant_id, occurred_on);
CREATE INDEX idx_movements_extref      ON money_movements (tenant_id, external_reference)    WHERE external_reference IS NOT NULL;
CREATE UNIQUE INDEX uq_movement_linked_doc ON money_movements (tenant_id, linked_document_id) WHERE linked_document_id IS NOT NULL;

CREATE TABLE import_batches (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                text NOT NULL,
    state                    text NOT NULL CHECK (state IN ('preview','committed','discarded')),
    account_id               uuid NOT NULL REFERENCES financial_accounts(id),
    source_id                uuid NOT NULL REFERENCES sources(id),
    filename                 text NOT NULL,
    format                   text NOT NULL CHECK (format IN ('csv','pdf')),
    line_count_valid         int NOT NULL DEFAULT 0,
    line_count_duplicate     int NOT NULL DEFAULT 0,
    line_count_possible_dup  int NOT NULL DEFAULT 0,
    line_count_error         int NOT NULL DEFAULT 0,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_import_batches_tenant ON import_batches (tenant_id);

CREATE TABLE import_lines (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          text NOT NULL,
    batch_id           uuid NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    line_ref           int NOT NULL,
    raw_line           text NOT NULL,
    occurred_on        date,
    amount             numeric,
    direction          text CHECK (direction IN ('in','out')),
    description        text,
    norm_description   text,
    external_reference text,
    status             text NOT NULL CHECK (status IN ('valid','duplicate','possible-duplicate','error')),
    error_reason       text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_line_ref UNIQUE (batch_id, line_ref)
);
CREATE INDEX idx_import_lines_tenant ON import_lines (tenant_id);
CREATE INDEX idx_import_lines_batch  ON import_lines (tenant_id, batch_id, line_ref);

-- +goose Down
DROP TABLE IF EXISTS import_lines;
DROP TABLE IF EXISTS import_batches;
DROP TABLE IF EXISTS money_movements;
DROP TABLE IF EXISTS financial_accounts;
