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

// AbsenceRepository es la única capa que toca SQL de academic.absences.
// La propiedad se resuelve vía subjects → curricula (user_id).
// Nota: el DDL no tiene UNIQUE(subject_id, date); el duplicado se detecta
// a nivel aplicación (ExistsOnDate → 409). Dos POST simultáneos de la misma
// fecha podrían colarse (sin constraint no hay garantía); se documenta.
type AbsenceRepository struct {
	pool *pgxpool.Pool
}

func NewAbsenceRepository(pool *pgxpool.Pool) *AbsenceRepository {
	return &AbsenceRepository{pool: pool}
}

const absenceColumns = `id, subject_id, date, justified, created_at`

func scanAbsence(row pgx.Row) (*model.Absence, error) {
	var a model.Absence
	if err := row.Scan(&a.ID, &a.SubjectID, &a.Date, &a.Justified, &a.CreatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}

func absenceSubjectOwned() string {
	return `EXISTS (
		SELECT 1 FROM academic.subjects s
		JOIN academic.curricula c ON c.id = s.curriculum_id
		WHERE s.id = $1::uuid AND c.user_id = $2
	)`
}

// Create inserta una falta solo si la materia es propia;
// nil si la materia no existe o es ajena.
func (r *AbsenceRepository) Create(ctx context.Context, userID, subjectID string, date time.Time, justified bool) (*model.Absence, error) {
	a, err := scanAbsence(r.pool.QueryRow(ctx, `
		INSERT INTO academic.absences (subject_id, date, justified)
		SELECT $1::uuid, $3::date, $4
		WHERE `+absenceSubjectOwned()+`
		RETURNING `+absenceColumns+`
	`, subjectID, userID, date.Format("2006-01-02"), justified))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create absence: %w", err)
	}
	return a, nil
}

// ExistsOnDate reporta si ya hay una falta propia en esa materia+fecha.
func (r *AbsenceRepository) ExistsOnDate(ctx context.Context, userID, subjectID string, date time.Time) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM academic.absences a
			JOIN academic.subjects s ON s.id = a.subject_id
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE a.subject_id = $1 AND c.user_id = $2 AND a.date = $3::date
		)
	`, subjectID, userID, date.Format("2006-01-02")).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check absence date: %w", err)
	}
	return exists, nil
}

// GetByID retorna la falta solo si su materia es propia.
func (r *AbsenceRepository) GetByID(ctx context.Context, id, userID string) (*model.Absence, error) {
	a, err := scanAbsence(r.pool.QueryRow(ctx, `
		SELECT `+qualify(absenceColumns, "a")+`
		FROM academic.absences a
		JOIN academic.subjects s ON s.id = a.subject_id
		JOIN academic.curricula c ON c.id = s.curriculum_id
		WHERE a.id = $1 AND c.user_id = $2
	`, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get absence: %w", err)
	}
	return a, nil
}

// ListBySubject lista las faltas ordenadas por fecha si la materia es propia.
// owned=false si la materia no existe o es ajena (service → 404).
func (r *AbsenceRepository) ListBySubject(ctx context.Context, subjectID, userID string) (list []model.Absence, owned bool, err error) {
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
		SELECT `+absenceColumns+`
		FROM academic.absences
		WHERE subject_id = $1
		ORDER BY date ASC, id ASC
	`, subjectID)
	if err != nil {
		return nil, true, fmt.Errorf("list absences: %w", err)
	}
	defer rows.Close()
	out := make([]model.Absence, 0)
	for rows.Next() {
		var a model.Absence
		if err := rows.Scan(&a.ID, &a.SubjectID, &a.Date, &a.Justified, &a.CreatedAt); err != nil {
			return nil, true, fmt.Errorf("list absences: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("list absences: %w", err)
	}
	return out, true, nil
}

// UpdateJustified cambia justified de una falta propia.
func (r *AbsenceRepository) UpdateJustified(ctx context.Context, id, userID string, justified bool) (*model.Absence, error) {
	a, err := scanAbsence(r.pool.QueryRow(ctx, `
		UPDATE academic.absences a
		SET justified = $3
		WHERE a.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = a.subject_id AND c.user_id = $2
		)
		RETURNING `+qualify(absenceColumns, "a")+`
	`, id, userID, justified))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update absence: %w", err)
	}
	return a, nil
}

// Delete borra una falta propia. False si no existe o es ajena.
func (r *AbsenceRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.absences a
		WHERE a.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = a.subject_id AND c.user_id = $2
		)
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete absence: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// CountsBySubject cuenta justificadas/injustificadas de una materia propia.
// owned=false si la materia no existe o es ajena.
func (r *AbsenceRepository) CountsBySubject(ctx context.Context, subjectID, userID string) (unjustified, justified int, owned bool, err error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = $1 AND c.user_id = $2
		)
	`, subjectID, userID).Scan(&exists); err != nil {
		return 0, 0, false, fmt.Errorf("check subject ownership: %w", err)
	}
	if !exists {
		return 0, 0, false, nil
	}
	err = r.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE justified = false),
		       count(*) FILTER (WHERE justified = true)
		FROM academic.absences
		WHERE subject_id = $1
	`, subjectID).Scan(&unjustified, &justified)
	if err != nil {
		return 0, 0, true, fmt.Errorf("count absences: %w", err)
	}
	return unjustified, justified, true, nil
}
