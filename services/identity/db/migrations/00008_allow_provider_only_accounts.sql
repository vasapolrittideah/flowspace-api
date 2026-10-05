-- +goose Up
-- Identity code from before this migration cannot read a NULL password_hash.
-- For an account without a password, that code returns errors on password
-- login, password recovery, and session refresh. No shared environment ran
-- that code, so no rolling deployment mixes it with such accounts. Do not
-- deploy that code against this schema.
ALTER TABLE identity_accounts ALTER COLUMN password_hash DROP NOT NULL;

-- +goose Down
-- This fails while provider-only accounts exist, so a rollback never deletes them.
ALTER TABLE identity_accounts ALTER COLUMN password_hash SET NOT NULL;
