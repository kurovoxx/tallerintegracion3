package service

import (
	"context"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type observedMeetingNotifier struct {
	MeetingCreatedNotifier
	done chan struct{}
}

func (n *observedMeetingNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	n.MeetingCreatedNotifier.OnMeetingCreated(ctx, meeting)
	close(n.done)
}

// Esperar el cierre sincroniza las lecturas del test con las escrituras del notifier.
// Cada flujo observado crea una sola reunión.
func observeMeetingNotification(t *testing.T, svc *MeetingService) func() {
	t.Helper()
	n := &observedMeetingNotifier{MeetingCreatedNotifier: svc.notifier, done: make(chan struct{})}
	svc.notifier = n
	return func() {
		t.Helper()
		select {
		case <-n.done:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout esperando la notificación de la reunión")
		}
	}
}
