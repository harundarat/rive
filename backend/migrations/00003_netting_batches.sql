-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS netting_batches (
        id UUID PRIMARY KEY,
        batch_status VARCHAR(255) NOT NULL CHECK (batch_status IN ('open', 'processing', 'settled', 'failed')),
        settlement_tx_hash VARCHAR(255),
        total_gas_saved DECIMAL(78,0) NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        window_start TIMESTAMPTZ,
        window_end TIMESTAMPTZ
    );

    CREATE INDEX IF NOT EXISTS idx_netting_batches_status ON netting_batches (batch_status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS netting_batches;
-- +goose StatementEnd