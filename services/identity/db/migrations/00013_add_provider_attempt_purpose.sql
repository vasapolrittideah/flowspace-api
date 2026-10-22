-- +goose Up
-- Existing attempts get the login purpose. A re-authentication claim sets its
-- own column, and the claim check rejects a session claim of a
-- re-authentication attempt, so older replicas cannot complete it as login.
ALTER TABLE identity_provider_login_attempts
    ADD COLUMN purpose text NOT NULL DEFAULT 'login' CHECK (purpose IN ('login', 'reauthenticate')),
    ADD COLUMN account_subject text REFERENCES identity_accounts (subject),
    ADD COLUMN session_id uuid REFERENCES identity_sessions (id),
    ADD COLUMN reauthentication_claimed_at timestamptz,
    ADD CONSTRAINT identity_provider_login_attempts_binding_check CHECK (
        (purpose = 'reauthenticate') = (account_subject IS NOT NULL)
        AND (purpose = 'reauthenticate') = (session_id IS NOT NULL)
    ),
    ADD CONSTRAINT identity_provider_login_attempts_purpose_claim_check CHECK (
        (purpose = 'login' OR session_claimed_at IS NULL)
        AND (purpose = 'reauthenticate' OR reauthentication_claimed_at IS NULL)
        AND (reauthentication_claimed_at IS NULL OR handoff_code_verifier IS NOT NULL)
    );

-- +goose Down
-- Older code completes every attempt as provider login, so delete the
-- re-authentication attempts before you deploy it.
DELETE FROM identity_provider_login_attempts WHERE purpose = 'reauthenticate';
ALTER TABLE identity_provider_login_attempts
    DROP CONSTRAINT identity_provider_login_attempts_purpose_claim_check,
    DROP CONSTRAINT identity_provider_login_attempts_binding_check,
    DROP COLUMN reauthentication_claimed_at,
    DROP COLUMN session_id,
    DROP COLUMN account_subject,
    DROP COLUMN purpose;
