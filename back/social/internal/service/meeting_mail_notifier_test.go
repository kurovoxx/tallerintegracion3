package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type fakeMailSender struct {
	to    []string
	title string
	calls int
	err   error
}

func (f *fakeMailSender) SendMeetingInvite(_ context.Context, to []string, _, title string, _ time.Time, _ bool, _ string) error {
	f.calls++
	f.to = append([]string{}, to...)
	f.title = title
	return f.err
}

// Crea grupo + reunión en memoria y devuelve el notifier con mail falso.
func mailSeam(t *testing.T, attendees []string) (sqlc.SocialMeeting, *MeetingMailNotifier, *fakeMailSender) {
	t.Helper()
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewMeetingService(store, NoopMeetingCreatedNotifier{})
	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Daily mail", nil, "2026-11-20T15:00:00Z", nil, attendees)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	mail := &fakeMailSender{}
	return m, NewMeetingMailNotifier(store, mail), mail
}

func TestMailNotifier_SendsToAttendees(t *testing.T) {
	m, n, mail := mailSeam(t, []string{"a@x.com", "b@x.com"})
	n.OnMeetingCreated(context.Background(), m)
	if mail.calls != 1 {
		t.Fatalf("se esperaba 1 envío, got %d", mail.calls)
	}
	if len(mail.to) != 2 || mail.title != "Daily mail" {
		t.Fatalf("destinatarios/título: %v %q", mail.to, mail.title)
	}
}

func TestMailNotifier_NoAttendeesSkips(t *testing.T) {
	m, n, mail := mailSeam(t, nil)
	n.OnMeetingCreated(context.Background(), m)
	if mail.calls != 0 {
		t.Fatal("sin invitados no debe enviar")
	}
}

func TestMailNotifier_FailureIsBestEffort(t *testing.T) {
	m, n, mail := mailSeam(t, []string{"a@x.com"})
	mail.err = errors.New("smtp caído")
	n.OnMeetingCreated(context.Background(), m) // no debe propagar ni panic
	if mail.calls != 1 {
		t.Fatal("se intentó igual el envío")
	}
}
