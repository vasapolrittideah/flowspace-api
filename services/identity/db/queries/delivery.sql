-- name: StoreChallengeDelivery :exec
INSERT INTO identity_challenge_deliveries (challenge_id, key_version, nonce, ciphertext)
VALUES (sqlc.arg(challenge_id), sqlc.arg(key_version), sqlc.arg(nonce), sqlc.arg(ciphertext));

-- name: DeleteChallengeDelivery :execrows
DELETE FROM identity_challenge_deliveries
WHERE challenge_id = sqlc.arg(challenge_id);

-- name: DeleteAccountChallengeDeliveries :execrows
DELETE FROM identity_challenge_deliveries AS delivery
USING identity_challenges AS challenge
WHERE delivery.challenge_id = challenge.id
  AND challenge.account_subject = sqlc.arg(account_subject);

-- name: DeleteReplacedChallengeDeliveries :execrows
DELETE FROM identity_challenge_deliveries AS delivery
USING identity_challenges AS challenge
WHERE delivery.challenge_id = challenge.id
  AND challenge.account_subject = sqlc.arg(account_subject)
  AND challenge.purpose = sqlc.arg(purpose)
  AND challenge.replaced_at IS NOT NULL;

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
       OR (account.email_verified_at IS NOT NULL AND challenge.purpose <> 'password-reset')
    FOR UPDATE OF account, challenge, delivery SKIP LOCKED
)
DELETE FROM identity_challenge_deliveries AS delivery
USING terminal
WHERE delivery.challenge_id = terminal.challenge_id;
