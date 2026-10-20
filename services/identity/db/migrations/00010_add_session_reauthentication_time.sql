-- +goose Up
-- Existing sessions keep a closed window until their owners prove their identity.
ALTER TABLE identity_sessions ADD COLUMN reauthenticated_at timestamptz;

-- +goose StatementBegin
CREATE FUNCTION identity_check_reauthentication_deadline() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    deadline timestamptz := NULLIF(current_setting('flowspace.reauthentication_deadline', true), '')::timestamptz;
BEGIN
    IF deadline IS NOT NULL AND clock_timestamp() >= deadline THEN
        RAISE EXCEPTION 're-authentication required'
            USING ERRCODE = 'P0001', CONSTRAINT = 'identity_sessions_reauthentication_deadline';
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER identity_sessions_reauthentication_deadline
AFTER UPDATE OF reauthenticated_at ON identity_sessions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION identity_check_reauthentication_deadline();

-- +goose Down
-- Deploy older code before rollback. This discards the proof times.
DROP TRIGGER identity_sessions_reauthentication_deadline ON identity_sessions;
DROP FUNCTION identity_check_reauthentication_deadline();
ALTER TABLE identity_sessions DROP COLUMN reauthenticated_at;
