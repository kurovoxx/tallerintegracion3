-- name: GetSheetByID :one
SELECT * FROM social.sprint_sheets
WHERE id = $1 LIMIT 1;

-- name: ListSheetsByGroup :many
SELECT * FROM social.sprint_sheets
WHERE group_id = $1
ORDER BY created_at ASC;

-- name: CreateSheet :one
INSERT INTO social.sprint_sheets (group_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetSprintSheetTaskByID :one
SELECT * FROM social.sprint_sheet_tasks
WHERE id = $1 LIMIT 1;

-- name: CreateSprintSheetTask :one
INSERT INTO social.sprint_sheet_tasks (
  sheet_id, assignee_user_id, title, priority, status, estimated_hours
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: ListSprintSheetTasksByGroup :many
SELECT t.*, s.name AS sheet_name
FROM social.sprint_sheet_tasks t
JOIN social.sprint_sheets s ON s.id = t.sheet_id
WHERE s.group_id = $1
  AND ($2 = '' OR t.status = $2)
  AND ($3 = '' OR t.priority = $3)
  AND ($4::uuid IS NULL OR t.sheet_id = $4)
ORDER BY t.created_at ASC;

-- name: UpdateSprintSheetTask :one
UPDATE social.sprint_sheet_tasks SET
  title = $2,
  assignee_user_id = $3,
  priority = $4,
  status = $5,
  estimated_hours = $6,
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteSprintSheetTask :one
DELETE FROM social.sprint_sheet_tasks
WHERE id = $1
RETURNING *;
