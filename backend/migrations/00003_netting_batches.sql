-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS netting_batches (
        id UUID PRIMARY KEY,
        batch_status VARCHAR(255) NOT NULL CHECK (batch_status IN ('open', 'processing', 'settled', 'failed')),
        batch_hash VARCHAR(255) UNIQUE,
        manifest_tx_hash VARCHAR(255),
        settlement_tx_hash VARCHAR(255),
        failure_reason TEXT,
        gross_intent_count INT NOT NULL DEFAULT 0,
        settlement_transfer_count INT NOT NULL DEFAULT 0,
        gross_amount DECIMAL(78,0) NOT NULL DEFAULT 0,
        net_amount DECIMAL(78,0) NOT NULL DEFAULT 0,
        total_gas_saved DECIMAL(78,0) NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        window_start TIMESTAMPTZ,
        window_end TIMESTAMPTZ
    );

    CREATE INDEX IF NOT EXISTS idx_netting_batches_status ON netting_batches (batch_status);
    CREATE INDEX IF NOT EXISTS idx_netting_batches_hash ON netting_batches (batch_hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP INDEX IF EXISTS idx_netting_batches_hash;
    DROP TABLE IF EXISTS netting_batches;
-- +goose StatementEnd
