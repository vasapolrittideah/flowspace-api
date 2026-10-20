-- +goose Up
ALTER TABLE identity_challenges DROP CONSTRAINT identity_challenges_purpose_check;
ALTER TABLE identity_challenges ADD CONSTRAINT identity_challenges_purpose_check
    CHECK (purpose IN ('verify-email', 'claim-account', 'password-reset', 'reauthenticate'));

-- +goose Down
-- Rollback discards re-authentication codes and their pending delivery requests.
DELETE FROM identity_outbox_events
WHERE challenge_id IN (SELECT id FROM identity_challenges WHERE purpose = 'reauthenticate');
DELETE FROM identity_challenges WHERE purpose = 'reauthenticate';
ALTER TABLE identity_challenges DROP CONSTRAINT identity_challenges_purpose_check;
ALTER TABLE identity_challenges ADD CONSTRAINT identity_challenges_purpose_check
    CHECK (purpose IN ('verify-email', 'claim-account', 'password-reset'));
