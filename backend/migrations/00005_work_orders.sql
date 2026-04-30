-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS work_orders (
        id UUID PRIMARY KEY,
        idempotency_key VARCHAR(255) UNIQUE NOT NULL,
        creator_id UUID REFERENCES agents(id) NOT NULL,
        provider_id UUID REFERENCES agents(id) NOT NULL,
        amount DECIMAL(78,0) NOT NULL CHECK (amount > 0),
        status VARCHAR(255) NOT NULL CHECK (status IN ('draft', 'funded', 'completed', 'refunded')),
        spec_hash CHAR(66) UNIQUE NOT NULL CHECK (spec_hash ~ '^0x[0-9a-fA-F]{64}$'),
        spec_version VARCHAR(32) NOT NULL,
        spec_tx_hash CHAR(66) NOT NULL CHECK (spec_tx_hash ~ '^0x[0-9a-fA-F]{64}$'),
        deliverable_cid CHAR(66) CHECK (deliverable_cid ~ '^0x[0-9a-fA-F]{64}$'),
        delivered_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        refunded_at TIMESTAMPTZ,
        onchain_order_id NUMERIC(78,0) UNIQUE,
        order_tx_hash CHAR(66) CHECK (order_tx_hash ~ '^0x[0-9a-fA-F]{64}$'),
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS work_orders;
-- +goose StatementEnd
