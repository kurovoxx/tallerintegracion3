package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// RequirementStore es el contrato que el service espera del repository.
type RequirementStore interface {
	Create(ctx context.Context, userID, subjectID string, sourceGradeID *string, checksFinal bool, comparator string, threshold float64, action string) (*model.Requirement, error)
	ListBySubject(ctx context.Context, subjectID, userID string) ([]model.Requirement, bool, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
	GradeSubjectOf(ctx context.Context, gradeID, subjectID, userID string) (bool, error)
}

// RequirementInput agrupa el alta de requisitos.
type RequirementInput struct {
	SourceGradeID      *string
	ChecksFinalAverage *bool
	Comparator         *string
	ThresholdValue     *float64
	Action             *string
}

// RequirementService contiene las reglas de grade_requirements.
type RequirementService struct {
	repo RequirementStore
}

func NewRequirementService(repo RequirementStore) *RequirementService {
	return &RequirementService{repo: repo}
}

// Create crea un requisito en una materia propia; 404 si es ajena.
// Fuente XOR: exactamente una entre promedio final y nota (400 si ambas o ninguna).
func (s *RequirementService) Create(ctx context.Context, userID, subjectID string, in RequirementInput) (*model.Requirement, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	checks := in.ChecksFinalAverage != nil && *in.ChecksFinalAverage
	hasSource := in.SourceGradeID != nil && strings.TrimSpace(*in.SourceGradeID) != ""
	if checks == hasSource {
		return nil, NewServiceError(utils.ErrInvalidRequirement)
	}
	var source *string
	if hasSource {
		src, err := ValidateUUID(*in.SourceGradeID)
		if err != nil {
			return nil, err
		}
		// La fuente debe ser una nota de la MISMA materia propia → 400 si no.
		ok, err := s.repo.GradeSubjectOf(ctx, src, subjectID, strings.TrimSpace(userID))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, NewServiceError(utils.ErrInvalidRequirement)
		}
		source = &src
	}
	if in.Comparator == nil || !model.IsValidComparator(strings.TrimSpace(*in.Comparator)) {
		return nil, NewServiceError(utils.ErrInvalidComparator)
	}
	if in.ThresholdValue == nil {
		return nil, NewServiceError(utils.ErrInvalidRequirement)
	}
	if in.Action == nil || !model.IsValidRequirementAction(strings.TrimSpace(*in.Action)) {
		return nil, NewServiceError(utils.ErrInvalidAction)
	}
	out, err := s.repo.Create(ctx, strings.TrimSpace(userID), subjectID, source, checks,
		strings.TrimSpace(*in.Comparator), *in.ThresholdValue, strings.TrimSpace(*in.Action))
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// List lista los requisitos de una materia propia; 404 si es ajena.
func (s *RequirementService) List(ctx context.Context, userID, subjectID string) ([]model.Requirement, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	out, owned, err := s.repo.ListBySubject(ctx, subjectID, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	if out == nil {
		out = []model.Requirement{}
	}
	return out, nil
}

// Delete borra un requisito propio; 404 si no existe o es ajeno.
func (s *RequirementService) Delete(ctx context.Context, userID, id string) error {
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
