-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS payment_intents (
        id UUID PRIMARY KEY,
        idempotency_key VARCHAR(255) UNIQUE NOT NULL,
        payer_id UUID NOT NULL REFERENCES agents(id),
        payee_id UUID NOT NULL REFERENCES agents(id),
        amount DECIMAL(78,0) NOT NULL CHECK ( amount > 0 ),
        asset VARCHAR(32) NOT NULL,
        status VARCHAR(32) NOT NULL CHECK ( status IN ('pending', 'batched', 'settled', 'failed') ),
        netting_batch_id UUID REFERENCES netting_batches(id),
        failure_reason TEXT,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        settled_at TIMESTAMPTZ
    );

    CREATE INDEX IF NOT EXISTS idx_payment_intents_status ON payment_intents (status, created_at);
    CREATE INDEX IF NOT EXISTS idx_payment_intents_batch ON payment_intents (netting_batch_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP INDEX IF EXISTS idx_payment_intents_batch;
    DROP INDEX IF EXISTS idx_payment_intents_status;
    DROP TABLE IF EXISTS payment_intents;
-- +goose StatementEnd
