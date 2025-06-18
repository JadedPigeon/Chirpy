-- +goose Up
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
ALTER TABLE users
ALTER COLUMN id SET DEFAULT gen_random_uuid();

-- +goose Down
ALTER TABLE users
ALTER COLUMN id DROP DEFAULT;