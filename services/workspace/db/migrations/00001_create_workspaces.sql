-- +goose Up
CREATE TABLE workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (name = btrim(name) AND char_length(name) BETWEEN 1 AND 100),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workspace_memberships (
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    subject text NOT NULL CHECK (subject <> ''),
    role text NOT NULL CHECK (role IN ('viewer', 'member', 'admin', 'owner')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, subject)
);

CREATE UNIQUE INDEX workspace_memberships_one_owner
    ON workspace_memberships (workspace_id)
    WHERE role = 'owner';

CREATE TABLE workspace_creations (
    subject text NOT NULL CHECK (subject <> ''),
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 255),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    workspace_id uuid NOT NULL REFERENCES workspaces (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subject, idempotency_key)
);

-- +goose Down
DROP TABLE workspace_creations;
DROP TABLE workspace_memberships;
DROP TABLE workspaces;
