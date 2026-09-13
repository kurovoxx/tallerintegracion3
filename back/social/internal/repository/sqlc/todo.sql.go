package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getBoardByID = `-- name: GetBoardByID :one
SELECT id, group_id, name, created_at FROM social.todo_boards
WHERE id = $1 LIMIT 1
`

func (q *Queries) GetBoardByID(ctx context.Context, id pgtype.UUID) (SocialTodoBoard, error) {
	row := q.db.QueryRow(ctx, getBoardByID, id)
	var i SocialTodoBoard
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Name,
		&i.CreatedAt,
	)
	return i, err
}

const listBoardsByGroup = `-- name: ListBoardsByGroup :many
SELECT id, group_id, name, created_at FROM social.todo_boards
WHERE group_id = $1
ORDER BY created_at ASC
`

func (q *Queries) ListBoardsByGroup(ctx context.Context, groupID pgtype.UUID) ([]SocialTodoBoard, error) {
	rows, err := q.db.Query(ctx, listBoardsByGroup, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SocialTodoBoard
	for rows.Next() {
		var i SocialTodoBoard
		if err := rows.Scan(
			&i.ID,
			&i.GroupID,
			&i.Name,
			&i.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const createBoard = `-- name: CreateBoard :one
INSERT INTO social.todo_boards (group_id, name)
VALUES ($1, $2)
RETURNING id, group_id, name, created_at
`

type CreateBoardParams struct {
	GroupID pgtype.UUID
	Name    string
}

func (q *Queries) CreateBoard(ctx context.Context, arg CreateBoardParams) (SocialTodoBoard, error) {
	row := q.db.QueryRow(ctx, createBoard, arg.GroupID, arg.Name)
	var i SocialTodoBoard
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Name,
		&i.CreatedAt,
	)
	return i, err
}

const getTodoTaskByID = `-- name: GetTodoTaskByID :one
SELECT id, board_id, title, status, assignee_user_id, due_date, created_at, updated_at FROM social.todo_tasks
WHERE id = $1 LIMIT 1
`

func (q *Queries) GetTodoTaskByID(ctx context.Context, id pgtype.UUID) (SocialTodoTask, error) {
	row := q.db.QueryRow(ctx, getTodoTaskByID, id)
	var i SocialTodoTask
	err := row.Scan(
		&i.ID,
		&i.BoardID,
		&i.Title,
		&i.Status,
		&i.AssigneeUserID,
		&i.DueDate,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const createTodoTask = `-- name: CreateTodoTask :one
INSERT INTO social.todo_tasks (
  board_id, title, status, assignee_user_id, due_date
) VALUES (
  $1, $2, $3, $4, $5
) RETURNING id, board_id, title, status, assignee_user_id, due_date, created_at, updated_at
`

type CreateTodoTaskParams struct {
	BoardID        pgtype.UUID
	Title          string
	Status         string
	AssigneeUserID pgtype.UUID
	DueDate        pgtype.Date
}

func (q *Queries) CreateTodoTask(ctx context.Context, arg CreateTodoTaskParams) (SocialTodoTask, error) {
	row := q.db.QueryRow(ctx, createTodoTask,
		arg.BoardID,
		arg.Title,
		arg.Status,
		arg.AssigneeUserID,
		arg.DueDate,
	)
	var i SocialTodoTask
	err := row.Scan(
		&i.ID,
		&i.BoardID,
		&i.Title,
		&i.Status,
		&i.AssigneeUserID,
		&i.DueDate,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const listTodoTasksByGroup = `-- name: ListTodoTasksByGroup :many
SELECT t.id, t.board_id, t.title, t.status, t.assignee_user_id, t.due_date, t.created_at, t.updated_at, b.name AS board_name
FROM social.todo_tasks t
JOIN social.todo_boards b ON b.id = t.board_id
WHERE b.group_id = $1
  AND ($2 = '' OR t.status = $2)
  AND ($3::uuid IS NULL OR t.board_id = $3)
ORDER BY t.created_at ASC
`

type ListTodoTasksByGroupParams struct {
	GroupID pgtype.UUID
	Status  string
	BoardID pgtype.UUID
}

type ListTodoTasksByGroupRow struct {
	ID             pgtype.UUID
	BoardID        pgtype.UUID
	Title          string
	Status         string
	AssigneeUserID pgtype.UUID
	DueDate        pgtype.Date
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
	BoardName      string
}

func (q *Queries) ListTodoTasksByGroup(ctx context.Context, arg ListTodoTasksByGroupParams) ([]ListTodoTasksByGroupRow, error) {
	rows, err := q.db.Query(ctx, listTodoTasksByGroup, arg.GroupID, arg.Status, arg.BoardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListTodoTasksByGroupRow
	for rows.Next() {
		var i ListTodoTasksByGroupRow
		if err := rows.Scan(
			&i.ID,
			&i.BoardID,
			&i.Title,
			&i.Status,
			&i.AssigneeUserID,
			&i.DueDate,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.BoardName,
		); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const updateTodoTask = `-- name: UpdateTodoTask :one
UPDATE social.todo_tasks SET
  title = $2,
  status = $3,
  assignee_user_id = $4,
  due_date = $5,
  updated_at = now()
WHERE id = $1
RETURNING id, board_id, title, status, assignee_user_id, due_date, created_at, updated_at
`

type UpdateTodoTaskParams struct {
	ID             pgtype.UUID
	Title          string
	Status         string
	AssigneeUserID pgtype.UUID
	DueDate        pgtype.Date
}

func (q *Queries) UpdateTodoTask(ctx context.Context, arg UpdateTodoTaskParams) (SocialTodoTask, error) {
	row := q.db.QueryRow(ctx, updateTodoTask,
		arg.ID,
		arg.Title,
		arg.Status,
		arg.AssigneeUserID,
		arg.DueDate,
	)
	var i SocialTodoTask
	err := row.Scan(
		&i.ID,
		&i.BoardID,
		&i.Title,
		&i.Status,
		&i.AssigneeUserID,
		&i.DueDate,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const deleteTodoTask = `-- name: DeleteTodoTask :one
DELETE FROM social.todo_tasks
WHERE id = $1
RETURNING id, board_id, title, status, assignee_user_id, due_date, created_at, updated_at
`

func (q *Queries) DeleteTodoTask(ctx context.Context, id pgtype.UUID) (SocialTodoTask, error) {
	row := q.db.QueryRow(ctx, deleteTodoTask, id)
	var i SocialTodoTask
	err := row.Scan(
		&i.ID,
		&i.BoardID,
		&i.Title,
		&i.Status,
		&i.AssigneeUserID,
		&i.DueDate,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}
