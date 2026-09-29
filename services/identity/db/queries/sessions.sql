-- name: GetActiveAccountForSessionForUpdate :one
SELECT account.email_local, account.email_domain, account.email_verified_at
FROM identity_accounts AS account
JOIN identity_sessions AS session ON session.account_subject = account.subject
WHERE account.subject = sqlc.arg(subject)
  AND session.id = sqlc.arg(session_id)
  AND account.retired_at IS NULL
  AND session.revoked_at IS NULL
  AND session.idle_expires_at > statement_timestamp()
  AND session.absolute_expires_at > statement_timestamp()
FOR UPDATE OF account, session;

-- name: GetActiveSessionState :one
SELECT account.email_verified_at
FROM identity_accounts AS account
JOIN identity_sessions AS session ON session.account_subject = account.subject
WHERE account.subject = sqlc.arg(subject)
  AND session.id = sqlc.arg(session_id)
  AND account.retired_at IS NULL
  AND session.revoked_at IS NULL
  AND session.idle_expires_at > statement_timestamp()
  AND session.absolute_expires_at > statement_timestamp();

-- name: CreateSession :one
INSERT INTO identity_sessions (account_subject, refresh_token_hash, idle_expires_at, absolute_expires_at)
VALUES (sqlc.arg(account_subject), sqlc.arg(refresh_token_hash),
    statement_timestamp() + INTERVAL '30 days', statement_timestamp() + INTERVAL '90 days')
RETURNING id, created_at, idle_expires_at, absolute_expires_at;

-- name: FindRefreshSession :one
SELECT id FROM identity_sessions WHERE refresh_token_hash = sqlc.arg(token_hash)
UNION ALL
SELECT session_id AS id FROM identity_rotated_refresh_tokens WHERE token_hash = sqlc.arg(token_hash)
LIMIT 1;

-- name: GetRefreshSessionForUpdate :one
SELECT session.account_subject, session.refresh_token_hash,
    account.retired_at IS NULL AND session.revoked_at IS NULL
    AND session.idle_expires_at > statement_timestamp()
    AND session.absolute_expires_at > statement_timestamp() AS active
FROM identity_sessions AS session
JOIN identity_accounts AS account ON account.subject = session.account_subject
WHERE session.id = sqlc.arg(session_id)
FOR UPDATE OF account, session;

-- name: RecordRotatedRefreshToken :exec
INSERT INTO identity_rotated_refresh_tokens (token_hash, session_id)
VALUES (sqlc.arg(token_hash), sqlc.arg(session_id));

-- name: RotateCurrentRefreshToken :one
UPDATE identity_sessions
SET refresh_token_hash = sqlc.arg(new_hash),
    idle_expires_at = LEAST(statement_timestamp() + INTERVAL '30 days', absolute_expires_at)
WHERE id = sqlc.arg(session_id)
  AND refresh_token_hash = sqlc.arg(old_hash)
  AND revoked_at IS NULL
  AND idle_expires_at > statement_timestamp()
  AND absolute_expires_at > statement_timestamp()
RETURNING statement_timestamp()::timestamptz AS issued_at, idle_expires_at, absolute_expires_at;

-- name: RevokeRefreshSession :exec
UPDATE identity_sessions SET revoked_at = statement_timestamp()
WHERE id = sqlc.arg(session_id) AND revoked_at IS NULL;

-- name: RevokeAccountSessions :execrows
UPDATE identity_sessions
SET revoked_at = statement_timestamp()
WHERE account_subject = sqlc.arg(account_subject)
  AND revoked_at IS NULL;

-- name: RevokeCurrentSession :execrows
UPDATE identity_sessions AS session
SET revoked_at = statement_timestamp()
FROM identity_accounts AS account
WHERE session.id = sqlc.arg(session_id)
  AND session.account_subject = sqlc.arg(subject)
  AND account.subject = session.account_subject
  AND account.retired_at IS NULL
  AND session.revoked_at IS NULL
  AND session.idle_expires_at > statement_timestamp()
  AND session.absolute_expires_at > statement_timestamp();
