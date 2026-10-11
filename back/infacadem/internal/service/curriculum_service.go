package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// CurriculumStore es el contrato que el service espera del repository.
// Permite fakes en tests sin tocar la BD.
type CurriculumStore interface {
	Create(ctx context.Context, userID, name string) (*model.Curriculum, error)
	GetByID(ctx context.Context, id, userID string) (*model.Curriculum, error)
	ListByUser(ctx context.Context, userID string) ([]model.Curriculum, error)
	UpdateName(ctx context.Context, id, userID, name string) (*model.Curriculum, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// CurriculumService contiene las reglas de curricula (100% personal).
type CurriculumService struct {
	repo CurriculumStore
}

func NewCurriculumService(repo CurriculumStore) *CurriculumService {
	return &CurriculumService{repo: repo}
}

func requireUser(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return NewServiceError(utils.ErrUnauthorized)
	}
	return nil
}

// Create crea una malla propia. Name 1..200 → 400 invalid_name.
func (s *CurriculumService) Create(ctx context.Context, userID, name string) (*model.Curriculum, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	clean, err := ValidateName(name)
	if err != nil {
		return nil, err
	}
	return s.repo.Create(ctx, strings.TrimSpace(userID), clean)
}

// Get retorna una malla propia; 404 si no existe o es ajena.
func (s *CurriculumService) Get(ctx context.Context, userID, id string) (*model.Curriculum, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.GetByID(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return c, nil
}

// List retorna solo las mallas propias (nunca null).
func (s *CurriculumService) List(ctx context.Context, userID string) ([]model.Curriculum, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	out, err := s.repo.ListByUser(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.Curriculum{}
	}
	return out, nil
}

// Update renombra una malla propia; 404 si no existe o es ajena.
func (s *CurriculumService) Update(ctx context.Context, userID, id, name string) (*model.Curriculum, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	clean, err := ValidateName(name)
	if err != nil {
		return nil, err
	}
	c, err := s.repo.UpdateName(ctx, id, strings.TrimSpace(userID), clean)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return c, nil
}

// Delete borra una malla propia (subjects/schedules caen por CASCADE).
// 404 si no existe o es ajena.
func (s *CurriculumService) Delete(ctx context.Context, userID, id string) error {
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
