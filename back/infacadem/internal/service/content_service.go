package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// ContentStore es el contrato que el service espera del repository.
type ContentStore interface {
	Create(ctx context.Context, userID, subjectID, title string, body *string, position int) (*model.Content, error)
	GetByID(ctx context.Context, id, userID string) (*model.Content, error)
	ListBySubject(ctx context.Context, subjectID, userID string) ([]model.Content, bool, error)
	Update(ctx context.Context, id, userID string, p repository.ContentPatch) (*model.Content, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// ContentInput agrupa alta/edición de contenidos (nil = omitido).
// Body "": limpiar (NULL) solo en Update; en Create equivale a omitido.
type ContentInput struct {
	Title    *string
	Body     *string
	Position *int
	Done     *bool
}

// ContentService contiene las reglas de contents (propiedad vía la materia).
type ContentService struct {
	repo ContentStore
}

func NewContentService(repo ContentStore) *ContentService {
	return &ContentService{repo: repo}
}

// Create crea un contenido en una materia propia; 404 si no existe o es ajena.
func (s *ContentService) Create(ctx context.Context, userID, subjectID string, in ContentInput) (*model.Content, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	if in.Title == nil {
		return nil, NewServiceError(utils.ErrInvalidName)
	}
	title, err := ValidateName(*in.Title)
	if err != nil {
		return nil, err
	}
	var body *string
	if in.Body != nil {
		if t := strings.TrimSpace(*in.Body); t != "" {
			body = &t
		}
	}
	position := 0
	if in.Position != nil {
		position = *in.Position
	}
	out, err := s.repo.Create(ctx, strings.TrimSpace(userID), subjectID, title, body, position)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// Get retorna un contenido propio; 404 si no existe o es ajeno.
func (s *ContentService) Get(ctx context.Context, userID, id string) (*model.Content, error) {
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

// List lista los contenidos ordenados por posición; 404 si la materia es ajena.
func (s *ContentService) List(ctx context.Context, userID, subjectID string) ([]model.Content, error) {
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
		out = []model.Content{}
	}
	return out, nil
}

// Update aplica PATCH parcial; 400 si no hay campos, 404 si no existe o es ajeno.
func (s *ContentService) Update(ctx context.Context, userID, id string, in ContentInput) (*model.Content, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	var p repository.ContentPatch
	if in.Title != nil {
		title, err := ValidateName(*in.Title)
		if err != nil {
			return nil, err
		}
		p.Title = &title
	}
	if in.Body != nil {
		if t := strings.TrimSpace(*in.Body); t == "" {
			p.ClearBody = true
		} else {
			p.Body = &t
		}
	}
	if in.Position != nil {
		p.Position = in.Position
	}
	if in.Done != nil {
		p.Done = in.Done
	}
	if p.Title == nil && p.Body == nil && p.Position == nil && p.Done == nil && !p.ClearBody {
		return nil, NewServiceError(utils.ErrBadRequest)
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

// Delete borra un contenido propio; 404 si no existe o es ajeno.
func (s *ContentService) Delete(ctx context.Context, userID, id string) error {
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
