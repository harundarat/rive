-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS payment_intents (
        id UUID PRIMARY KEY,
        payer_id UUID NOT NULL REFERENCES agents(id),
        payee_id UUID NOT NULL REFERENCES agents(id),
        amount DECIMAL(78,0) NOT NULL CHECK ( amount > 0 ),
        status VARCHAR(32) NOT NULL CHECK ( status IN ('pending', 'batched', 'settled', 'failed') ),
        netting_batch_id UUID REFERENCES netting_batches(id),
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS payment_intents;
-- +goose StatementEnd