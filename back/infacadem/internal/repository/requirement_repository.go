package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
)

// RequirementRepository es la única capa que toca SQL de
// academic.grade_requirements.
type RequirementRepository struct {
	pool *pgxpool.Pool
}

func NewRequirementRepository(pool *pgxpool.Pool) *RequirementRepository {
	return &RequirementRepository{pool: pool}
}

const requirementColumns = `id, subject_id, source_grade_id, checks_final_average, comparator, threshold_value, action, created_at`

func scanRequirement(row pgx.Row) (*model.Requirement, error) {
	var r model.Requirement
	if err := row.Scan(&r.ID, &r.SubjectID, &r.SourceGradeID, &r.ChecksFinalAverage,
		&r.Comparator, &r.ThresholdValue, &r.Action, &r.CreatedAt); err != nil {
		return nil, err
	}
	return &r, nil
}

// Create inserta un requisito solo si la materia es propia; nil si no existe o es ajena.
func (r *RequirementRepository) Create(ctx context.Context, userID, subjectID string, sourceGradeID *string, checksFinal bool, comparator string, threshold float64, action string) (*model.Requirement, error) {
	var source any
	if sourceGradeID != nil {
		source = *sourceGradeID
	}
	out, err := scanRequirement(r.pool.QueryRow(ctx, `
		INSERT INTO academic.grade_requirements (subject_id, source_grade_id, checks_final_average, comparator, threshold_value, action)
		SELECT $1::uuid, $3::uuid, $4, $5, $6, $7
		WHERE EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = $1::uuid AND c.user_id = $2
		)
		RETURNING `+requirementColumns+`
	`, subjectID, userID, source, checksFinal, comparator, threshold, action))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create requirement: %w", err)
	}
	return out, nil
}

// ListBySubject lista los requisitos si la materia es propia.
func (r *RequirementRepository) ListBySubject(ctx context.Context, subjectID, userID string) (list []model.Requirement, owned bool, err error) {
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
		SELECT `+requirementColumns+`
		FROM academic.grade_requirements
		WHERE subject_id = $1
		ORDER BY created_at ASC, id ASC
	`, subjectID)
	if err != nil {
		return nil, true, fmt.Errorf("list requirements: %w", err)
	}
	defer rows.Close()
	out := make([]model.Requirement, 0)
	for rows.Next() {
		var req model.Requirement
		if err := rows.Scan(&req.ID, &req.SubjectID, &req.SourceGradeID, &req.ChecksFinalAverage,
			&req.Comparator, &req.ThresholdValue, &req.Action, &req.CreatedAt); err != nil {
			return nil, true, fmt.Errorf("list requirements: %w", err)
		}
		out = append(out, req)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("list requirements: %w", err)
	}
	return out, true, nil
}

// Delete borra un requisito propio. False si no existe o es ajeno.
func (r *RequirementRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.grade_requirements q
		WHERE q.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = q.subject_id AND c.user_id = $2
		)
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete requirement: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// GradeSubjectOf verifica que una nota pertenezca a una materia propia.
// Retorna ("", false) si la nota no existe, es de otra materia o es ajena
// (el service lo mapea a 400, no 404: el id vino como referencia, no como recurso).
func (r *RequirementRepository) GradeSubjectOf(ctx context.Context, gradeID, subjectID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM academic.grades g
			JOIN academic.subjects s ON s.id = g.subject_id
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE g.id = $1 AND g.subject_id = $2 AND c.user_id = $3
		)
	`, gradeID, subjectID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check grade subject: %w", err)
	}
	return ok, nil
}
