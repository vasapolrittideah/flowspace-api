-- +goose Up
ALTER TABLE workspace_creations
    RENAME COLUMN created_at TO completed_at;

ALTER TABLE workspace_creations
    ADD COLUMN expires_at timestamptz;

UPDATE workspace_creations
SET expires_at = completed_at + INTERVAL '24 hours';

ALTER TABLE workspace_creations
    ALTER COLUMN expires_at SET DEFAULT (now() + INTERVAL '24 hours'),
    ALTER COLUMN expires_at SET NOT NULL,
    ADD CONSTRAINT workspace_creations_expiry_window
        CHECK (expires_at = completed_at + INTERVAL '24 hours');

-- +goose Down
ALTER TABLE workspace_creations
    DROP CONSTRAINT workspace_creations_expiry_window,
    DROP COLUMN expires_at;

ALTER TABLE workspace_creations
    RENAME COLUMN completed_at TO created_at;
