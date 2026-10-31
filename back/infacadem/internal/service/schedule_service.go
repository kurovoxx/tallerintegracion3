package service

import (
	"context"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// ScheduleStore es el contrato que el service espera del repository.
type ScheduleStore interface {
	Create(ctx context.Context, userID, subjectID string, dayOfWeek int, start, end time.Time, modality *string) (*model.Schedule, error)
	ListBySubject(ctx context.Context, subjectID, userID string) ([]model.Schedule, bool, error)
	Delete(ctx context.Context, id, userID string) (bool, error)
}

// ScheduleService contiene las reglas de schedules (propiedad vía la malla).
type ScheduleService struct {
	repo ScheduleStore
}

func NewScheduleService(repo ScheduleStore) *ScheduleService {
	return &ScheduleService{repo: repo}
}

// timeLayouts aceptados en la API (documentado): "15:04" y "15:04:05".
var timeLayouts = []string{"15:04:05", "15:04"}

func parseDayTime(raw string) (time.Time, error) {
	v := strings.TrimSpace(raw)
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, NewServiceError(utils.ErrInvalidTimeFormat)
}

// Create crea un horario en una materia propia; 404 si la materia no existe o es ajena.
// modality nil o vacía (tras trim) = NULL.
func (s *ScheduleService) Create(ctx context.Context, userID, subjectID string, dayOfWeek int, startRaw, endRaw string, modality *string) (*model.Schedule, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	if !model.IsValidDayOfWeek(dayOfWeek) {
		return nil, NewServiceError(utils.ErrInvalidDayOfWeek)
	}
	start, err := parseDayTime(startRaw)
	if err != nil {
		return nil, err
	}
	end, err := parseDayTime(endRaw)
	if err != nil {
		return nil, err
	}
	if !end.After(start) {
		return nil, NewServiceError(utils.ErrInvalidTimeRange)
	}
	var mod *string
	if modality != nil {
		if t := strings.TrimSpace(*modality); t != "" {
			mod = &t
		}
	}
	out, err := s.repo.Create(ctx, strings.TrimSpace(userID), subjectID, dayOfWeek, start, end, mod)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	return out, nil
}

// List lista los horarios de una materia propia; 404 si no existe o es ajena.
func (s *ScheduleService) List(ctx context.Context, userID, subjectID string) ([]model.Schedule, error) {
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
		out = []model.Schedule{}
	}
	return out, nil
}

// Delete borra un horario propio; 404 si no existe o es ajeno.
func (s *ScheduleService) Delete(ctx context.Context, userID, id string) error {
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
