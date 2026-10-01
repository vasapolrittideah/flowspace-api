-- +goose Up
ALTER TABLE identity_provider_login_attempts
    ADD COLUMN state_consumed_at timestamptz,
    ADD COLUMN failed_at timestamptz,
    ADD COLUMN provider_subject text CHECK (provider_subject <> ''),
    ADD COLUMN provider_email text CHECK (provider_email <> ''),
    ADD COLUMN provider_email_verified boolean,
    ADD COLUMN provider_hosted_domain text CHECK (provider_hosted_domain <> ''),
    ADD COLUMN handoff_code_verifier bytea UNIQUE CHECK (octet_length(handoff_code_verifier) = 32),
    ADD CONSTRAINT identity_provider_login_attempts_result_check CHECK (
        (handoff_code_verifier IS NULL) = (provider_subject IS NULL)
        AND (handoff_code_verifier IS NULL) = (provider_email_verified IS NULL)
        AND (handoff_code_verifier IS NULL OR (state_consumed_at IS NOT NULL AND failed_at IS NULL))
        AND (failed_at IS NULL OR state_consumed_at IS NOT NULL)
    );

ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check
    CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login', 'provider-login-start', 'provider-callback'));

-- +goose Down
DELETE FROM identity_limit_counters WHERE action = 'provider-callback';
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check
    CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login', 'provider-login-start'));
ALTER TABLE identity_provider_login_attempts
    DROP CONSTRAINT identity_provider_login_attempts_result_check,
    DROP COLUMN handoff_code_verifier,
    DROP COLUMN provider_hosted_domain,
    DROP COLUMN provider_email_verified,
    DROP COLUMN provider_email,
    DROP COLUMN provider_subject,
    DROP COLUMN failed_at,
    DROP COLUMN state_consumed_at;
