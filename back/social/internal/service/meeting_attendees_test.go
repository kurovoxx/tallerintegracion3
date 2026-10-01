package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/calendar"
)

func TestParseAttendees_Normaliza(t *testing.T) {
	out, err := parseAttendees([]string{"  Ana@X.cl ", "ana@x.cl", "", "  ", "B@y.cl"})
	if err != nil {
		t.Fatalf("no debe fallar: %v", err)
	}
	if len(out) != 2 || out[0] != "ana@x.cl" || out[1] != "b@y.cl" {
		t.Fatalf("normalización inesperada: %v", out)
	}
	if out, err := parseAttendees(nil); err != nil || len(out) != 0 {
		t.Fatalf("nil debe dar vacío sin error: %v %v", out, err)
	}
}

func TestParseAttendees_Invalidos(t *testing.T) {
	for _, bad := range [][]string{{"no-es-email"}, {"a@b"}, {"x@y.zz!"}, {strings.Repeat("a", 252) + "@x.cl"}} {
		if _, err := parseAttendees(bad); err != ErrInvalidAttendeeEmail {
			t.Fatalf("%v debe dar ErrInvalidAttendeeEmail, got %v", bad, err)
		}
	}
	muchos := make([]string, 0, 55)
	for i := 0; i < 55; i++ {
		muchos = append(muchos, string(rune('a'+i%26))+string(rune('0'+i/26))+"@x.cl")
	}
	if _, err := parseAttendees(muchos); err != ErrTooManyAttendees {
		t.Fatalf("55 debe dar ErrTooManyAttendees, got %v", err)
	}
}

func TestMeetingAttendeesFlow_PersisteYEnvia(t *testing.T) {
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	cli := calendar.NewMockClient()
	svc := NewMeetingService(store, NewCalendarMeetingNotifier(&StubCalendarGateway{Token: "tok"}, cli, store))

	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Con invitados", nil, "2026-09-25T15:00:00Z", nil,
		[]string{"Sofia@x.cl", "ana@x.cl", "sofia@X.cl"})
	if err != nil {
		t.Fatalf("crear no debe fallar: %v", err)
	}
	mid, _ := uuid.FromBytes(m.ID.Bytes[:])

	rows, err := store.ListAttendeesByMeeting(context.Background(), m.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("se esperaban 2 filas persistidas, got %v %v", len(rows), err)
	}

	waitFor(t, "evento con invitados", func() bool { return cli.Count == 1 })
	if cli.LastEvent == nil || len(cli.LastEvent.Attendees) != 2 {
		t.Fatalf("evento debe llevar 2 attendees, got %+v", cli.LastEvent)
	}
	_ = mid
}

func TestMeetingAttendeesFlow_Invalido400(t *testing.T) {
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewMeetingService(store, NoopMeetingCreatedNotifier{})

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Mala", nil, "2026-09-25T15:00:00Z", nil,
		[]string{"no-email"}); err != ErrInvalidAttendeeEmail {
		t.Fatalf("se esperaba ErrInvalidAttendeeEmail, got %v", err)
	}
}
