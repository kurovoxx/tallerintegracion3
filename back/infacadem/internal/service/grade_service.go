package service

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// GradeStore es el contrato que el service espera del repository.
type GradeStore interface {
	Create(ctx context.Context, userID, subjectID string, in repository.GradeCreate) (*model.Grade, error)
	GetByID(ctx context.Context, id, userID string) (*model.Grade, error)
	ListBySubject(ctx context.Context, subjectID, userID string) ([]model.Grade, bool, error)
	Update(ctx context.Context, id, userID string, p repository.GradePatch) (*model.Grade, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
	CountFinalExams(ctx context.Context, subjectID, userID, excludeID string) (int, error)
}

// GradeInput agrupa alta/edición (nil = omitido; Clear* = NULL explícito).
type GradeInput struct {
	Name                  *string
	Score                 *int
	Weight                *float64
	Status                *string
	ScheduledDate         *string
	Topics                *string
	NotifyDiscord         *bool
	GoogleCalendarEventID *string
	IsFinalExam           *bool
	Pinned                *bool
	ClearScore            bool
	ClearScheduledDate    bool
	ClearTopics           bool
	ClearCalendarEvent    bool
}

// GradeNode es una nota tope con sus hijas anidadas (GET de la materia).
type GradeNode struct {
	Grade    model.Grade   `json:"grade"`
	Children []model.Grade `json:"children"`
}

// GradeService contiene las reglas de grades (propiedad vía su materia).
type GradeService struct {
	repo GradeStore
}

func NewGradeService(repo GradeStore) *GradeService {
	return &GradeService{repo: repo}
}

// RoundHalfUp replica el redondeo del front (grade_calculator_screen.dart):
// (v*factor + 0.5).floor / factor.
func RoundHalfUp(value float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))
	return math.Floor(value*factor+0.5) / factor
}

func cleanWeight(v *float64, required bool) (float64, error) {
	if v == nil {
		if required {
			return 0, NewServiceError(utils.ErrInvalidWeight)
		}
		return 0, nil
	}
	if *v < 0 || *v > 1 {
		return 0, NewServiceError(utils.ErrInvalidWeight)
	}
	return *v, nil
}

func cleanScore(v *int) (*int, error) {
	if v == nil {
		return nil, nil
	}
	if *v < 10 || *v > 70 {
		return nil, NewServiceError(utils.ErrInvalidScore)
	}
	cp := *v
	return &cp, nil
}

func cleanGradeStatus(v *string) (string, error) {
	if v == nil {
		return model.GradeStatusPending, nil
	}
	s := strings.TrimSpace(*v)
	if !model.IsValidGradeStatus(s) {
		return "", NewServiceError(utils.ErrInvalidStatus)
	}
	return s, nil
}

func cleanDate(v *string) (*time.Time, error) {
	if v == nil {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(*v))
	if err != nil {
		return nil, NewServiceError(utils.ErrInvalidDate)
	}
	return &t, nil
}

// checkFinalConflict valida el máximo de 1 final por materia → 409.
func (s *GradeService) checkFinalConflict(ctx context.Context, userID, subjectID, excludeID string, wantFinal bool) error {
	if !wantFinal {
		return nil
	}
	n, err := s.repo.CountFinalExams(ctx, subjectID, userID, excludeID)
	if err != nil {
		return err
	}
	if n > 0 {
		return NewServiceError(utils.ErrConflict)
	}
	return nil
}

// Create crea una nota tope en una materia propia; 404 si es ajena.
func (s *GradeService) Create(ctx context.Context, userID, subjectID string, in GradeInput) (*model.Grade, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	if in.Name == nil {
		return nil, NewServiceError(utils.ErrInvalidName)
	}
	name, err := ValidateName(*in.Name)
	if err != nil {
		return nil, err
	}
	weight, err := cleanWeight(in.Weight, true)
	if err != nil {
		return nil, err
	}
	score, err := cleanScore(in.Score)
	if err != nil {
		return nil, err
	}
	status, err := cleanGradeStatus(in.Status)
	if err != nil {
		return nil, err
	}
	date, err := cleanDate(in.ScheduledDate)
	if err != nil {
		return nil, err
	}
	final := in.IsFinalExam != nil && *in.IsFinalExam
	if err := s.checkFinalConflict(ctx, strings.TrimSpace(userID), subjectID, "", final); err != nil {
		return nil, err
	}
	out, err := s.repo.Create(ctx, strings.TrimSpace(userID), subjectID, repository.GradeCreate{
		Status: status, Name: name, Score: score, Weight: weight,
		ScheduledDate: date, Topics: in.Topics, NotifyDiscord: in.NotifyDiscord != nil && *in.NotifyDiscord,
		GoogleCalendarEventID: in.GoogleCalendarEventID, IsFinalExam: final,
	})
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// CreateChild crea una subnota (fija parent_grade_id del padre propio).
// Hereda subject_id del padre; el score propio del padre se ignora desde entonces.
func (s *GradeService) CreateChild(ctx context.Context, userID, parentID string, in GradeInput) (*model.Grade, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	parentID, err := ValidateUUID(parentID)
	if err != nil {
		return nil, err
	}
	parent, err := s.repo.GetByID(ctx, parentID, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if parent == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	if in.Name == nil {
		return nil, NewServiceError(utils.ErrInvalidName)
	}
	name, err := ValidateName(*in.Name)
	if err != nil {
		return nil, err
	}
	weight, err := cleanWeight(in.Weight, true)
	if err != nil {
		return nil, err
	}
	score, err := cleanScore(in.Score)
	if err != nil {
		return nil, err
	}
	status, err := cleanGradeStatus(in.Status)
	if err != nil {
		return nil, err
	}
	date, err := cleanDate(in.ScheduledDate)
	if err != nil {
		return nil, err
	}
	final := in.IsFinalExam != nil && *in.IsFinalExam
	if err := s.checkFinalConflict(ctx, strings.TrimSpace(userID), parent.SubjectID, "", final); err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, strings.TrimSpace(userID), parent.SubjectID, repository.GradeCreate{
		ParentGradeID: &parent.ID, Status: status, Name: name, Score: score, Weight: weight,
		ScheduledDate: date, Topics: in.Topics, NotifyDiscord: in.NotifyDiscord != nil && *in.NotifyDiscord,
		GoogleCalendarEventID: in.GoogleCalendarEventID, IsFinalExam: final,
	})
}

// Get retorna una nota propia; 404 si no existe o es ajena.
func (s *GradeService) Get(ctx context.Context, userID, id string) (*model.Grade, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	out, err := s.repo.GetByID(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// List retorna las notas tope con hijas anidadas; 404 si la materia es ajena.
func (s *GradeService) List(ctx context.Context, userID, subjectID string) ([]GradeNode, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	all, owned, err := s.repo.ListBySubject(ctx, subjectID, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	byParent := make(map[string][]model.Grade)
	var tops []GradeNode
	for _, g := range all {
		if g.ParentGradeID != nil {
			byParent[*g.ParentGradeID] = append(byParent[*g.ParentGradeID], g)
		}
	}
	for _, g := range all {
		if g.ParentGradeID == nil {
			tops = append(tops, GradeNode{Grade: g, Children: byParent[g.ID]})
		}
	}
	if tops == nil {
		tops = []GradeNode{}
	}
	for i := range tops {
		if tops[i].Children == nil {
			tops[i].Children = []model.Grade{}
		}
	}
	return tops, nil
}

// Update aplica PATCH parcial; 400 si no hay campos, 409 si duplica el final.
func (s *GradeService) Update(ctx context.Context, userID, id string, in GradeInput) (*model.Grade, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	var p repository.GradePatch
	if in.Name != nil {
		name, err := ValidateName(*in.Name)
		if err != nil {
			return nil, err
		}
		p.Name = &name
	}
	if in.ClearScore {
		p.ClearScore = true
	} else if in.Score != nil {
		score, err := cleanScore(in.Score)
		if err != nil {
			return nil, err
		}
		p.Score = score
	}
	if in.Weight != nil {
		w, err := cleanWeight(in.Weight, false)
		if err != nil {
			return nil, err
		}
		p.Weight = &w
	}
	if in.Status != nil {
		status, err := cleanGradeStatus(in.Status)
		if err != nil {
			return nil, err
		}
		p.Status = &status
	}
	if in.ClearScheduledDate {
		p.ClearScheduledAt = true
	} else if in.ScheduledDate != nil {
		date, err := cleanDate(in.ScheduledDate)
		if err != nil {
			return nil, err
		}
		p.ScheduledDate = date
	}
	if in.ClearTopics {
		p.ClearTopics = true
	} else if in.Topics != nil {
		p.Topics = in.Topics
	}
	if in.NotifyDiscord != nil {
		p.NotifyDiscord = in.NotifyDiscord
	}
	if in.ClearCalendarEvent {
		p.ClearCalendarEvt = true
	} else if in.GoogleCalendarEventID != nil {
		p.GoogleCalendarEventID = in.GoogleCalendarEventID
	}
	if in.IsFinalExam != nil {
		p.IsFinalExam = in.IsFinalExam
	}
	if in.Pinned != nil {
		p.Pinned = in.Pinned
	}
	if p.Name == nil && p.Score == nil && !p.ClearScore && p.Weight == nil && p.Status == nil &&
		p.ScheduledDate == nil && !p.ClearScheduledAt && p.Topics == nil && !p.ClearTopics &&
		p.NotifyDiscord == nil && p.GoogleCalendarEventID == nil && !p.ClearCalendarEvt && p.IsFinalExam == nil &&
		p.Pinned == nil {
		return nil, NewServiceError(utils.ErrBadRequest)
	}
	// Para validar el final único se necesita la materia: leer primero (404 si ajena).
	current, err := s.repo.GetByID(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	if err := s.checkFinalConflict(ctx, strings.TrimSpace(userID), current.SubjectID, id, p.IsFinalExam != nil && *p.IsFinalExam); err != nil {
		return nil, err
	}
	out, err := s.repo.Update(ctx, id, strings.TrimSpace(userID), p)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// Delete borra una nota propia (hijas por CASCADE); 404 si es ajena.
func (s *GradeService) Delete(ctx context.Context, userID, id string) error {
	if err := requireUser(userID); err != nil {
		return err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return err
	}
	deleted, err := s.repo.Delete(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	if !deleted {
		return NewServiceError(utils.ErrNotFound)
	}
	return nil
}
