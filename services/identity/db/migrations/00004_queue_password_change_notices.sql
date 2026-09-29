-- +goose Up
CREATE TABLE identity_password_change_notices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_subject text NOT NULL REFERENCES identity_accounts (subject),
    email_local text NOT NULL CHECK (email_local <> ''),
    email_domain text NOT NULL CHECK (email_domain <> '' AND email_domain = lower(email_domain)),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    next_attempt_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    claim_owner text,
    claimed_until timestamptz,
    delivered_at timestamptz,
    CHECK ((claim_owner IS NULL) = (claimed_until IS NULL))
);

-- +goose Down
DROP TABLE identity_password_change_notices;
