-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS journal_entries (
        id UUID PRIMARY KEY,
        idempotency_key VARCHAR(255) UNIQUE NOT NULL,
        work_order_id UUID REFERENCES work_orders(id) ON DELETE RESTRICT,
        payment_intent_id UUID REFERENCES payment_intents(id) ON DELETE RESTRICT,
        description TEXT NOT NULL,
        storage_cid VARCHAR(255),
        netting_batch_id UUID REFERENCES netting_batches(id) ON DELETE RESTRICT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS journal_entries;
-- +goose StatementEnd
