-- +goose Up
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_scope_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_scope_check CHECK (scope IN ('source', 'account', 'email'));
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check CHECK (action IN ('signup', 'code-request', 'code-guess', 'password-login'));

-- +goose Down
DELETE FROM identity_limit_counters WHERE action = 'password-login';
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_action_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_action_check CHECK (action IN ('signup', 'code-request', 'code-guess'));
ALTER TABLE identity_limit_counters DROP CONSTRAINT identity_limit_counters_scope_check;
ALTER TABLE identity_limit_counters ADD CONSTRAINT identity_limit_counters_scope_check CHECK (scope IN ('source', 'account'));
