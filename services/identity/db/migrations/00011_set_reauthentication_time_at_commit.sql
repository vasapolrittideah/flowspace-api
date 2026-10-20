-- +goose Up
-- Stamp a successful proof during COMMIT, after application work has finished.
-- +goose StatementBegin
CREATE FUNCTION identity_stamp_session_reauthentication() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    requested_session text := NULLIF(current_setting('flowspace.reauthentication_session', true), '');
BEGIN
    IF requested_session = NEW.id::text THEN
        PERFORM set_config('flowspace.reauthentication_session', '', true);
        UPDATE identity_sessions
        SET reauthenticated_at = clock_timestamp()
        WHERE id = NEW.id;
    END IF;
    RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER identity_sessions_reauthentication_proof
AFTER UPDATE OF reauthenticated_at ON identity_sessions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION identity_stamp_session_reauthentication();

-- +goose Down
-- Before you remove the commit-time proof mechanism, deploy the older code.
DROP TRIGGER identity_sessions_reauthentication_proof ON identity_sessions;
DROP FUNCTION identity_stamp_session_reauthentication();
