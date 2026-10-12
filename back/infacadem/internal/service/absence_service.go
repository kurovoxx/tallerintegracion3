package service

import (
	"context"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// AbsenceStore es el contrato que el service espera del repository.
type AbsenceStore interface {
	Create(ctx context.Context, userID, subjectID string, date time.Time, justified bool, source, typ *string) (*model.Absence, error)
	ExistsOnDate(ctx context.Context, userID, subjectID string, date time.Time) (bool, error)
	GetByID(ctx context.Context, id, userID string) (*model.Absence, error)
	ListBySubject(ctx context.Context, subjectID, userID string) ([]model.Absence, bool, error)
	UpdateJustified(ctx context.Context, id, userID string, justified bool) (*model.Absence, error)
	Update(ctx context.Context, id, userID string, p repository.AbsencePatch) (*model.Absence, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
	CountsBySubject(ctx context.Context, subjectID, userID string) (int, int, bool, error)
}

// AbsenceService contiene las reglas de absences (propiedad vía su materia).
type AbsenceService struct {
	repo AbsenceStore
}

func NewAbsenceService(repo AbsenceStore) *AbsenceService {
	return &AbsenceService{repo: repo}
}

// parseAbsenceDate valida YYYY-MM-DD.
// Decisión documentada: se permite cualquier fecha válida (pasada, hoy o
// futura); el front pide justificación para futuras lejanas, pero la API no
// la exige.
func parseAbsenceDate(raw string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}, NewServiceError(utils.ErrInvalidDate)
	}
	return t, nil
}

// AbsenceUpdate agrupa la edición parcial de una falta (nil = omitido).
// Source/Type: nil = omitido, "" = limpiar (NULL).
type AbsenceUpdate struct {
	Justified *bool
	Source    *string
	Type      *string
}

// cleanAbsenceText normaliza un varchar opcional (source/type):
// nil = omitido; "" tras trim = limpiar (NULL); resto = valor trimmed.
func cleanAbsenceText(v *string) (val *string, clear bool) {
	if v == nil {
		return nil, false
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil, true
	}
	return &t, false
}

// Create registra una falta en una materia propia; 409 si ya hay una en esa fecha.
func (s *AbsenceService) Create(ctx context.Context, userID, subjectID, dateRaw string, justified bool, source, typ *string) (*model.Absence, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	date, err := parseAbsenceDate(dateRaw)
	if err != nil {
		return nil, err
	}
	uid := strings.TrimSpace(userID)
	dup, err := s.repo.ExistsOnDate(ctx, uid, subjectID, date)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, NewServiceError(utils.ErrConflict)
	}
	src, _ := cleanAbsenceText(source)
	tp, _ := cleanAbsenceText(typ)
	out, err := s.repo.Create(ctx, uid, subjectID, date, justified, src, tp)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// Get retorna una falta propia; 404 si no existe o es ajena.
func (s *AbsenceService) Get(ctx context.Context, userID, id string) (*model.Absence, error) {
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

// List lista las faltas ordenadas por fecha; 404 si la materia es ajena.
func (s *AbsenceService) List(ctx context.Context, userID, subjectID string) ([]model.Absence, error) {
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
		out = []model.Absence{}
	}
	return out, nil
}

// SetJustified cambia justified (requerido); 404 si no existe o es ajena.
func (s *AbsenceService) SetJustified(ctx context.Context, userID, id string, justified *bool) (*model.Absence, error) {
	return s.Update(ctx, userID, id, AbsenceUpdate{Justified: justified})
}

// Update aplica PATCH parcial (justified y/o source/type); 400 si no hay
// campos, 404 si no existe o es ajena.
func (s *AbsenceService) Update(ctx context.Context, userID, id string, in AbsenceUpdate) (*model.Absence, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	id, err := ValidateUUID(id)
	if err != nil {
		return nil, err
	}
	var p repository.AbsencePatch
	if in.Justified != nil {
		p.Justified = in.Justified
	}
	if src, clear := cleanAbsenceText(in.Source); clear {
		p.ClearSource = true
	} else if src != nil {
		p.Source = src
	}
	if tp, clear := cleanAbsenceText(in.Type); clear {
		p.ClearType = true
	} else if tp != nil {
		p.Type = tp
	}
	if p.Justified == nil && p.Source == nil && p.Type == nil &&
		!p.ClearSource && !p.ClearType {
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

// Delete borra una falta propia; 404 si no existe o es ajena.
func (s *AbsenceService) Delete(ctx context.Context, userID, id string) error {
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
