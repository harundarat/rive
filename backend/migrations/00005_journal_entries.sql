-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS journal_entries (
        id UUID PRIMARY KEY,
        idempotency_key VARCHAR(255) NOT NULL,
        work_order_id UUID NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
        description TEXT NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        storage_cid VARCHAR(255)
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS journal_entries;
-- +goose StatementEnd