-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS work_orders (
        id UUID PRIMARY KEY,
        idempotency_key VARCHAR(255) UNIQUE NOT NULL,
        creator_id UUID REFERENCES agents(id) NOT NULL,
        provider_id UUID REFERENCES agents(id) NOT NULL,
        amount DECIMAL(78,0) NOT NULL,
        status VARCHAR(255) NOT NULL CHECK (status IN ('draft', 'locked', 'completed', 'disputed', 'refunded')),
        criteria_cid VARCHAR(255) UNIQUE NOT NULL,
        deliverable_cid VARCHAR(255) UNIQUE,
        netting_batch_id UUID REFERENCES netting_batches(id),
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP

    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS work_orders;
-- +goose StatementEnd