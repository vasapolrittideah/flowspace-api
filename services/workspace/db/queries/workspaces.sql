-- name: TryCreateWorkspaceLock :one
SELECT pg_try_advisory_xact_lock(sqlc.arg(lock_id)::bigint);

-- name: GetWorkspaceCreation :one
SELECT
    request_hash,
    workspace_id::text AS id,
    workspace_name AS name,
    workspace_created_at AS created_at
FROM workspace_creations AS wc
WHERE wc.subject = sqlc.arg(subject)
  AND wc.idempotency_key = sqlc.arg(idempotency_key);

-- name: CreateWorkspace :one
INSERT INTO workspaces (name)
VALUES (sqlc.arg(name))
RETURNING id::text AS id, name, created_at;

-- name: CreateWorkspaceOwner :exec
INSERT INTO workspace_memberships (workspace_id, subject, role)
VALUES (sqlc.arg(workspace_id)::uuid, sqlc.arg(subject), 'owner');

-- name: RecordWorkspaceCreation :exec
INSERT INTO workspace_creations (
    subject,
    idempotency_key,
    request_hash,
    workspace_id,
    workspace_name,
    workspace_created_at
)
VALUES (
    sqlc.arg(subject),
    sqlc.arg(idempotency_key),
    sqlc.arg(request_hash),
    sqlc.arg(workspace_id)::uuid,
    sqlc.arg(workspace_name),
    sqlc.arg(workspace_created_at)
);

-- name: GetWorkspaceForSubject :one
SELECT w.id::text AS id, w.name, w.created_at
FROM workspaces AS w
JOIN workspace_memberships AS wm ON wm.workspace_id = w.id
WHERE w.id = sqlc.arg(workspace_id)::uuid
  AND wm.subject = sqlc.arg(subject);
