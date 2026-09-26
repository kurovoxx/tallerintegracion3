package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/discord"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

func TestValidateWebhookURL(t *testing.T) {
	ok, err := validateWebhookURL("https://discord.com/api/webhooks/123/abc")
	if err != nil || !ok.Valid {
		t.Fatalf("webhook válido rechazado: %v", err)
	}
	for _, bad := range []string{
		"http://discord.com/api/webhooks/123/abc", // no https
		"https://example.com/api/webhooks/123",    // host inválido
		"https://discord.com/invite/abc",          // path inválido
		"no-es-url",
	} {
		if _, err := validateWebhookURL(bad); !errors.Is(err, ErrInvalidWebhookURL) {
			t.Fatalf("se esperaba ErrInvalidWebhookURL para %q, got %v", bad, err)
		}
	}
	empty, err := validateWebhookURL("  ")
	if err != nil || empty.Valid {
		t.Fatalf("webhook vacío debe ser NULL sin error, got %v, %v", empty, err)
	}
}

type fakeDiscordStore struct {
	cfg sqlc.SocialDiscordIntegration
	err error
}

func (f *fakeDiscordStore) GetConfigByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialDiscordIntegration, error) {
	return f.cfg, f.err
}

func discordTestMeeting(notify bool) sqlc.SocialMeeting {
	var mid, gid pgtype.UUID
	_ = mid.Scan(uuid.NewString())
	_ = gid.Scan(uuid.NewString())
	var sched pgtype.Timestamptz
	_ = sched.Scan(time.Now().Add(24 * time.Hour))
	var desc pgtype.Text
	_ = desc.Scan("descripción prueba")
	return sqlc.SocialMeeting{
		ID:            mid,
		GroupID:       gid,
		Title:         "Reunión prueba",
		Description:   desc,
		ScheduledAt:   sched,
		NotifyDiscord: notify,
	}
}

func discordTestConfig(webhook string) sqlc.SocialDiscordIntegration {
	var wh pgtype.Text
	if webhook != "" {
		_ = wh.Scan(webhook)
	}
	return sqlc.SocialDiscordIntegration{WebhookUrl: wh}
}

func TestDiscordNotifier_EnviaConWebhook(t *testing.T) {
	cli := discord.NewMockClient()
	n := NewDiscordMeetingNotifier(
		&fakeDiscordStore{cfg: discordTestConfig("https://discord.com/api/webhooks/1/x")},
		cli,
	)
	n.OnMeetingCreated(context.Background(), discordTestMeeting(true))
	if cli.Count != 1 {
		t.Fatalf("se esperaba 1 envío, got %d", cli.Count)
	}
	if cli.LastMessage == nil || cli.LastMessage.Title != "Reunión prueba" || !cli.LastMessage.HasSchedule {
		t.Fatalf("mensaje inesperado: %+v", cli.LastMessage)
	}
}

func TestDiscordNotifier_Skips(t *testing.T) {
	// notify_discord=false → skip aunque haya webhook
	cli := discord.NewMockClient()
	n := NewDiscordMeetingNotifier(
		&fakeDiscordStore{cfg: discordTestConfig("https://discord.com/api/webhooks/1/x")},
		cli,
	)
	n.OnMeetingCreated(context.Background(), discordTestMeeting(false))
	if cli.Count != 0 {
		t.Fatalf("con notify=false no debe enviar, got %d", cli.Count)
	}

	// sin config en BD → skip sin error
	cli2 := discord.NewMockClient()
	n2 := NewDiscordMeetingNotifier(&fakeDiscordStore{err: pgx.ErrNoRows}, cli2)
	n2.OnMeetingCreated(context.Background(), discordTestMeeting(true))
	if cli2.Count != 0 {
		t.Fatalf("sin config no debe enviar, got %d", cli2.Count)
	}

	// config sin webhook → skip sin error
	cli3 := discord.NewMockClient()
	n3 := NewDiscordMeetingNotifier(&fakeDiscordStore{cfg: discordTestConfig("")}, cli3)
	n3.OnMeetingCreated(context.Background(), discordTestMeeting(true))
	if cli3.Count != 0 {
		t.Fatalf("sin webhook no debe enviar, got %d", cli3.Count)
	}
}

func TestDiscordNotifier_ErrorWebhook_NoPropaga(t *testing.T) {
	cli := discord.NewMockClient()
	cli.SendErr = &discord.WebhookError{Code: 404, Message: "webhook borrado"}
	n := NewDiscordMeetingNotifier(
		&fakeDiscordStore{cfg: discordTestConfig("https://discord.com/api/webhooks/1/x")},
		cli,
	)
	// No debe panic ni retornar error (firma void): solo loguea
	n.OnMeetingCreated(context.Background(), discordTestMeeting(true))
	if cli.Count != 0 {
		t.Fatalf("fallo no debe contar como envío, got %d", cli.Count)
	}
}

func TestMultiNotifier_FanOut(t *testing.T) {
	cli := discord.NewMockClient()
	d := NewDiscordMeetingNotifier(
		&fakeDiscordStore{cfg: discordTestConfig("https://discord.com/api/webhooks/1/x")},
		cli,
	)
	m := NewMultiMeetingNotifier(nil, d, NoopMeetingCreatedNotifier{})
	m.OnMeetingCreated(context.Background(), discordTestMeeting(true))
	if cli.Count != 1 {
		t.Fatalf("fan-out debe llegar a discord, got %d", cli.Count)
	}
}
