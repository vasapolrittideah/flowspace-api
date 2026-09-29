-- name: CreateOutboxEvent :one
INSERT INTO identity_outbox_events (challenge_id)
VALUES (sqlc.arg(challenge_id))
RETURNING id;

-- name: ClaimOutboxEvent :one
WITH next_event AS (
    SELECT event.id
    FROM identity_outbox_events AS event
    JOIN identity_challenges AS challenge ON challenge.id = event.challenge_id
    WHERE challenge.purpose <> 'password-reset'
      AND event.published_at IS NULL
      AND event.next_attempt_at <= statement_timestamp()
      AND (event.claimed_until IS NULL OR event.claimed_until <= statement_timestamp())
    ORDER BY event.next_attempt_at, event.created_at
    LIMIT 1
    FOR UPDATE OF event SKIP LOCKED
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
