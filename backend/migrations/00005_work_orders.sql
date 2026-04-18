-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS work_orders (
        id UUID PRIMARY KEY,
        idempotency_key VARCHAR(255) UNIQUE NOT NULL,
        creator_id UUID REFERENCES agents(id) NOT NULL,
        provider_id UUID REFERENCES agents(id) NOT NULL,
        amount DECIMAL(78,0) NOT NULL CHECK (amount > 0),
        status VARCHAR(255) NOT NULL CHECK (status IN ('draft', 'locked', 'completed', 'refunded')),
        criteria_cid VARCHAR(255) NOT NULL,
        deliverable_cid VARCHAR(255),
        funded_at TIMESTAMPTZ,
        completed_at TIMESTAMPTZ,
        refunded_at TIMESTAMPTZ,
        onchain_order_id BIGINT,
        funding_tx_hash VARCHAR(66),
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP

    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS work_orders;
-- +goose StatementEnd