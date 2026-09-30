package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func discordSeamSetup() (gid, admin, member, stranger string) {
	return uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
}

const (
	testServerName = "Servidor prueba"
	testInviteURL  = "https://discord.gg/abc123"
	testWebhookURL = "https://discord.com/api/webhooks/123/abc"
)

func TestDiscordPutConfig_AdminGuarda(t *testing.T) {
	gid, admin, _, _ := discordSeamSetup()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin)
	svc := NewDiscordService(store)

	cfg, err := svc.PutConfig(context.Background(), gid, admin, testServerName, testInviteURL, testWebhookURL)
	if err != nil {
		t.Fatalf("admin debe poder guardar: %v", err)
	}
	if !cfg.ServerName.Valid || cfg.ServerName.String != testServerName {
		t.Fatalf("server_name inesperado: %+v", cfg.ServerName)
	}
	if !cfg.WebhookUrl.Valid || cfg.WebhookUrl.String != testWebhookURL {
		t.Fatalf("webhook inesperada: %+v", cfg.WebhookUrl)
	}

	// Segundo PUT pisa (upsert).
	cfg2, err := svc.PutConfig(context.Background(), gid, admin, "Otro nombre", testInviteURL, "")
	if err != nil {
		t.Fatalf("reconfigurar no debe fallar: %v", err)
	}
	if cfg2.ServerName.String != "Otro nombre" || cfg2.WebhookUrl.Valid {
		t.Fatalf("upsert debe pisar: %+v", cfg2)
	}
}

func TestDiscordGetConfig_MiembroLeeYExtranoNo(t *testing.T) {
	gid, admin, member, stranger := discordSeamSetup()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewDiscordService(store)

	if _, err := svc.GetConfig(context.Background(), gid, member); err != ErrDiscordNotConfigured {
		t.Fatalf("sin configurar debe dar ErrDiscordNotConfigured, got %v", err)
	}
	if _, err := svc.PutConfig(context.Background(), gid, admin, testServerName, testInviteURL, testWebhookURL); err != nil {
		t.Fatalf("put: %v", err)
	}
	cfg, err := svc.GetConfig(context.Background(), gid, member)
	if err != nil {
		t.Fatalf("miembro debe leer: %v", err)
	}
	if !cfg.InviteUrl.Valid || cfg.InviteUrl.String != testInviteURL {
		t.Fatalf("invite inesperada: %+v", cfg.InviteUrl)
	}
	if _, err := svc.GetConfig(context.Background(), gid, stranger); err != ErrForbidden {
		t.Fatalf("extraño debe dar ErrForbidden, got %v", err)
	}
}

func TestDiscordPutConfig_Permisos(t *testing.T) {
	gid, admin, member, stranger := discordSeamSetup()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewDiscordService(store)

	if _, err := svc.PutConfig(context.Background(), gid, member, testServerName, testInviteURL, testWebhookURL); err != ErrForbidden {
		t.Fatalf("member debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.PutConfig(context.Background(), gid, stranger, testServerName, testInviteURL, testWebhookURL); err != ErrForbidden {
		t.Fatalf("no-miembro debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.PutConfig(context.Background(), uuid.NewString(), admin, testServerName, testInviteURL, testWebhookURL); err != ErrGroupNotFound {
		t.Fatalf("grupo inexistente debe dar ErrGroupNotFound, got %v", err)
	}
}

func TestDiscordPutConfig_Validacion(t *testing.T) {
	gid, admin, _, _ := discordSeamSetup()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin)
	svc := NewDiscordService(store)

	if _, err := svc.PutConfig(context.Background(), gid, admin, "  ", testInviteURL, testWebhookURL); err != ErrServerNameEmpty {
		t.Fatalf("server vacío debe dar ErrServerNameEmpty, got %v", err)
	}
	if _, err := svc.PutConfig(context.Background(), gid, admin, testServerName, "  ", testWebhookURL); err != ErrInviteURLEmpty {
		t.Fatalf("invite vacío debe dar ErrInviteURLEmpty, got %v", err)
	}
	if _, err := svc.PutConfig(context.Background(), gid, admin, testServerName, testInviteURL, "https://example.com/hook"); err != ErrInvalidWebhookURL {
		t.Fatalf("webhook no-discord debe dar ErrInvalidWebhookURL, got %v", err)
	}
	if _, err := svc.PutConfig(context.Background(), "no-uuid", admin, testServerName, testInviteURL, testWebhookURL); err != ErrInvalidGroupID {
		t.Fatalf("grupo inválido debe dar ErrInvalidGroupID, got %v", err)
	}
}
