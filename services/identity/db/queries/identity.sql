-- name: CreateAccount :one
INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash)
VALUES (sqlc.arg(subject), sqlc.arg(email_local), sqlc.arg(email_domain), sqlc.arg(password_hash)::text)
RETURNING subject, email_local, email_domain, email_verified_at, created_at;

-- name: RetireUnverifiedAccount :execrows
UPDATE identity_accounts
SET retired_at = statement_timestamp()
WHERE subject = sqlc.arg(subject)
  AND email_verified_at IS NULL
  AND retired_at IS NULL;

-- name: GetActiveAccountForUpdate :one
SELECT subject, email_local, email_domain, email_verified_at
FROM identity_accounts
WHERE subject = sqlc.arg(subject)
  AND retired_at IS NULL
FOR UPDATE;

-- name: GetActiveAccountByEmailForUpdate :one
SELECT subject, email_local, email_domain, email_verified_at
FROM identity_accounts
WHERE email_local = sqlc.arg(email_local)
  AND email_domain = sqlc.arg(email_domain)
  AND retired_at IS NULL
FOR UPDATE;

-- name: GetActiveAccountByEmail :one
SELECT subject, email_verified_at
FROM identity_accounts
WHERE email_local = sqlc.arg(email_local)
  AND email_domain = sqlc.arg(email_domain)
  AND retired_at IS NULL;

-- name: GetPasswordAccountByEmail :one
SELECT subject, password_hash::text AS password_hash, email_verified_at
FROM identity_accounts
WHERE email_local = sqlc.arg(email_local)
  AND email_domain = sqlc.arg(email_domain)
  AND password_hash IS NOT NULL
  AND retired_at IS NULL;

-- name: GetRecoveryAccountForUpdate :one
SELECT subject, (password_hash IS NOT NULL)::boolean AS has_password, email_verified_at
FROM identity_accounts
WHERE email_local = sqlc.arg(email_local)
  AND email_domain = sqlc.arg(email_domain)
  AND retired_at IS NULL
FOR UPDATE;

-- name: GetPasswordAccountForUpdate :one
SELECT subject, password_hash::text AS password_hash, email_verified_at
FROM identity_accounts
WHERE subject = sqlc.arg(subject)
  AND password_hash IS NOT NULL
  AND retired_at IS NULL
FOR UPDATE;

-- name: MarkEmailVerified :execrows
UPDATE identity_accounts
SET email_verified_at = statement_timestamp()
WHERE subject = sqlc.arg(subject)
  AND email_verified_at IS NULL
  AND retired_at IS NULL;

-- name: UpdatePasswordHash :execrows
UPDATE identity_accounts
SET password_hash = sqlc.arg(password_hash)::text
WHERE subject = sqlc.arg(subject)
  AND email_verified_at IS NOT NULL
  AND retired_at IS NULL;
