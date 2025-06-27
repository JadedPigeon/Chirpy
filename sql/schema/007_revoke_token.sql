-- +goose Up
ALTER TABLE refresh_tokens
    RENAME COLUMN token TO refresh_token;

-- +goose Down
ALTER TABLE refresh_tokens
    RENAME COLUMN refresh_token TO token;