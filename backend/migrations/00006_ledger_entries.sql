-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS ledger_entries (
        id UUID PRIMARY KEY,
        account_id UUID NOT NULL REFERENCES accounts(id),
        journal_entry_id UUID NOT NULL REFERENCES journal_entries(id),
        amount DECIMAL(78,0) NOT NULL,
        entry_type VARCHAR(255) CHECK (entry_type IN ('debit', 'credit')) NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS ledger_entries;
-- +goose StatementEnd