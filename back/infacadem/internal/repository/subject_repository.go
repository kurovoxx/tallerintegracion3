package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
)

// SubjectRepository es la única capa que toca SQL de academic.subjects.
// La propiedad se resuelve vía JOIN a academic.curricula (user_id):
// recurso ajeno = invisible (nil), nunca error distinguible.
type SubjectRepository struct {
	pool *pgxpool.Pool
}

func NewSubjectRepository(pool *pgxpool.Pool) *SubjectRepository {
	return &SubjectRepository{pool: pool}
}

// SubjectCreate agrupa los campos de alta (name + opcionales).
type SubjectCreate struct {
	Name                 string
	Semester             *int
	Credits              *int
	TotalClasses         *int
	MinAttendancePct     *float64
	AverageDecimalPlaces int
	Status               string
}

// SubjectPatch agrupa actualización parcial (nil = no tocar).
type SubjectPatch struct {
	Name                 *string
	Semester             *int
	Credits              *int
	TotalClasses         *int
	MinAttendancePct     *float64
	AverageDecimalPlaces *int
	Status               *string
	// ClearSemester/ClearCredits/ClearTotalClasses/ClearAttendance piden
	// NULL explícito (el service los deriva de "" o null JSON).
	ClearSemester     bool
	ClearCredits      bool
	ClearTotalClasses bool
	ClearAttendance   bool
}

const subjectColumns = `id, curriculum_id, name, semester, credits, total_classes, min_attendance_pct, average_decimal_places, status`

func scanSubject(row pgx.Row) (*model.Subject, error) {
	var s model.Subject
	if err := row.Scan(&s.ID, &s.CurriculumID, &s.Name, &s.Semester, &s.Credits,
		&s.TotalClasses, &s.MinAttendancePct, &s.AverageDecimalPlaces, &s.Status); err != nil {
		return nil, err
	}
	return &s, nil
}

// Create inserta una materia solo si la malla es propia (atómico vía
// INSERT..SELECT..WHERE); nil si la malla no existe o es ajena.
func (r *SubjectRepository) Create(ctx context.Context, userID, curriculumID string, in SubjectCreate) (*model.Subject, error) {
	var sem, cred, tot any
	if in.Semester != nil {
		sem = *in.Semester
	}
	if in.Credits != nil {
		cred = *in.Credits
	}
	if in.TotalClasses != nil {
		tot = *in.TotalClasses
	}
	var att any
	if in.MinAttendancePct != nil {
		att = *in.MinAttendancePct
	}
	s, err := scanSubject(r.pool.QueryRow(ctx, `
		INSERT INTO academic.subjects (curriculum_id, name, semester, credits, total_classes, min_attendance_pct, average_decimal_places, status)
		SELECT $1::uuid, $3, $4, $5, $6, $7, $8, $9
		WHERE EXISTS (SELECT 1 FROM academic.curricula WHERE id = $1::uuid AND user_id = $2)
		RETURNING `+subjectColumns+`
	`, curriculumID, userID, in.Name, sem, cred, tot, att, in.AverageDecimalPlaces, in.Status))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create subject: %w", err)
	}
	return s, nil
}

// GetByID retorna la materia solo si su malla es propia.
func (r *SubjectRepository) GetByID(ctx context.Context, id, userID string) (*model.Subject, error) {
	s, err := scanSubject(r.pool.QueryRow(ctx, `
		SELECT s.`+strings.ReplaceAll(subjectColumns, ", ", ", s.")+`
		FROM academic.subjects s
		JOIN academic.curricula c ON c.id = s.curriculum_id
		WHERE s.id = $1 AND c.user_id = $2
	`, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get subject: %w", err)
	}
	return s, nil
}

// ListByCurriculum lista las materias si la malla es propia.
// owned=false si la malla no existe o es ajena (service → 404).
func (r *SubjectRepository) ListByCurriculum(ctx context.Context, curriculumID, userID string) (list []model.Subject, owned bool, err error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM academic.curricula WHERE id = $1 AND user_id = $2)
	`, curriculumID, userID).Scan(&exists); err != nil {
		return nil, false, fmt.Errorf("check curriculum ownership: %w", err)
	}
	if !exists {
		return nil, false, nil
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+subjectColumns+`
		FROM academic.subjects
		WHERE curriculum_id = $1
		ORDER BY name ASC, id ASC
	`, curriculumID)
	if err != nil {
		return nil, true, fmt.Errorf("list subjects: %w", err)
	}
	defer rows.Close()
	out := make([]model.Subject, 0)
	for rows.Next() {
		var s model.Subject
		if err := rows.Scan(&s.ID, &s.CurriculumID, &s.Name, &s.Semester, &s.Credits,
			&s.TotalClasses, &s.MinAttendancePct, &s.AverageDecimalPlaces, &s.Status); err != nil {
			return nil, true, fmt.Errorf("list subjects: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("list subjects: %w", err)
	}
	return out, true, nil
}

// Update aplica PATCH parcial solo si la malla es propia; nil si no existe o es ajena.
func (r *SubjectRepository) Update(ctx context.Context, id, userID string, p SubjectPatch) (*model.Subject, error) {
	set := make([]string, 0, 7)
	args := []any{id, userID}
	add := func(expr string, v any) {
		args = append(args, v)
		set = append(set, fmt.Sprintf(expr, len(args)))
	}
	if p.Name != nil {
		add("name = $%d", *p.Name)
	}
	if p.ClearSemester {
		set = append(set, "semester = NULL")
	} else if p.Semester != nil {
		add("semester = $%d", *p.Semester)
	}
	if p.ClearCredits {
		set = append(set, "credits = NULL")
	} else if p.Credits != nil {
		add("credits = $%d", *p.Credits)
	}
	if p.ClearTotalClasses {
		set = append(set, "total_classes = NULL")
	} else if p.TotalClasses != nil {
		add("total_classes = $%d", *p.TotalClasses)
	}
	if p.ClearAttendance {
		set = append(set, "min_attendance_pct = NULL")
	} else if p.MinAttendancePct != nil {
		add("min_attendance_pct = $%d", *p.MinAttendancePct)
	}
	if p.AverageDecimalPlaces != nil {
		add("average_decimal_places = $%d", *p.AverageDecimalPlaces)
	}
	if p.Status != nil {
		add("status = $%d", *p.Status)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	query := fmt.Sprintf(`
		UPDATE academic.subjects s
		SET %s
		WHERE s.id = $1 AND EXISTS (
			SELECT 1 FROM academic.curricula c
			WHERE c.id = s.curriculum_id AND c.user_id = $2
		)
		RETURNING s.`+strings.ReplaceAll(subjectColumns, ", ", ", s."), strings.Join(set, ", "))
	s, err := scanSubject(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update subject: %w", err)
	}
	return s, nil
}

// Delete borra una materia propia (schedules caen por CASCADE).
// False si no existe o su malla es ajena.
func (r *SubjectRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.subjects s
		WHERE s.id = $1 AND EXISTS (
			SELECT 1 FROM academic.curricula c
			WHERE c.id = s.curriculum_id AND c.user_id = $2
		)
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete subject: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
