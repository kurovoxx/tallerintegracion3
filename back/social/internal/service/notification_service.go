package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

// NotificationRepo es la porción de persistencia que usa NotificationService
// (tarea 3_5_14). *repository.MeetingRepository es la implementación real.
type NotificationRepo interface {
	ListNotificationsByUser(ctx context.Context, userID string) ([]model.MeetingNotificationItem, error)
	MarkNotificationRead(ctx context.Context, id, userID string) (bool, error)
}

type NotificationService struct {
	repo NotificationRepo
}

func NewNotificationService(repo NotificationRepo) *NotificationService {
	return &NotificationService{repo: repo}
}

// ListMine devuelve las notificaciones in-app del usuario (no leídas primero).
func (s *NotificationService) ListMine(ctx context.Context, userID string) ([]model.MeetingNotificationItem, error) {
	if _, err := parseMeetingUserUUID(userID); err != nil {
		return nil, err
	}
	items, err := s.repo.ListNotificationsByUser(ctx, strings.TrimSpace(userID))
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []model.MeetingNotificationItem{}
	}
	return items, nil
}

// MarkRead marca una notificación como leída. Solo la propia:
// ajena o inexistente (o ya leída) → ErrMeetingNotFound (404, sin filtrar).
func (s *NotificationService) MarkRead(ctx context.Context, userID, id string) error {
	if _, err := parseMeetingUserUUID(userID); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		return ErrMeetingNotFound
	}
	ok, err := s.repo.MarkNotificationRead(ctx, id, strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	if !ok {
		return ErrMeetingNotFound
	}
	return nil
}
