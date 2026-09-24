-- +goose Up
CREATE TABLE identity_accounts (
    subject text PRIMARY KEY CHECK (subject <> ''),
    email_local text NOT NULL CHECK (email_local <> ''),
    email_domain text NOT NULL CHECK (email_domain <> '' AND email_domain = lower(email_domain)),
    password_hash text NOT NULL CHECK (password_hash LIKE '$argon2id$%'),
    email_verified_at timestamptz,
    retired_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    CHECK (retired_at IS NULL OR email_verified_at IS NULL)
);

CREATE UNIQUE INDEX identity_accounts_active_email
    ON identity_accounts (email_local, email_domain)
    WHERE retired_at IS NULL;

CREATE TABLE identity_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_subject text NOT NULL REFERENCES identity_accounts (subject),
    refresh_token_hash bytea NOT NULL UNIQUE CHECK (octet_length(refresh_token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    idle_expires_at timestamptz NOT NULL,
    absolute_expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    CHECK (idle_expires_at > created_at AND idle_expires_at <= absolute_expires_at)
);

CREATE TABLE identity_challenges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_subject text NOT NULL REFERENCES identity_accounts (subject),
    purpose text NOT NULL CHECK (purpose IN ('verify-email', 'claim-account', 'password-reset')),
    email_local text NOT NULL CHECK (email_local <> ''),
    email_domain text NOT NULL CHECK (email_domain <> '' AND email_domain = lower(email_domain)),
    code_verifier bytea NOT NULL CHECK (octet_length(code_verifier) = 32),
    wrong_guesses smallint NOT NULL DEFAULT 0 CHECK (wrong_guesses BETWEEN 0 AND 5),
    issued_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    expires_at timestamptz NOT NULL,
    replaced_at timestamptz,
    consumed_at timestamptz,
    CHECK (expires_at = issued_at + INTERVAL '10 minutes'),
    CHECK (replaced_at IS NULL OR consumed_at IS NULL)
);

CREATE UNIQUE INDEX identity_challenges_one_current_purpose
    ON identity_challenges (account_subject, purpose)
    WHERE replaced_at IS NULL AND consumed_at IS NULL;

CREATE TABLE identity_challenge_deliveries (
    challenge_id uuid PRIMARY KEY REFERENCES identity_challenges (id) ON DELETE CASCADE,
    key_version integer NOT NULL CHECK (key_version > 0),
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    ciphertext bytea NOT NULL CHECK (octet_length(ciphertext) > 16)
);

CREATE TABLE identity_outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    challenge_id uuid NOT NULL UNIQUE REFERENCES identity_challenges (id),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    next_attempt_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    claim_owner text,
    claimed_until timestamptz,
    published_at timestamptz,
    CHECK ((claim_owner IS NULL) = (claimed_until IS NULL))
);

CREATE TABLE identity_limit_counters (
    scope text NOT NULL CHECK (scope IN ('source', 'account')),
    counter_key text NOT NULL CHECK (counter_key <> ''),
    action text NOT NULL CHECK (action IN ('signup', 'code-request', 'code-guess')),
    window_start timestamptz NOT NULL,
    count integer NOT NULL CHECK (count > 0),
    PRIMARY KEY (scope, counter_key, action, window_start)
);

-- +goose Down
DROP TABLE identity_limit_counters;
DROP TABLE identity_outbox_events;
DROP TABLE identity_challenge_deliveries;
DROP TABLE identity_challenges;
DROP TABLE identity_sessions;
DROP TABLE identity_accounts;
