-- +goose Up
-- +goose StatementBegin
    CREATE TABLE IF NOT EXISTS storage_contents (
        id UUID PRIMARY KEY,
        content_hash VARCHAR(66) NOT NULL,
        content BYTEA NOT NULL,
        created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
    );

    CREATE INDEX IF NOT EXISTS storage_contents_content_hash_idx ON storage_contents (content_hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
    DROP TABLE IF EXISTS storage_contents;
-- +goose StatementEnd
