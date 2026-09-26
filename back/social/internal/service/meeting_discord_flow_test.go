package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/discord"
)

// Flujo meeting + Discord: MeetingService real + DiscordMeetingNotifier real,
// con repo fake y mock. El notifier corre en background (GoBestEffort).

func discordFlowSetup(t *testing.T, withWebhook bool) (*MeetingService, *MemoryMeetingStore, *discord.MockClient, string, string) {
	t.Helper()
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	if withWebhook {
		dsvc := NewDiscordService(store)
		if _, err := dsvc.PutConfig(context.Background(), gid, admin, "Servidor", "https://discord.gg/abc", "https://discord.com/api/webhooks/1/x"); err != nil {
			t.Fatalf("configurar webhook: %v", err)
		}
	}
	cli := discord.NewMockClient()
	svc := NewMeetingService(store, NewDiscordMeetingNotifier(store, cli))
	return svc, store, cli, gid, admin
}

func TestMeetingDiscordFlow_ConWebhook_Envia(t *testing.T) {
	svc, _, cli, gid, admin := discordFlowSetup(t, true)

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Reunión flujo dc", nil, "2026-09-25T15:00:00Z", nil); err != nil {
		t.Fatalf("crear no debe fallar: %v", err)
	}
	waitFor(t, "aviso discord", func() bool { return cli.Count == 1 })
	if cli.LastMessage == nil || cli.LastMessage.Title != "Reunión flujo dc" {
		t.Fatalf("mensaje inesperado: %+v", cli.LastMessage)
	}
	if cli.LastURL != "https://discord.com/api/webhooks/1/x" {
		t.Fatalf("webhook inesperada: %q", cli.LastURL)
	}
}

func TestMeetingDiscordFlow_SinWebhook_CreadaIgual(t *testing.T) {
	svc, _, cli, gid, admin := discordFlowSetup(t, false)

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Sin webhook", nil, "2026-09-25T15:00:00Z", nil); err != nil {
		t.Fatalf("sin webhook igual debe crear (201): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if cli.Count != 0 {
		t.Fatalf("sin webhook no debe enviar, got %d", cli.Count)
	}
}

func TestMeetingDiscordFlow_NotifyFalse_CreadaIgual(t *testing.T) {
	svc, _, cli, gid, admin := discordFlowSetup(t, true)
	notify := false

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Silenciosa", nil, "2026-09-25T15:00:00Z", &notify); err != nil {
		t.Fatalf("con notify=false igual debe crear (201): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if cli.Count != 0 {
		t.Fatalf("con notify=false no debe enviar, got %d", cli.Count)
	}
}

func TestMeetingDiscordFlow_ErrorWebhook_CreadaIgual(t *testing.T) {
	svc, _, cli, gid, admin := discordFlowSetup(t, true)
	cli.SendErr = &discord.WebhookError{Code: 404, Message: "webhook borrado"}

	if _, err := svc.CreateMeeting(context.Background(), gid, admin, "Webhook roto", nil, "2026-09-25T15:00:00Z", nil); err != nil {
		t.Fatalf("con webhook roto igual debe crear (201): %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if cli.Count != 0 {
		t.Fatalf("fallo no debe contar como envío, got %d", cli.Count)
	}
}
