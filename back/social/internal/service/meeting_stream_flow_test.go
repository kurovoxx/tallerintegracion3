package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

// Flujo meeting + Stream: MeetingService real + StreamMessageNotifier real,
// con repo fake y mock. El notifier corre en background (GoBestEffort).

func streamFlowSetup(t *testing.T, withChannel bool) (*MeetingService, *MemoryMeetingStore, *stream.MockClient, string, string) {
	t.Helper()
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	if withChannel {
		store.SetStreamChannel(gid, ChannelIDForGroup(gid))
	}
	cli := stream.NewMockClient()
	svc := NewMeetingService(store, NewStreamMessageNotifier(store, cli))
	return svc, store, cli, gid, admin
}

func TestMeetingStreamFlow_ConCanal_Anuncia(t *testing.T) {
	svc, _, cli, gid, admin := streamFlowSetup(t, true)

	m, err := svc.CreateMeeting(context.Background(), gid, admin, "Reunión flujo stream", nil, "2026-09-25T15:00:00Z", nil)
	if err != nil {
		t.Fatalf("crear no debe fallar: %v", err)
	}
	creator, _ := uuid.FromBytes(m.CreatedByUserID.Bytes[:])

	waitFor(t, "anuncio stream", func() bool { return len(cli.Messages) == 1 })
	msg := cli.Messages[0]
	if msg.ChannelType != "messaging" || msg.ChannelID != ChannelIDForGroup(gid) {
		t.Fatalf("destino inesperado: %+v", msg)
	}
	if msg.SenderID != creator.String() {
		t.Fatalf("sender debe ser el creador, got %q", msg.SenderID)
	}
	if msg.Text == "" || !strings.Contains(msg.Text, "Reunión flujo stream") {
		t.Fatalf("texto incompleto: %q", msg.Text)
	}
}

func TestMeetingStreamFlow_SinCanal_CreadaIgual(t *testing.T) {
	svc, _, cli, gid, admin := streamFlowSetup(t, false)

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Sin canal", nil, "2026-09-25T15:00:00Z", nil); err != nil {
		t.Fatalf("sin canal igual debe crear (201): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if len(cli.Messages) != 0 {
		t.Fatalf("sin canal no debe anunciar, got %d", len(cli.Messages))
	}
}

func TestMeetingStreamFlow_ErrorStream_CreadaIgual(t *testing.T) {
	svc, _, cli, gid, admin := streamFlowSetup(t, true)
	cli.SendErr = &stream.StreamError{Code: 404, Message: "canal inexistente"}

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Stream roto", nil, "2026-09-25T15:00:00Z", nil); err != nil {
		t.Fatalf("con Stream roto igual debe crear (201): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if len(cli.Messages) != 0 {
		t.Fatalf("fallo no debe contar como envío, got %d", len(cli.Messages))
	}
}
