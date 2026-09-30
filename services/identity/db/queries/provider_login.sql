-- name: CreateProviderLoginAttempt :one
INSERT INTO identity_provider_login_attempts (
    provider, attempt_token_verifier, state_verifier, code_verifier, nonce, callback_url, expires_at
)
VALUES (
    sqlc.arg(provider), sqlc.arg(attempt_token_verifier), sqlc.arg(state_verifier), sqlc.arg(code_verifier),
    sqlc.narg(nonce), sqlc.arg(callback_url), statement_timestamp() + INTERVAL '10 minutes'
)
RETURNING expires_at;
