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
