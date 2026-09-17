-- name: GetDailyHoursByTaskAndDate :one
SELECT * FROM social.sprint_sheet_daily_hours
WHERE task_id = $1 AND log_date = $2 LIMIT 1;

-- name: ListDailyHoursByTask :many
SELECT * FROM social.sprint_sheet_daily_hours
WHERE task_id = $1
  AND ($2::date IS NULL OR log_date >= $2)
  AND ($3::date IS NULL OR log_date <= $3)
ORDER BY log_date ASC;

-- name: CreateDailyHours :one
INSERT INTO social.sprint_sheet_daily_hours (task_id, log_date, hours)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateDailyHours :one
UPDATE social.sprint_sheet_daily_hours SET
  hours = $2
WHERE id = $1
RETURNING *;
