-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS netting_batches (
        id UUID PRIMARY KEY,
        batch_status VARCHAR(255) NOT NULL CHECK (batch_status IN ('open', 'processing', 'settled')),
        settlement_tx_hash VARCHAR(255),
        total_saved_gases DECIMAL(78,0) NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS netting_batches;
-- +goose StatementEnd