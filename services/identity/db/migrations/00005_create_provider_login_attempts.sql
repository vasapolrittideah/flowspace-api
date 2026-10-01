-- +goose Up
CREATE TABLE identity_provider_login_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider text NOT NULL CHECK (provider IN ('google', 'github')),
    attempt_token_verifier bytea NOT NULL UNIQUE CHECK (octet_length(attempt_token_verifier) = 32),
    state_verifier bytea NOT NULL UNIQUE CHECK (octet_length(state_verifier) = 32),
    code_verifier text NOT NULL CHECK (char_length(code_verifier) BETWEEN 43 AND 128),
    nonce text CHECK (nonce <> ''),
    callback_url text NOT NULL CHECK (callback_url <> ''),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    expires_at timestamptz NOT NULL,
    CHECK ((provider = 'google') = (nonce IS NOT NULL)),
    CHECK (expires_at = created_at + INTERVAL '10 minutes')
);

ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check
    CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login', 'provider-login-start'));

-- +goose Down
DELETE FROM identity_limit_counters WHERE action = 'provider-login-start';
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check
    CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login'));
DROP TABLE identity_provider_login_attempts;
