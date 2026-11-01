package service

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// MeetingMailStore es lo mínimo que el notifier necesita: emails invitados
// y nombre del grupo para el asunto.
type MeetingMailStore interface {
	GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error)
	ListAttendeesByMeeting(ctx context.Context, meetingID pgtype.UUID) ([]sqlc.SocialMeetingAttendee, error)
}

// MailSender envía el aviso. *mail.MeetingMailer es la implementación real;
// en tests se usa un falso que captura.
type MailSender interface {
	SendMeetingInvite(ctx context.Context, to []string, groupName, title string, scheduled time.Time, hasSchedule bool, description string) error
}

// MeetingMailNotifier implementa MeetingCreatedNotifier avisando por mail
// a los invitados (tarea 3_5_15). Todo best-effort: sin invitados hace skip;
// si el envío falla solo se loguea. Un fallo jamás revierte la reunión.
type MeetingMailNotifier struct {
	store MeetingMailStore
	mail  MailSender
}

func NewMeetingMailNotifier(store MeetingMailStore, mail MailSender) *MeetingMailNotifier {
	return &MeetingMailNotifier{store: store, mail: mail}
}

func (n *MeetingMailNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	if n == nil || n.store == nil || n.mail == nil {
		return
	}
	rows, err := n.store.ListAttendeesByMeeting(ctx, meeting.ID)
	if err != nil {
		log.Printf("mail notify: no se pudieron leer invitados: %v", err)
		return
	}
	to := make([]string, 0, len(rows))
	for _, r := range rows {
		if e := strings.TrimSpace(r.Email); e != "" {
			to = append(to, e)
		}
	}
	if len(to) == 0 {
		return // nadie a quien avisar por mail (miembros usan in-app)
	}
	var groupName string
	if meeting.GroupID.Valid {
		if g, err := n.store.GetGroupByID(ctx, meeting.GroupID); err == nil {
			groupName = g.Name
		}
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
	if err := n.mail.SendMeetingInvite(ctx, to, groupName, meeting.Title, scheduled, hasSchedule, description); err != nil {
		log.Printf("mail notify: no se pudo enviar aviso: %v", err)
		return
	}
	log.Printf("mail notify: aviso enviado a %d invitado(s)", len(to))
}
