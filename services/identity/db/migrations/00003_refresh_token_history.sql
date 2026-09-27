-- +goose Up
CREATE TABLE identity_rotated_refresh_tokens (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    session_id uuid NOT NULL REFERENCES identity_sessions (id) ON DELETE CASCADE
);

-- +goose Down
DROP TABLE identity_rotated_refresh_tokens;
