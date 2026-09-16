package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getSheetByID = `-- name: GetSheetByID :one
SELECT id, group_id, name, period_start, period_end, created_at FROM social.sprint_sheets
WHERE id = $1 LIMIT 1
`

func (q *Queries) GetSheetByID(ctx context.Context, id pgtype.UUID) (SocialSprintSheet, error) {
	row := q.db.QueryRow(ctx, getSheetByID, id)
	var i SocialSprintSheet
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Name,
		&i.PeriodStart,
		&i.PeriodEnd,
		&i.CreatedAt,
	)
	return i, err
}

const listSheetsByGroup = `-- name: ListSheetsByGroup :many
SELECT id, group_id, name, period_start, period_end, created_at FROM social.sprint_sheets
WHERE group_id = $1
ORDER BY created_at ASC
`

func (q *Queries) ListSheetsByGroup(ctx context.Context, groupID pgtype.UUID) ([]SocialSprintSheet, error) {
	rows, err := q.db.Query(ctx, listSheetsByGroup, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SocialSprintSheet
	for rows.Next() {
		var i SocialSprintSheet
		if err := rows.Scan(
			&i.ID,
			&i.GroupID,
			&i.Name,
			&i.PeriodStart,
			&i.PeriodEnd,
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

const createSheet = `-- name: CreateSheet :one
INSERT INTO social.sprint_sheets (group_id, name)
VALUES ($1, $2)
RETURNING id, group_id, name, period_start, period_end, created_at
`

type CreateSheetParams struct {
	GroupID pgtype.UUID
	Name    string
}

func (q *Queries) CreateSheet(ctx context.Context, arg CreateSheetParams) (SocialSprintSheet, error) {
	row := q.db.QueryRow(ctx, createSheet, arg.GroupID, arg.Name)
	var i SocialSprintSheet
	err := row.Scan(
		&i.ID,
		&i.GroupID,
		&i.Name,
		&i.PeriodStart,
		&i.PeriodEnd,
		&i.CreatedAt,
	)
	return i, err
}

const getSprintSheetTaskByID = `-- name: GetSprintSheetTaskByID :one
SELECT id, sheet_id, assignee_user_id, title, priority, status, estimated_hours, created_at, updated_at FROM social.sprint_sheet_tasks
WHERE id = $1 LIMIT 1
`

func (q *Queries) GetSprintSheetTaskByID(ctx context.Context, id pgtype.UUID) (SocialSprintSheetTask, error) {
	row := q.db.QueryRow(ctx, getSprintSheetTaskByID, id)
	var i SocialSprintSheetTask
	err := row.Scan(
		&i.ID,
		&i.SheetID,
		&i.AssigneeUserID,
		&i.Title,
		&i.Priority,
		&i.Status,
		&i.EstimatedHours,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const createSprintSheetTask = `-- name: CreateSprintSheetTask :one
INSERT INTO social.sprint_sheet_tasks (
  sheet_id, assignee_user_id, title, priority, status, estimated_hours
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING id, sheet_id, assignee_user_id, title, priority, status, estimated_hours, created_at, updated_at
`

type CreateSprintSheetTaskParams struct {
	SheetID        pgtype.UUID
	AssigneeUserID pgtype.UUID
	Title          string
	Priority       string
	Status         string
	EstimatedHours pgtype.Numeric
}

func (q *Queries) CreateSprintSheetTask(ctx context.Context, arg CreateSprintSheetTaskParams) (SocialSprintSheetTask, error) {
	row := q.db.QueryRow(ctx, createSprintSheetTask,
		arg.SheetID,
		arg.AssigneeUserID,
		arg.Title,
		arg.Priority,
		arg.Status,
		arg.EstimatedHours,
	)
	var i SocialSprintSheetTask
	err := row.Scan(
		&i.ID,
		&i.SheetID,
		&i.AssigneeUserID,
		&i.Title,
		&i.Priority,
		&i.Status,
		&i.EstimatedHours,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const listSprintSheetTasksByGroup = `-- name: ListSprintSheetTasksByGroup :many
SELECT t.id, t.sheet_id, t.assignee_user_id, t.title, t.priority, t.status, t.estimated_hours, t.created_at, t.updated_at, s.name AS sheet_name
FROM social.sprint_sheet_tasks t
JOIN social.sprint_sheets s ON s.id = t.sheet_id
WHERE s.group_id = $1
  AND ($2 = '' OR t.status = $2)
  AND ($3 = '' OR t.priority = $3)
  AND ($4::uuid IS NULL OR t.sheet_id = $4)
ORDER BY t.created_at ASC
`

type ListSprintSheetTasksByGroupParams struct {
	GroupID  pgtype.UUID
	Status   string
	Priority string
	SheetID  pgtype.UUID
}

type ListSprintSheetTasksByGroupRow struct {
	ID             pgtype.UUID
	SheetID        pgtype.UUID
	AssigneeUserID pgtype.UUID
	Title          string
	Priority       string
	Status         string
	EstimatedHours pgtype.Numeric
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
	SheetName      string
}

func (q *Queries) ListSprintSheetTasksByGroup(ctx context.Context, arg ListSprintSheetTasksByGroupParams) ([]ListSprintSheetTasksByGroupRow, error) {
	rows, err := q.db.Query(ctx, listSprintSheetTasksByGroup, arg.GroupID, arg.Status, arg.Priority, arg.SheetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ListSprintSheetTasksByGroupRow
	for rows.Next() {
		var i ListSprintSheetTasksByGroupRow
		if err := rows.Scan(
			&i.ID,
			&i.SheetID,
			&i.AssigneeUserID,
			&i.Title,
			&i.Priority,
			&i.Status,
			&i.EstimatedHours,
			&i.CreatedAt,
			&i.UpdatedAt,
			&i.SheetName,
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

const updateSprintSheetTask = `-- name: UpdateSprintSheetTask :one
UPDATE social.sprint_sheet_tasks SET
  title = $2,
  assignee_user_id = $3,
  priority = $4,
  status = $5,
  estimated_hours = $6,
  updated_at = now()
WHERE id = $1
RETURNING id, sheet_id, assignee_user_id, title, priority, status, estimated_hours, created_at, updated_at
`

type UpdateSprintSheetTaskParams struct {
	ID             pgtype.UUID
	Title          string
	AssigneeUserID pgtype.UUID
	Priority       string
	Status         string
	EstimatedHours pgtype.Numeric
}

func (q *Queries) UpdateSprintSheetTask(ctx context.Context, arg UpdateSprintSheetTaskParams) (SocialSprintSheetTask, error) {
	row := q.db.QueryRow(ctx, updateSprintSheetTask,
		arg.ID,
		arg.Title,
		arg.AssigneeUserID,
		arg.Priority,
		arg.Status,
		arg.EstimatedHours,
	)
	var i SocialSprintSheetTask
	err := row.Scan(
		&i.ID,
		&i.SheetID,
		&i.AssigneeUserID,
		&i.Title,
		&i.Priority,
		&i.Status,
		&i.EstimatedHours,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}

const deleteSprintSheetTask = `-- name: DeleteSprintSheetTask :one
DELETE FROM social.sprint_sheet_tasks
WHERE id = $1
RETURNING id, sheet_id, assignee_user_id, title, priority, status, estimated_hours, created_at, updated_at
`

func (q *Queries) DeleteSprintSheetTask(ctx context.Context, id pgtype.UUID) (SocialSprintSheetTask, error) {
	row := q.db.QueryRow(ctx, deleteSprintSheetTask, id)
	var i SocialSprintSheetTask
	err := row.Scan(
		&i.ID,
		&i.SheetID,
		&i.AssigneeUserID,
		&i.Title,
		&i.Priority,
		&i.Status,
		&i.EstimatedHours,
		&i.CreatedAt,
		&i.UpdatedAt,
	)
	return i, err
}
