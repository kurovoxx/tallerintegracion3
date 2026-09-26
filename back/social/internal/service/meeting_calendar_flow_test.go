package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/calendar"
)

// Flujo meeting + Calendar: MeetingService real + CalendarMeetingNotifier real,
// con repo fake y mocks. El notifier corre en background (GoBestEffort), así
// que se espera con deadline en vez de sleep fijo.

func calendarFlowSetup(token string) (svc *MeetingService, store *MemoryMeetingStore, cli *calendar.MockClient, gid, admin, member string) {
	gid, admin, member = uuid.NewString(), uuid.NewString(), uuid.NewString()
	store = NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	cli = calendar.NewMockClient()
	var gw CalendarTokenGateway = &StubCalendarGateway{Token: token}
	notifier := NewCalendarMeetingNotifier(gw, cli, store)
	svc = NewMeetingService(store, notifier)
	return svc, store, cli, gid, admin, member
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("timeout esperando: %s", what)
	}
}

func TestMeetingCalendarFlow_Conectado_PersisteEventID(t *testing.T) {
	svc, store, cli, gid, admin, _ := calendarFlowSetup("tok-cal")

	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Reunión flujo cal", nil, "2026-09-25T15:00:00Z", nil)
	if err != nil {
		t.Fatalf("crear no debe fallar: %v", err)
	}
	mid, _ := uuid.FromBytes(m.ID.Bytes[:])

	waitFor(t, "evento calendar", func() bool { return cli.Count == 1 })
	if cli.LastEvent == nil || cli.LastEvent.Summary != "Reunión flujo cal" {
		t.Fatalf("evento inesperado: %+v", cli.LastEvent)
	}
	if cli.LastToken != "tok-cal" {
		t.Fatalf("token inesperado: %q", cli.LastToken)
	}
	waitFor(t, "event id persistido", func() bool { return store.CalendarEventFor(mid.String()) != "" })
	if got := store.CalendarEventFor(mid.String()); got != "mock_evt_1" {
		t.Fatalf("event id inesperado: %q", got)
	}
}

func TestMeetingCalendarFlow_SinConexion_CreadaIgual(t *testing.T) {
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	cli := calendar.NewMockClient()
	notifier := NewCalendarMeetingNotifier(&noConnGateway{}, cli, store)
	svc := NewMeetingService(store, notifier)

	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Sin calendar", nil, "2026-09-25T15:00:00Z", nil)
	if err != nil {
		t.Fatalf("sin conexión igual debe crear (201): %v", err)
	}
	mid, _ := uuid.FromBytes(m.ID.Bytes[:])
	// Dar margen al background y verificar que NO persistió nada.
	time.Sleep(200 * time.Millisecond)
	if cli.Count != 0 {
		t.Fatalf("sin conexión no debe crear eventos, got %d", cli.Count)
	}
	if got := store.CalendarEventFor(mid.String()); got != "" {
		t.Fatalf("sin conexión no debe persistir event id, got %q", got)
	}
}

func TestMeetingCalendarFlow_ErrorGoogle_CreadaIgual(t *testing.T) {
	svc, store, cli, gid, admin, _ := calendarFlowSetup("tok-cal")
	cli.CreateErr = &calendar.CalendarError{Code: 500, Message: "boom"}

	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Google caído", nil, "2026-09-25T15:00:00Z", nil)
	if err != nil {
		t.Fatalf("con Google caído igual debe crear (201): %v", err)
	}
	mid, _ := uuid.FromBytes(m.ID.Bytes[:])
	time.Sleep(200 * time.Millisecond)
	if got := store.CalendarEventFor(mid.String()); got != "" {
		t.Fatalf("con error no debe persistir event id, got %q", got)
	}
}
