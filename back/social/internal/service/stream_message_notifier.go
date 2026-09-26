package service

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

// StreamMessageStore es lo mínimo que el notifier necesita: el canal del grupo.
type StreamMessageStore interface {
	GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error)
}

// FormatMeetingAnnouncement arma el texto del anuncio. Exportada para tests.
func FormatMeetingAnnouncement(title, description string, scheduled time.Time, hasSchedule bool) string {
	var b strings.Builder
	b.WriteString("📅 **Nueva reunión agendada**\n")
	b.WriteString("**" + strings.TrimSpace(title) + "**")
	if hasSchedule {
		b.WriteString("\n🕒 " + scheduled.Format("2006-01-02 15:04 MST"))
	}
	if strings.TrimSpace(description) != "" {
		b.WriteString("\n" + strings.TrimSpace(description))
	}
	return b.String()
}

// StreamMessageNotifier implementa MeetingCreatedNotifier anunciando la reunión
// en el canal de Stream del grupo. Todo best-effort, en background:
//  1. Si el grupo no tiene canal registrado → skip silencioso.
//  2. Si Stream falla → solo log.
//
// Un fallo aquí jamás revierte la reunión ya creada.
// Limitación conocida: la creación del canal también es async; si la reunión
// se agenda antes de que exista el canal en Stream, el anuncio se pierde (log).
type StreamMessageNotifier struct {
	store  StreamMessageStore
	client stream.Client
}

func NewStreamMessageNotifier(store StreamMessageStore, client stream.Client) *StreamMessageNotifier {
	return &StreamMessageNotifier{store: store, client: client}
}

func (n *StreamMessageNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	ch, err := n.store.GetChannelByGroup(ctx, meeting.GroupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("stream announce: grupo sin canal registrado, skip")
			return
		}
		log.Printf("stream announce: no se pudo leer canal del grupo: %v", err)
		return
	}
	if strings.TrimSpace(ch.ChannelID) == "" {
		log.Printf("stream announce: grupo sin canal registrado, skip")
		return
	}
	if !meeting.CreatedByUserID.Valid {
		log.Printf("stream announce: creador inválido, skip")
		return
	}
	creatorID, err := uuid.FromBytes(meeting.CreatedByUserID.Bytes[:])
	if err != nil {
		log.Printf("stream announce: creador inválido, skip")
		return
	}

	description := ""
	if meeting.Description.Valid {
		description = meeting.Description.String
	}
	var scheduled time.Time
	hasSchedule := false
	if meeting.ScheduledAt.Valid {
		scheduled = meeting.ScheduledAt.Time
		hasSchedule = true
	}
	text := FormatMeetingAnnouncement(meeting.Title, description, scheduled, hasSchedule)
	if err := n.client.SendMessage(ctx, ChannelTypeStream, ch.ChannelID, creatorID.String(), text); err != nil {
		log.Printf("stream announce: no se pudo publicar el anuncio: %v", err)
		return
	}
	log.Printf("stream announce: reunión anunciada en canal %s", ch.ChannelID)
}
