-- name: CreateAccount :one
INSERT INTO identity_accounts (subject, email_local, email_domain, password_hash)
VALUES (sqlc.arg(subject), sqlc.arg(email_local), sqlc.arg(email_domain), sqlc.arg(password_hash))
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

-- name: MarkEmailVerified :execrows
UPDATE identity_accounts
SET email_verified_at = statement_timestamp()
WHERE subject = sqlc.arg(subject)
  AND email_verified_at IS NULL
  AND retired_at IS NULL;

-- name: CreateSession :one
INSERT INTO identity_sessions (account_subject, refresh_token_hash, idle_expires_at, absolute_expires_at)
VALUES (sqlc.arg(account_subject), sqlc.arg(refresh_token_hash),
    statement_timestamp() + INTERVAL '30 days', statement_timestamp() + INTERVAL '90 days')
RETURNING id, created_at, idle_expires_at, absolute_expires_at;

-- name: RevokeAccountSessions :execrows
UPDATE identity_sessions
SET revoked_at = statement_timestamp()
WHERE account_subject = sqlc.arg(account_subject)
  AND revoked_at IS NULL;

-- name: ReplaceCurrentChallenge :execrows
UPDATE identity_challenges
SET replaced_at = statement_timestamp()
WHERE account_subject = sqlc.arg(account_subject)
  AND purpose = sqlc.arg(purpose)
  AND replaced_at IS NULL
  AND consumed_at IS NULL;

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

-- name: StoreChallengeDelivery :exec
INSERT INTO identity_challenge_deliveries (challenge_id, key_version, nonce, ciphertext)
VALUES (sqlc.arg(challenge_id), sqlc.arg(key_version), sqlc.arg(nonce), sqlc.arg(ciphertext));

-- name: DeleteChallengeDelivery :execrows
DELETE FROM identity_challenge_deliveries
WHERE challenge_id = sqlc.arg(challenge_id);

-- name: GetDeliveryAccountForUpdate :one
SELECT account.subject, account.email_local, account.email_domain, account.email_verified_at, account.retired_at
FROM identity_accounts AS account
JOIN identity_challenges AS challenge ON challenge.account_subject = account.subject
WHERE challenge.id = sqlc.arg(challenge_id)
FOR UPDATE OF account;

-- name: GetDeliveryChallengeForUpdate :one
SELECT purpose, email_local, email_domain, replaced_at, consumed_at, wrong_guesses,
    expires_at <= statement_timestamp() AS expired
FROM identity_challenges
WHERE id = sqlc.arg(challenge_id)
FOR UPDATE;

-- name: GetChallengeDelivery :one
SELECT key_version, nonce, ciphertext
FROM identity_challenge_deliveries
WHERE challenge_id = sqlc.arg(challenge_id);

-- name: PurgeTerminalDeliveries :execrows
WITH terminal AS (
    SELECT delivery.challenge_id
    FROM identity_challenge_deliveries AS delivery
    JOIN identity_challenges AS challenge ON challenge.id = delivery.challenge_id
    JOIN identity_accounts AS account ON account.subject = challenge.account_subject
    WHERE challenge.expires_at <= statement_timestamp()
       OR challenge.replaced_at IS NOT NULL
       OR challenge.consumed_at IS NOT NULL
       OR challenge.wrong_guesses >= 5
       OR challenge.email_local <> account.email_local
       OR challenge.email_domain <> account.email_domain
       OR account.retired_at IS NOT NULL
       OR account.email_verified_at IS NOT NULL
    FOR UPDATE OF account, challenge, delivery SKIP LOCKED
)
DELETE FROM identity_challenge_deliveries AS delivery
USING terminal
WHERE delivery.challenge_id = terminal.challenge_id;

-- name: CreateOutboxEvent :one
INSERT INTO identity_outbox_events (challenge_id)
VALUES (sqlc.arg(challenge_id))
RETURNING id;

-- name: ClaimOutboxEvent :one
WITH next_event AS (
    SELECT id
    FROM identity_outbox_events
    WHERE published_at IS NULL
      AND next_attempt_at <= statement_timestamp()
      AND (claimed_until IS NULL OR claimed_until <= statement_timestamp())
    ORDER BY next_attempt_at, created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE identity_outbox_events AS event
SET claim_owner = sqlc.arg(claim_owner),
    claimed_until = statement_timestamp() + INTERVAL '30 seconds'
FROM next_event, identity_challenges AS challenge
WHERE event.id = next_event.id
  AND challenge.id = event.challenge_id
RETURNING event.id, event.challenge_id, challenge.purpose;

-- name: MarkOutboxPublished :execrows
UPDATE identity_outbox_events
SET published_at = statement_timestamp(), claim_owner = NULL, claimed_until = NULL
WHERE id = sqlc.arg(id)
  AND claim_owner = sqlc.arg(claim_owner)
  AND claimed_until > statement_timestamp()
  AND published_at IS NULL;

-- name: ReleaseOutboxClaim :execrows
UPDATE identity_outbox_events
SET attempt_count = attempt_count + 1,
    next_attempt_at = sqlc.arg(next_attempt_at),
    claim_owner = NULL,
    claimed_until = NULL
WHERE id = sqlc.arg(id)
  AND claim_owner = sqlc.arg(claim_owner)
  AND claimed_until > statement_timestamp()
  AND published_at IS NULL;

-- name: IncrementLimitCounter :one
INSERT INTO identity_limit_counters (scope, counter_key, action, window_start, count)
VALUES (sqlc.arg(scope), sqlc.arg(counter_key), sqlc.arg(action), sqlc.arg(window_start), 1)
ON CONFLICT (scope, counter_key, action, window_start)
DO UPDATE SET count = identity_limit_counters.count + 1
RETURNING count;

-- name: GetLimitUsage :one
SELECT COALESCE(SUM(count) FILTER (WHERE window_start > sqlc.arg(hour_cutoff)::timestamptz), 0)::bigint AS hourly_count,
       COALESCE(SUM(count) FILTER (WHERE window_start > sqlc.arg(day_cutoff)::timestamptz), 0)::bigint AS daily_count,
       (SELECT window_start FROM identity_limit_counters AS recent
        WHERE recent.scope = sqlc.arg(scope)
          AND recent.counter_key = sqlc.arg(counter_key)
          AND recent.action = sqlc.arg(action)
        ORDER BY window_start DESC LIMIT 1) AS latest_at
FROM identity_limit_counters
WHERE scope = sqlc.arg(scope)
  AND counter_key = sqlc.arg(counter_key)
  AND action = sqlc.arg(action)
  AND window_start > sqlc.arg(day_cutoff)::timestamptz;
