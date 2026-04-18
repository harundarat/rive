-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS accounts (
        id UUID PRIMARY KEY,
        agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
        name VARCHAR(255) NOT NULL,
        type VARCHAR(255) NOT NULL CHECK (type IN ('asset', 'liability', 'revenue', 'expense', 'equity')),
        balance DECIMAL(78, 0) NOT NULL DEFAULT 0,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
        version INT NOT NULL DEFAULT 1
    );

    CREATE UNIQUE INDEX idx_agent_name ON accounts (agent_id, name);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP INDEX IF EXISTS idx_agent_name;
    DROP TABLE IF EXISTS accounts;
-- +goose StatementEnd