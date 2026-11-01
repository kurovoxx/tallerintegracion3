package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Smoke test de la costura: el servicio opera contra el fake sin DB,
// incluyendo validaciones, 404/403 y notificaciones in-app.

func meetingSeamSetup() (gid, admin, member, stranger string) {
	return uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
}

func TestMeetingServiceSeam_CreateListNotifications(t *testing.T) {
	gid, admin, member, _ := meetingSeamSetup()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewMeetingService(store, NoopMeetingCreatedNotifier{})

	desc := "llevar apuntes"
	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Reunión costura", &desc, "2026-09-25T15:00:00Z", nil, nil)
	if err != nil {
		t.Fatalf("crear no debe fallar: %v", err)
	}
	if m.Title != "Reunión costura" || !m.NotifyDiscord {
		t.Fatalf("reunión inesperada: %+v", m)
	}
	mid, _ := uuid.FromBytes(m.ID.Bytes[:])
	if got := store.NotificationsFor(mid.String()); got != 1 {
		t.Fatalf("se esperaba 1 notificación (solo member), got %d", got)
	}

	upcoming, err := svc.ListUpcomingMeetings(context.Background(), gid, member, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(upcoming) != 1 {
		t.Fatalf("se esperaba 1 próxima, got %d, %v", len(upcoming), err)
	}
}

func TestMeetingServiceSeam_Errores(t *testing.T) {
	gid, admin, _, stranger := meetingSeamSetup()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin)
	svc := NewMeetingService(store, NoopMeetingCreatedNotifier{})

	if _, err := svc.CreateMeeting(context.Background(), uuid.NewString(), admin, "X", nil, "2026-09-25T15:00:00Z", nil, nil); err != ErrGroupNotFound {
		t.Fatalf("grupo inexistente debe dar ErrGroupNotFound, got %v", err)
	}
	if _, err := svc.CreateMeeting(context.Background(), gid, stranger, "X", nil, "2026-09-25T15:00:00Z", nil, nil); err != ErrForbidden {
		t.Fatalf("no-miembro debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "X", nil, "ayer", nil, nil); err != ErrInvalidScheduledAt {
		t.Fatalf("fecha mala debe dar ErrInvalidScheduledAt, got %v", err)
	}
	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "  ", nil, "2026-09-25T15:00:00Z", nil, nil); err != ErrInvalidTitle {
		t.Fatalf("título vacío debe dar ErrInvalidTitle, got %v", err)
	}
}
