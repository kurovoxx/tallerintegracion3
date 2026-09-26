package service

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/discord"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// MeetingDiscordStore es lo mínimo que el notifier necesita: leer la config del grupo.
type MeetingDiscordStore interface {
	GetConfigByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialDiscordIntegration, error)
}

// DiscordMeetingNotifier implementa MeetingCreatedNotifier avisando al webhook
// de Discord del grupo. Todo best-effort, en background:
//  1. Si la reunión trae notify_discord=false → skip silencioso.
//  2. Si el grupo no tiene webhook configurada → skip silencioso.
//  3. Si el POST falla → solo log.
//
// Un fallo aquí jamás revierte la reunión ya creada.
type DiscordMeetingNotifier struct {
	store  MeetingDiscordStore
	client discord.Client
}

func NewDiscordMeetingNotifier(store MeetingDiscordStore, client discord.Client) *DiscordMeetingNotifier {
	return &DiscordMeetingNotifier{store: store, client: client}
}

func (n *DiscordMeetingNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	if !meeting.NotifyDiscord {
		return
	}
	cfg, err := n.store.GetConfigByGroup(ctx, meeting.GroupID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			log.Printf("discord sync: grupo sin webhook configurado, skip")
			return
		}
		log.Printf("discord sync: no se pudo leer config: %v", err)
		return
	}
	if !cfg.WebhookUrl.Valid || cfg.WebhookUrl.String == "" {
		log.Printf("discord sync: grupo sin webhook configurada, skip")
		return
	}

	description := ""
	if meeting.Description.Valid {
		description = meeting.Description.String
	}
	msg := discord.Message{
		Title:       meeting.Title,
		Description: description,
	}
	if meeting.ScheduledAt.Valid {
		msg.ScheduledAt = meeting.ScheduledAt.Time
		msg.HasSchedule = true
	}
	if err := n.client.SendMeetingCreated(ctx, cfg.WebhookUrl.String, msg); err != nil {
		log.Printf("discord sync: no se pudo notificar al webhook: %v", err)
		return
	}
	log.Printf("discord sync: reunión notificada al webhook del grupo")
}

// MultiMeetingNotifier hace fan-out a varios notifiers (Calendar + Discord + Stream).
// Cada uno corre en su propia goroutine con timeout y recover propios:
// uno lento o en panic no retrasa ni tumba a los demás ni al servidor.
// El llamador (MeetingService) ya corre esto en background vía GoBestEffort.
type MultiMeetingNotifier struct {
	notifiers []MeetingCreatedNotifier
}

func NewMultiMeetingNotifier(notifiers ...MeetingCreatedNotifier) *MultiMeetingNotifier {
	return &MultiMeetingNotifier{notifiers: notifiers}
}

func (m *MultiMeetingNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	var wg sync.WaitGroup
	for i, n := range m.notifiers {
		if n == nil {
			continue
		}
		wg.Add(1)
		go func(idx int, child MeetingCreatedNotifier) {
			defer wg.Done()
			name := "meetings[" + strconv.Itoa(idx) + "]"
			RunBestEffortChild(name, ctx, func(childCtx context.Context) {
				child.OnMeetingCreated(childCtx, meeting)
			})
		}(i, n)
	}
	wg.Wait()
}
