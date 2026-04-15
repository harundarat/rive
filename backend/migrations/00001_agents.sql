-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS agents (
        id UUID PRIMARY KEY,
        0g_agent_id VARCHAR(255) UNIQUE NOT NULL,
        wallet_address VARCHAR(255) UNIQUE NOT NULL,
        reputation_score FLOAT NOT NULL,
        metadata_cid VARCHAR(255) NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS agents;
-- +goose StatementEnd
