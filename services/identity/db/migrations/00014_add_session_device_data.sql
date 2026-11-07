-- +goose Up
-- Existing sessions get no sign-in method and no device data. Their last-used
-- time is their creation time until their next refresh.
ALTER TABLE identity_sessions
    ADD COLUMN sign_in_method text CHECK (sign_in_method IN ('password', 'google', 'github')),
    ADD COLUMN created_ip_address inet,
    ADD COLUMN created_user_agent text CHECK (octet_length(created_user_agent) <= 512),
    ADD COLUMN last_used_at timestamptz,
    ADD COLUMN last_used_ip_address inet,
    ADD COLUMN last_used_user_agent text CHECK (octet_length(last_used_user_agent) <= 512);
UPDATE identity_sessions SET last_used_at = created_at;
ALTER TABLE identity_sessions
    ALTER COLUMN last_used_at SET DEFAULT statement_timestamp(),
    ALTER COLUMN last_used_at SET NOT NULL;

-- Each revocation sets revoked_at, so this trigger deletes the device data in
-- the same statement, also when older code revokes the session.
-- +goose StatementBegin
CREATE FUNCTION identity_delete_revoked_session_device_data() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    NEW.created_ip_address := NULL;
    NEW.created_user_agent := NULL;
    NEW.last_used_ip_address := NULL;
    NEW.last_used_user_agent := NULL;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER identity_sessions_delete_revoked_device_data
BEFORE INSERT OR UPDATE ON identity_sessions
FOR EACH ROW WHEN (NEW.revoked_at IS NOT NULL)
EXECUTE FUNCTION identity_delete_revoked_session_device_data();

-- +goose Down
-- Deploy older code before the rollback. This discards the device data and the
-- sign-in method of each session.
DROP TRIGGER identity_sessions_delete_revoked_device_data ON identity_sessions;
DROP FUNCTION identity_delete_revoked_session_device_data();
ALTER TABLE identity_sessions
    DROP COLUMN last_used_user_agent,
    DROP COLUMN last_used_ip_address,
    DROP COLUMN last_used_at,
    DROP COLUMN created_user_agent,
    DROP COLUMN created_ip_address,
    DROP COLUMN sign_in_method;
