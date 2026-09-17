package sqlc

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

const getDailyHoursByTaskAndDate = `-- name: GetDailyHoursByTaskAndDate :one
SELECT id, task_id, log_date, hours FROM social.sprint_sheet_daily_hours
WHERE task_id = $1 AND log_date = $2 LIMIT 1
`

type GetDailyHoursByTaskAndDateParams struct {
	TaskID  pgtype.UUID
	LogDate pgtype.Date
}

func (q *Queries) GetDailyHoursByTaskAndDate(ctx context.Context, arg GetDailyHoursByTaskAndDateParams) (SocialSprintSheetDailyHour, error) {
	row := q.db.QueryRow(ctx, getDailyHoursByTaskAndDate, arg.TaskID, arg.LogDate)
	var i SocialSprintSheetDailyHour
	err := row.Scan(
		&i.ID,
		&i.TaskID,
		&i.LogDate,
		&i.Hours,
	)
	return i, err
}

const listDailyHoursByTask = `-- name: ListDailyHoursByTask :many
SELECT id, task_id, log_date, hours FROM social.sprint_sheet_daily_hours
WHERE task_id = $1
  AND ($2::date IS NULL OR log_date >= $2)
  AND ($3::date IS NULL OR log_date <= $3)
ORDER BY log_date ASC
`

type ListDailyHoursByTaskParams struct {
	TaskID pgtype.UUID
	From   pgtype.Date
	To     pgtype.Date
}

func (q *Queries) ListDailyHoursByTask(ctx context.Context, arg ListDailyHoursByTaskParams) ([]SocialSprintSheetDailyHour, error) {
	rows, err := q.db.Query(ctx, listDailyHoursByTask, arg.TaskID, arg.From, arg.To)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []SocialSprintSheetDailyHour
	for rows.Next() {
		var i SocialSprintSheetDailyHour
		if err := rows.Scan(
			&i.ID,
			&i.TaskID,
			&i.LogDate,
			&i.Hours,
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

const createDailyHours = `-- name: CreateDailyHours :one
INSERT INTO social.sprint_sheet_daily_hours (task_id, log_date, hours)
VALUES ($1, $2, $3)
RETURNING id, task_id, log_date, hours
`

type CreateDailyHoursParams struct {
	TaskID  pgtype.UUID
	LogDate pgtype.Date
	Hours   pgtype.Numeric
}

func (q *Queries) CreateDailyHours(ctx context.Context, arg CreateDailyHoursParams) (SocialSprintSheetDailyHour, error) {
	row := q.db.QueryRow(ctx, createDailyHours, arg.TaskID, arg.LogDate, arg.Hours)
	var i SocialSprintSheetDailyHour
	err := row.Scan(
		&i.ID,
		&i.TaskID,
		&i.LogDate,
		&i.Hours,
	)
	return i, err
}

const updateDailyHours = `-- name: UpdateDailyHours :one
UPDATE social.sprint_sheet_daily_hours SET
  hours = $2
WHERE id = $1
RETURNING id, task_id, log_date, hours
`

type UpdateDailyHoursParams struct {
	ID    pgtype.UUID
	Hours pgtype.Numeric
}

func (q *Queries) UpdateDailyHours(ctx context.Context, arg UpdateDailyHoursParams) (SocialSprintSheetDailyHour, error) {
	row := q.db.QueryRow(ctx, updateDailyHours, arg.ID, arg.Hours)
	var i SocialSprintSheetDailyHour
	err := row.Scan(
		&i.ID,
		&i.TaskID,
		&i.LogDate,
		&i.Hours,
	)
	return i, err
}
