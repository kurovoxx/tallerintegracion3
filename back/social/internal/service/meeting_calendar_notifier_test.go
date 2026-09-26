package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/calendar"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type fakeMeetingStore struct {
	lastMeetingID pgtype.UUID
	lastEventID   string
	err           error
}

func (f *fakeMeetingStore) SetCalendarEventID(ctx context.Context, meetingID pgtype.UUID, eventID string) (sqlc.SocialMeeting, error) {
	f.lastMeetingID = meetingID
	f.lastEventID = eventID
	return sqlc.SocialMeeting{}, f.err
}

func testMeeting() sqlc.SocialMeeting {
	var mid, gid, uid pgtype.UUID
	_ = mid.Scan(uuid.NewString())
	_ = gid.Scan(uuid.NewString())
	_ = uid.Scan(uuid.NewString())
	var sched pgtype.Timestamptz
	_ = sched.Scan(time.Now().Add(24 * time.Hour))
	var desc pgtype.Text
	_ = desc.Scan("descripción prueba")
	return sqlc.SocialMeeting{
		ID:              mid,
		GroupID:         gid,
		Title:           "Reunión prueba",
		Description:     desc,
		ScheduledAt:     sched,
		CreatedByUserID: uid,
		NotifyDiscord:   true,
	}
}

func TestCalendarNotifier_CreaEventoYPersiste(t *testing.T) {
	gw := &StubCalendarGateway{Token: "tok-1"}
	cli := calendar.NewMockClient()
	store := &fakeMeetingStore{}
	n := NewCalendarMeetingNotifier(gw, cli, store)

	meeting := testMeeting()
	n.OnMeetingCreated(context.Background(), meeting)

	if cli.Count != 1 {
		t.Fatalf("se esperaba 1 evento creado, got %d", cli.Count)
	}
	if cli.LastEvent == nil || cli.LastEvent.Summary != "Reunión prueba" {
		t.Fatalf("evento inesperado: %+v", cli.LastEvent)
	}
	if store.lastEventID == "" || store.lastEventID != cli.LastEventID() {
		t.Fatalf("no se persistió el event id: %q", store.lastEventID)
	}
	if store.lastMeetingID != meeting.ID {
		t.Fatalf("se persistió con meeting id incorrecto")
	}
}

func TestCalendarNotifier_SinConexion_SkipSinError(t *testing.T) {
	gw := &noConnGateway{}
	cli := calendar.NewMockClient()
	store := &fakeMeetingStore{}
	n := NewCalendarMeetingNotifier(gw, cli, store)

	n.OnMeetingCreated(context.Background(), testMeeting())

	if cli.Count != 0 {
		t.Fatalf("sin conexión no debe crear eventos, got %d", cli.Count)
	}
	if store.lastEventID != "" {
		t.Fatalf("sin conexión no debe persistir nada, got %q", store.lastEventID)
	}
}

func TestCalendarNotifier_ErrorGoogle_NoPersiste(t *testing.T) {
	gw := &StubCalendarGateway{Token: "tok-1"}
	cli := calendar.NewMockClient()
	cli.CreateErr = &calendar.CalendarError{Code: 500, Message: "boom"}
	store := &fakeMeetingStore{}
	n := NewCalendarMeetingNotifier(gw, cli, store)

	n.OnMeetingCreated(context.Background(), testMeeting())

	if store.lastEventID != "" {
		t.Fatalf("con error de Google no debe persistir, got %q", store.lastEventID)
	}
}

func TestCalendarNotifier_Revocado_ReportaAAuth(t *testing.T) {
	gw := &StubCalendarGateway{Token: "tok-malo"}
	cli := calendar.NewMockClient()
	cli.CreateErr = &calendar.CalendarError{Code: 401, Message: "revocado"}
	store := &fakeMeetingStore{}
	n := NewCalendarMeetingNotifier(gw, cli, store)

	meeting := testMeeting()
	n.OnMeetingCreated(context.Background(), meeting)

	if len(gw.RevokedReports) != 1 {
		t.Fatalf("se esperaba 1 reporte de revocación, got %d", len(gw.RevokedReports))
	}
	if store.lastEventID != "" {
		t.Fatalf("revocado no debe persistir, got %q", store.lastEventID)
	}
}

type noConnGateway struct{}

func (n *noConnGateway) GetToken(ctx context.Context, userID string) (string, error) {
	return "", ErrCalendarNotConnected
}

func (n *noConnGateway) ReportRevoked(ctx context.Context, userID string) error {
	return errors.New("no debe llamarse")
}
