-- name: ReplaceCurrentChallenge :execrows
UPDATE identity_challenges
SET replaced_at = statement_timestamp()
WHERE account_subject = sqlc.arg(account_subject)
  AND purpose = sqlc.arg(purpose)
  AND replaced_at IS NULL
  AND consumed_at IS NULL;

-- name: CanIssueCode :one
SELECT count(*) < 5
   AND COALESCE(max(issued_at) <= statement_timestamp() - INTERVAL '60 seconds', true) AS allowed
FROM identity_challenges
WHERE account_subject = sqlc.arg(account_subject)
  AND issued_at > statement_timestamp() - INTERVAL '1 hour';

-- name: RevokeAccountChallenges :execrows
UPDATE identity_challenges
SET replaced_at = statement_timestamp()
WHERE account_subject = sqlc.arg(account_subject)
  AND replaced_at IS NULL
  AND consumed_at IS NULL;

-- name: CreateChallenge :one
INSERT INTO identity_challenges (
    account_subject, purpose, email_local, email_domain, code_verifier, expires_at
)
VALUES (
    sqlc.arg(account_subject), sqlc.arg(purpose), sqlc.arg(email_local),
    sqlc.arg(email_domain), sqlc.arg(code_verifier), statement_timestamp() + INTERVAL '10 minutes'
)
RETURNING id, expires_at;

-- name: GetCurrentChallengeForUpdate :one
SELECT id, account_subject, purpose, email_local, email_domain, code_verifier,
    wrong_guesses, expires_at
FROM identity_challenges
WHERE account_subject = sqlc.arg(account_subject)
  AND purpose = sqlc.arg(purpose)
  AND replaced_at IS NULL
  AND consumed_at IS NULL
FOR UPDATE;

-- name: IncrementChallengeWrongGuess :one
UPDATE identity_challenges
SET wrong_guesses = wrong_guesses + 1
WHERE id = sqlc.arg(id)
  AND replaced_at IS NULL
  AND consumed_at IS NULL
  AND expires_at > statement_timestamp()
  AND wrong_guesses < 5
RETURNING wrong_guesses;

-- name: ConsumeCurrentChallenge :execrows
UPDATE identity_challenges
SET consumed_at = statement_timestamp()
WHERE id = sqlc.arg(id)
  AND replaced_at IS NULL
  AND consumed_at IS NULL
  AND expires_at > statement_timestamp()
  AND wrong_guesses < 5;
