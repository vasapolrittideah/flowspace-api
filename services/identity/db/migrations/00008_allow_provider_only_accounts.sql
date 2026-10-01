-- +goose Up
ALTER TABLE identity_accounts ALTER COLUMN password_hash DROP NOT NULL;

-- +goose Down
-- This fails while provider-only accounts exist, so a rollback never deletes them.
ALTER TABLE identity_accounts ALTER COLUMN password_hash SET NOT NULL;
