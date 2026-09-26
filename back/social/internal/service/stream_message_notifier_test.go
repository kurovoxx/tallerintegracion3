package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

type fakeMessageStore struct {
	channel sqlc.SocialStreamChannel
	err     error
}

func (f *fakeMessageStore) GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error) {
	if f.err != nil {
		return sqlc.SocialStreamChannel{}, f.err
	}
	return f.channel, nil
}

func announceTestMeeting() sqlc.SocialMeeting {
	var mid, gid, uid pgtype.UUID
	_ = mid.Scan(uuid.NewString())
	_ = gid.Scan(uuid.NewString())
	_ = uid.Scan(uuid.NewString())
	var sched pgtype.Timestamptz
	_ = sched.Scan(time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC))
	var desc pgtype.Text
	_ = desc.Scan("llevar apuntes")
	return sqlc.SocialMeeting{
		ID:              mid,
		GroupID:         gid,
		Title:           "Reunión anuncio",
		Description:     desc,
		ScheduledAt:     sched,
		CreatedByUserID: uid,
		NotifyDiscord:   true,
	}
}

func announceTestChannel() sqlc.SocialStreamChannel {
	return sqlc.SocialStreamChannel{ChannelID: "group-test-123"}
}

func TestFormatMeetingAnnouncement(t *testing.T) {
	text := FormatMeetingAnnouncement("Título", "detalle", time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC), true)
	if !strings.Contains(text, "Título") || !strings.Contains(text, "detalle") || !strings.Contains(text, "2026-09-25") {
		t.Fatalf("anuncio incompleto: %q", text)
	}
	plain := FormatMeetingAnnouncement("Solo título", "", time.Time{}, false)
	if strings.Contains(plain, "🕒") {
		t.Fatalf("sin fecha no debe llevar hora: %q", plain)
	}
}

func TestStreamMessageNotifier_AnunciaConCanal(t *testing.T) {
	cli := stream.NewMockClient()
	n := NewStreamMessageNotifier(&fakeMessageStore{channel: announceTestChannel()}, cli)

	meeting := announceTestMeeting()
	n.OnMeetingCreated(context.Background(), meeting)

	if len(cli.Messages) != 1 {
		t.Fatalf("se esperaba 1 mensaje, got %d", len(cli.Messages))
	}
	msg := cli.Messages[0]
	if msg.ChannelType != "messaging" || msg.ChannelID != "group-test-123" {
		t.Fatalf("destino inesperado: %+v", msg)
	}
	creator, _ := uuid.FromBytes(meeting.CreatedByUserID.Bytes[:])
	if msg.SenderID != creator.String() {
		t.Fatalf("sender debe ser el creador, got %q", msg.SenderID)
	}
	if !strings.Contains(msg.Text, "Reunión anuncio") || !strings.Contains(msg.Text, "2026-09-25") {
		t.Fatalf("texto incompleto: %q", msg.Text)
	}
}

func TestStreamMessageNotifier_Skips(t *testing.T) {
	// sin canal registrado → skip sin error
	cli := stream.NewMockClient()
	n := NewStreamMessageNotifier(&fakeMessageStore{err: pgx.ErrNoRows}, cli)
	n.OnMeetingCreated(context.Background(), announceTestMeeting())
	if len(cli.Messages) != 0 {
		t.Fatalf("sin canal no debe anunciar, got %d", len(cli.Messages))
	}

	// canal con id vacío → skip
	cli2 := stream.NewMockClient()
	n2 := NewStreamMessageNotifier(&fakeMessageStore{channel: sqlc.SocialStreamChannel{}}, cli2)
	n2.OnMeetingCreated(context.Background(), announceTestMeeting())
	if len(cli2.Messages) != 0 {
		t.Fatalf("con channel vacío no debe anunciar, got %d", len(cli2.Messages))
	}
}

func TestStreamMessageNotifier_ErrorStream_NoPropaga(t *testing.T) {
	cli := stream.NewMockClient()
	cli.SendErr = &stream.StreamError{Code: 404, Message: "canal inexistente"}
	n := NewStreamMessageNotifier(&fakeMessageStore{channel: announceTestChannel()}, cli)

	// No debe panic ni propagar: solo loguea
	n.OnMeetingCreated(context.Background(), announceTestMeeting())
	if len(cli.Messages) != 0 {
		t.Fatalf("fallo no debe contar como envío, got %d", len(cli.Messages))
	}
}
