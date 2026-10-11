package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
)

// ScheduleRepository es la única capa que toca SQL de academic.schedules.
// La propiedad se resuelve vía subjects → curricula (user_id).
type ScheduleRepository struct {
	pool *pgxpool.Pool
}

func NewScheduleRepository(pool *pgxpool.Pool) *ScheduleRepository {
	return &ScheduleRepository{pool: pool}
}

const scheduleColumns = `id, subject_id, day_of_week, start_time, end_time`

func scanSchedule(row pgx.Row) (*model.Schedule, error) {
	var s model.Schedule
	if err := row.Scan(&s.ID, &s.SubjectID, &s.DayOfWeek, &s.StartTime, &s.EndTime); err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserta un horario solo si la materia es propia (atómico);
// nil si la materia no existe o es ajena.
func (r *ScheduleRepository) Create(ctx context.Context, userID, subjectID string, dayOfWeek int, start, end time.Time) (*model.Schedule, error) {
	s, err := scanSchedule(r.pool.QueryRow(ctx, `
		INSERT INTO academic.schedules (subject_id, day_of_week, start_time, end_time)
		SELECT $1::uuid, $2, $3, $4
		WHERE EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = $1::uuid AND c.user_id = $5
		)
		RETURNING `+scheduleColumns+`
	`, subjectID, dayOfWeek, start, end, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create schedule: %w", err)
	}
	return s, nil
}

// ListBySubject lista los horarios si la materia es propia.
// owned=false si la materia no existe o es ajena (service → 404).
func (r *ScheduleRepository) ListBySubject(ctx context.Context, subjectID, userID string) (list []model.Schedule, owned bool, err error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = $1 AND c.user_id = $2
		)
	`, subjectID, userID).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("check subject ownership: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+scheduleColumns+`
		FROM academic.schedules
		WHERE subject_id = $1
		ORDER BY day_of_week ASC, start_time ASC, id ASC
	`, subjectID)
	if err != nil {
		return nil, true, fmt.Errorf("list schedules: %w", err)
	}
	defer rows.Close()
	out := make([]model.Schedule, 0)
	for rows.Next() {
		var s model.Schedule
		if err := rows.Scan(&s.ID, &s.SubjectID, &s.DayOfWeek, &s.StartTime, &s.EndTime); err != nil {
			return nil, true, fmt.Errorf("list schedules: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("list schedules: %w", err)
	}
	return out, true, nil
}

// Delete borra un horario propio. False si no existe o es ajeno.
func (r *ScheduleRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.schedules h
		WHERE h.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = h.subject_id AND c.user_id = $2
		)
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete schedule: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
