-- name: GetWorkspaceForSubject :one
SELECT w.id::text AS id, w.name, w.created_at
FROM workspaces AS w
JOIN workspace_memberships AS wm ON wm.workspace_id = w.id
WHERE w.id = sqlc.arg(workspace_id)::uuid
  AND wm.subject = sqlc.arg(subject);
