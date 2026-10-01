-- +goose Up
CREATE TABLE identity_provider_links (
    provider text NOT NULL CHECK (provider IN ('google', 'github')),
    provider_subject text NOT NULL CHECK (provider_subject <> ''),
    account_subject text NOT NULL REFERENCES identity_accounts (subject),
    created_at timestamptz NOT NULL DEFAULT statement_timestamp(),
    PRIMARY KEY (provider, provider_subject)
);

ALTER TABLE identity_provider_login_attempts
    ADD COLUMN handoff_failures smallint NOT NULL DEFAULT 0 CHECK (handoff_failures BETWEEN 0 AND 5),
    ADD COLUMN session_claimed_at timestamptz,
    ADD CONSTRAINT identity_provider_login_attempts_claim_check
        CHECK (session_claimed_at IS NULL OR handoff_code_verifier IS NOT NULL);

ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check
    CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login', 'provider-login-start', 'provider-callback',
        'provider-session-failure'));

-- +goose Down
DELETE FROM identity_limit_counters WHERE action = 'provider-session-failure';
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check
    CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login', 'provider-login-start', 'provider-callback'));
ALTER TABLE identity_provider_login_attempts
    DROP CONSTRAINT identity_provider_login_attempts_claim_check,
    DROP COLUMN session_claimed_at,
    DROP COLUMN handoff_failures;
DROP TABLE identity_provider_links;
