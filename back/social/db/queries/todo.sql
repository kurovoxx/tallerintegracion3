-- name: GetGroupByID :one
SELECT * FROM social.groups
WHERE id = $1 LIMIT 1;

-- name: GetBoardByID :one
SELECT * FROM social.todo_boards
WHERE id = $1 LIMIT 1;

-- name: ListBoardsByGroup :many
SELECT * FROM social.todo_boards
WHERE group_id = $1
ORDER BY created_at ASC;

-- name: CreateBoard :one
INSERT INTO social.todo_boards (group_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: GetTodoTaskByID :one
SELECT * FROM social.todo_tasks
WHERE id = $1 LIMIT 1;

-- name: CreateTodoTask :one
INSERT INTO social.todo_tasks (
  board_id, title, status, assignee_user_id, due_date
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING *;

-- name: ListTodoTasksByGroup :many
SELECT t.*, b.name AS board_name
FROM social.todo_tasks t
JOIN social.todo_boards b ON b.id = t.board_id
WHERE b.group_id = $1
  AND ($2 = '' OR t.status = $2)
  AND ($3::uuid IS NULL OR t.board_id = $3)
ORDER BY t.created_at ASC;

-- name: UpdateTodoTask :one
UPDATE social.todo_tasks SET
  title = $2,
  status = $3,
  assignee_user_id = $4,
  due_date = $5,
  updated_at = now()
WHERE id = $1
RETURNING *;

-- name: DeleteTodoTask :one
DELETE FROM social.todo_tasks
WHERE id = $1
RETURNING *;
