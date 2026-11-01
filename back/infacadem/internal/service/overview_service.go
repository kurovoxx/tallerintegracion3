package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// CurriculumLister lista solo mallas propias (orden creación desc).
type CurriculumLister interface {
	List(ctx context.Context, userID string) ([]model.Curriculum, error)
}

// SubjectCounter agrega conteos por malla en una sola query.
type SubjectCounter interface {
	CountsByCurricula(ctx context.Context, userID string) (map[string]repository.SubjectCounts, error)
}

// CurriculumOverview es una malla con sus conteos (vista /academic/overview).
type CurriculumOverview struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	SubjectsCount   int    `json:"subjects_count"`
	ApprovedCount   int    `json:"approved_count"`
	InProgressCount int    `json:"in_progress_count"`
}

// OverviewStats son los totales propios.
type OverviewStats struct {
	CurriculaCount int `json:"curricula_count"`
	SubjectsCount  int `json:"subjects_count"`
}

// Overview es la respuesta de GET /academic/overview.
type Overview struct {
	Curricula []CurriculumOverview `json:"curricula"`
	Stats     OverviewStats        `json:"stats"`
}

// OverviewService compone CurriculumService + conteo agregado.
// Sin N+1: 1 query de mallas + 1 GROUP BY de conteos.
type OverviewService struct {
	curricula CurriculumLister
	subjects  SubjectCounter
}

func NewOverviewService(curricula CurriculumLister, subjects SubjectCounter) *OverviewService {
	return &OverviewService{curricula: curricula, subjects: subjects}
}

// Get arma el overview solo con datos propios (orden creación desc,
// heredado del listado). Listas nunca null.
func (s *OverviewService) Get(ctx context.Context, userID string) (*Overview, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	uid := strings.TrimSpace(userID)
	mallas, err := s.curricula.List(ctx, uid)
	if err != nil {
		return nil, err
	}
	counts, err := s.subjects.CountsByCurricula(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := &Overview{Curricula: make([]CurriculumOverview, 0, len(mallas))}
	for _, m := range mallas {
		c := counts[m.ID]
		out.Curricula = append(out.Curricula, CurriculumOverview{
			ID: m.ID, Name: m.Name,
			SubjectsCount: c.Total, ApprovedCount: c.Approved, InProgressCount: c.InProgress,
		})
		out.Stats.CurriculaCount++
		out.Stats.SubjectsCount += c.Total
	}
	return out, nil
}

// Dependencias mínimas del detalle (satisfechas por los services existentes).

type SubjectGetter interface {
	Get(ctx context.Context, userID, id string) (*model.Subject, error)
}

type ScheduleLister interface {
	List(ctx context.Context, userID, subjectID string) ([]model.Schedule, error)
}

type GradeLister interface {
	List(ctx context.Context, userID, subjectID string) ([]GradeNode, error)
}

type Averager interface {
	Average(ctx context.Context, userID, subjectID string) (*AverageResult, error)
}

type Attender interface {
	Summary(ctx context.Context, userID, subjectID string) (*AttendanceSummary, error)
}

// SubjectDetail es la respuesta de GET /subjects/{id}/detail: misma forma
// que las rutas individuales (subject directo, listas [], average y
// attendance tal cual). Solo lectura.
type SubjectDetail struct {
	Subject    model.Subject      `json:"subject"`
	Schedules  []model.Schedule   `json:"schedules"`
	Grades     []GradeNode        `json:"grades"`
	Average    *AverageResult     `json:"average"`
	Attendance *AttendanceSummary `json:"attendance"`
}

// DetailService compone los 5 services existentes sin SQL nuevo.
// Son ~7 queries indexadas por PK (sin N+1: ninguna por ítem),
// el mismo costo que las 5 llamadas individuales que reemplaza.
type DetailService struct {
	subjects  SubjectGetter
	schedules ScheduleLister
	grades    GradeLister
	avg       Averager
	att       Attender
}

func NewDetailService(subjects SubjectGetter, schedules ScheduleLister, grades GradeLister, avg Averager, att Attender) *DetailService {
	return &DetailService{subjects: subjects, schedules: schedules, grades: grades, avg: avg, att: att}
}

// Get arma el detalle; 404 si la materia no existe o es ajena.
// Los servicios internos ya devuelven listas [] nunca null.
func (s *DetailService) Get(ctx context.Context, userID, subjectID string) (*SubjectDetail, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	uid := strings.TrimSpace(userID)
	sub, err := s.subjects.Get(ctx, uid, subjectID)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	schedules, err := s.schedules.List(ctx, uid, subjectID)
	if err != nil {
		return nil, err
	}
	grades, err := s.grades.List(ctx, uid, subjectID)
	if err != nil {
		return nil, err
	}
	avg, err := s.avg.Average(ctx, uid, subjectID)
	if err != nil {
		return nil, err
	}
	att, err := s.att.Summary(ctx, uid, subjectID)
	if err != nil {
		return nil, err
	}
	return &SubjectDetail{
		Subject: *sub, Schedules: schedules, Grades: grades,
		Average: avg, Attendance: att,
	}, nil
}
