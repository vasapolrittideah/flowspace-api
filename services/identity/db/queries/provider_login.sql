-- name: CreateProviderLoginAttempt :one
INSERT INTO identity_provider_login_attempts (
    provider, attempt_token_verifier, state_verifier, code_verifier, nonce, callback_url, expires_at
)
VALUES (
    sqlc.arg(provider), sqlc.arg(attempt_token_verifier), sqlc.arg(state_verifier), sqlc.arg(code_verifier),
    sqlc.narg(nonce), sqlc.arg(callback_url), statement_timestamp() + INTERVAL '10 minutes'
)
RETURNING expires_at;

-- name: ConsumeProviderLoginState :one
UPDATE identity_provider_login_attempts
SET state_consumed_at = statement_timestamp()
WHERE provider = sqlc.arg(provider)
  AND state_verifier = sqlc.arg(state_verifier)
  AND state_consumed_at IS NULL
  AND expires_at > statement_timestamp()
RETURNING id, code_verifier, nonce, callback_url;

-- name: RecordProviderLoginResult :execrows
UPDATE identity_provider_login_attempts
SET provider_subject = sqlc.arg(provider_subject),
    provider_email = sqlc.narg(provider_email),
    provider_email_verified = sqlc.arg(provider_email_verified),
    provider_hosted_domain = sqlc.narg(provider_hosted_domain),
    handoff_code_verifier = sqlc.arg(handoff_code_verifier)
WHERE id = sqlc.arg(id)
  AND state_consumed_at IS NOT NULL
  AND failed_at IS NULL
  AND handoff_code_verifier IS NULL
  AND expires_at > statement_timestamp();

-- name: FailProviderLoginAttempt :exec
UPDATE identity_provider_login_attempts
SET failed_at = statement_timestamp()
WHERE id = sqlc.arg(id)
  AND state_consumed_at IS NOT NULL
  AND failed_at IS NULL
  AND handoff_code_verifier IS NULL;

-- name: ClaimProviderLoginResult :one
UPDATE identity_provider_login_attempts
SET session_claimed_at = statement_timestamp()
WHERE attempt_token_verifier = sqlc.arg(attempt_token_verifier)
  AND handoff_code_verifier = sqlc.arg(handoff_code_verifier)
  AND session_claimed_at IS NULL
  AND handoff_failures < 5
  AND expires_at > statement_timestamp()
RETURNING provider, provider_subject;

-- name: RecordProviderHandoffFailure :exec
UPDATE identity_provider_login_attempts
SET handoff_failures = handoff_failures + 1
WHERE attempt_token_verifier = sqlc.arg(attempt_token_verifier)
  AND session_claimed_at IS NULL
  AND handoff_failures < 5
  AND expires_at > statement_timestamp();

-- name: GetLinkedAccountForUpdate :one
SELECT account.subject, account.email_verified_at
FROM identity_provider_links AS link
JOIN identity_accounts AS account ON account.subject = link.account_subject
WHERE link.provider = sqlc.arg(provider)
  AND link.provider_subject = sqlc.arg(provider_subject)
  AND account.retired_at IS NULL
FOR UPDATE OF account;
