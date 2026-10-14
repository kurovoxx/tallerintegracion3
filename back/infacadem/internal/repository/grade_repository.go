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

// GradeRepository es la única capa que toca SQL de academic.grades.
// La propiedad se resuelve vía subjects → curricula (user_id):
// recurso ajeno = invisible (nil), nunca error distinguible.
type GradeRepository struct {
	pool *pgxpool.Pool
}

func NewGradeRepository(pool *pgxpool.Pool) *GradeRepository {
	return &GradeRepository{pool: pool}
}

// GradeCreate agrupa el alta (parent nil = nota tope de la materia).
type GradeCreate struct {
	ParentGradeID         *string
	IsFinalExam           bool
	Status                string
	Name                  string
	Score                 *int
	Weight                float64
	ScheduledDate         *time.Time
	Topics                *string
	NotifyDiscord         bool
	GoogleCalendarEventID *string
}

// GradePatch agrupa actualización parcial (nil = no tocar).
type GradePatch struct {
	Name                  *string
	Score                 *int
	Weight                *float64
	Status                *string
	ScheduledDate         *time.Time
	Topics                *string
	NotifyDiscord         *bool
	GoogleCalendarEventID *string
	IsFinalExam           *bool
	Pinned                *bool
	// Clear* piden NULL explícito.
	ClearScore       bool
	ClearScheduledAt bool
	ClearTopics      bool
	ClearCalendarEvt bool
}

const gradeColumns = `id, subject_id, parent_grade_id, is_final_exam, status, name, score, weight, scheduled_date, topics, notify_discord, google_calendar_event_id, pinned, created_at`

func scanGrade(row pgx.Row) (*model.Grade, error) {
	var g model.Grade
	if err := row.Scan(&g.ID, &g.SubjectID, &g.ParentGradeID, &g.IsFinalExam, &g.Status,
		&g.Name, &g.Score, &g.Weight, &g.ScheduledDate, &g.Topics, &g.NotifyDiscord,
		&g.GoogleCalendarEventID, &g.Pinned, &g.CreatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}

func subjectOwned() string {
	return `EXISTS (
		SELECT 1 FROM academic.subjects s
		JOIN academic.curricula c ON c.id = s.curriculum_id
		WHERE s.id = $1::uuid AND c.user_id = $2
	)`
}

// Create inserta una nota solo si la materia es propia (atómico);
// nil si la materia no existe o es ajena.
func (r *GradeRepository) Create(ctx context.Context, userID, subjectID string, in GradeCreate) (*model.Grade, error) {
	var score, sched, topics, event, parent any
	if in.Score != nil {
		score = *in.Score
	}
	if in.ScheduledDate != nil {
		sched = *in.ScheduledDate
	}
	if in.Topics != nil {
		topics = *in.Topics
	}
	if in.GoogleCalendarEventID != nil {
		event = *in.GoogleCalendarEventID
	}
	if in.ParentGradeID != nil {
		parent = *in.ParentGradeID
	}
	g, err := scanGrade(r.pool.QueryRow(ctx, `
		INSERT INTO academic.grades (subject_id, parent_grade_id, is_final_exam, status, name, score, weight, scheduled_date, topics, notify_discord, google_calendar_event_id)
		SELECT $1::uuid, $3::uuid, $4, $5, $6, $7, $8, $9, $10, $11, $12
		WHERE `+subjectOwned()+`
		RETURNING `+gradeColumns+`
	`, subjectID, userID, parent, in.IsFinalExam, in.Status, in.Name, score, in.Weight, sched, topics, in.NotifyDiscord, event))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("create grade: %w", err)
	}
	return g, nil
}

// GetByID retorna la nota solo si su materia es propia.
func (r *GradeRepository) GetByID(ctx context.Context, id, userID string) (*model.Grade, error) {
	g, err := scanGrade(r.pool.QueryRow(ctx, `
		SELECT `+qualify(gradeColumns, "g")+`
		FROM academic.grades g
		JOIN academic.subjects s ON s.id = g.subject_id
		JOIN academic.curricula c ON c.id = s.curriculum_id
		WHERE g.id = $1 AND c.user_id = $2
	`, id, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get grade: %w", err)
	}
	return g, nil
}

// ListBySubject lista todas las notas de una materia propia (plano:
// el service agrupa hijos por parent_grade_id).
// owned=false si la materia no existe o es ajena (service → 404).
func (r *GradeRepository) ListBySubject(ctx context.Context, subjectID, userID string) (list []model.Grade, owned bool, err error) {
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
		SELECT `+gradeColumns+`
		FROM academic.grades
		WHERE subject_id = $1
		ORDER BY created_at ASC, id ASC
	`, subjectID)
	if err != nil {
		return nil, true, fmt.Errorf("list grades: %w", err)
	}
	defer rows.Close()
	out := make([]model.Grade, 0)
	for rows.Next() {
		var g model.Grade
		if err := rows.Scan(&g.ID, &g.SubjectID, &g.ParentGradeID, &g.IsFinalExam, &g.Status,
			&g.Name, &g.Score, &g.Weight, &g.ScheduledDate, &g.Topics, &g.NotifyDiscord,
			&g.GoogleCalendarEventID, &g.Pinned, &g.CreatedAt); err != nil {
			return nil, true, fmt.Errorf("list grades: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("list grades: %w", err)
	}
	return out, true, nil
}

// Update aplica PATCH parcial solo si la materia es propia.
func (r *GradeRepository) Update(ctx context.Context, id, userID string, p GradePatch) (*model.Grade, error) {
	set := make([]string, 0, 9)
	args := []any{id, userID}
	add := func(expr string, v any) {
		args = append(args, v)
		set = append(set, fmt.Sprintf(expr, len(args)))
	}
	if p.Name != nil {
		add("name = $%d", *p.Name)
	}
	if p.ClearScore {
		set = append(set, "score = NULL")
	} else if p.Score != nil {
		add("score = $%d", *p.Score)
	}
	if p.Weight != nil {
		add("weight = $%d", *p.Weight)
	}
	if p.Status != nil {
		add("status = $%d", *p.Status)
	}
	if p.ClearScheduledAt {
		set = append(set, "scheduled_date = NULL")
	} else if p.ScheduledDate != nil {
		add("scheduled_date = $%d", *p.ScheduledDate)
	}
	if p.ClearTopics {
		set = append(set, "topics = NULL")
	} else if p.Topics != nil {
		add("topics = $%d", *p.Topics)
	}
	if p.NotifyDiscord != nil {
		add("notify_discord = $%d", *p.NotifyDiscord)
	}
	if p.ClearCalendarEvt {
		set = append(set, "google_calendar_event_id = NULL")
	} else if p.GoogleCalendarEventID != nil {
		add("google_calendar_event_id = $%d", *p.GoogleCalendarEventID)
	}
	if p.IsFinalExam != nil {
		add("is_final_exam = $%d", *p.IsFinalExam)
	}
	if p.Pinned != nil {
		add("pinned = $%d", *p.Pinned)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}
	// Pin nuevo despinnea al resto de la materia en la misma sentencia
	// (un solo pin por materia, atómico): solo cuando se fija pinned=true.
	unpin := ""
	if p.Pinned != nil && *p.Pinned {
		unpin = `WITH sib AS (
			UPDATE academic.grades g
			SET pinned = false
			WHERE g.pinned = true AND g.id <> $1
			  AND g.subject_id = (SELECT subject_id FROM academic.grades WHERE id = $1)
			  AND EXISTS (
				SELECT 1 FROM academic.subjects s
				JOIN academic.curricula c ON c.id = s.curriculum_id
				WHERE s.id = g.subject_id AND c.user_id = $2
			)
		) `
	}
	query := fmt.Sprintf(unpin+`
		UPDATE academic.grades g
		SET %s
		WHERE g.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = g.subject_id AND c.user_id = $2
		)
		RETURNING `+qualify(gradeColumns, "g"), joinSet(set))
	g, err := scanGrade(r.pool.QueryRow(ctx, query, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update grade: %w", err)
	}
	return g, nil
}

// Delete borra una nota propia (hijos caen por CASCADE).
// False si no existe o es ajena.
func (r *GradeRepository) Delete(ctx context.Context, id, userID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM academic.grades g
		WHERE g.id = $1 AND EXISTS (
			SELECT 1 FROM academic.subjects s
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE s.id = g.subject_id AND c.user_id = $2
		)
	`, id, userID)
	if err != nil {
		return false, fmt.Errorf("delete grade: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// CountFinalExams cuenta finales de la materia propia (excludeID omite una
// nota, para validar PATCH sin chocar consigo misma).
// Nota: con excludeID vacío se usa una query sin filtro de exclusión.
// (Comparar $3=” con casteo a uuid es frágil: ” no es uuid válido y
// Postgres puede evaluarlo → 22P02. Rama explícita en Go, sin trucos SQL.)
func (r *GradeRepository) CountFinalExams(ctx context.Context, subjectID, userID, excludeID string) (int, error) {
	var n int
	var err error
	if excludeID == "" {
		err = r.pool.QueryRow(ctx, `
			SELECT count(*)
			FROM academic.grades g
			JOIN academic.subjects s ON s.id = g.subject_id
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE g.subject_id = $1 AND c.user_id = $2
			  AND g.is_final_exam = true
		`, subjectID, userID).Scan(&n)
	} else {
		err = r.pool.QueryRow(ctx, `
			SELECT count(*)
			FROM academic.grades g
			JOIN academic.subjects s ON s.id = g.subject_id
			JOIN academic.curricula c ON c.id = s.curriculum_id
			WHERE g.subject_id = $1 AND c.user_id = $2
			  AND g.is_final_exam = true
			  AND g.id <> $3::uuid
		`, subjectID, userID, excludeID).Scan(&n)
	}
	if err != nil {
		return 0, fmt.Errorf("count finals: %w", err)
	}
	return n, nil
}
