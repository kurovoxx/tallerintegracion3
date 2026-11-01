package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/calendar"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// MeetingCalendarStore es lo mínimo que el notifier necesita: leer invitados
// y persistir el sync.
type MeetingCalendarStore interface {
	SetCalendarEventID(ctx context.Context, meetingID pgtype.UUID, eventID string) (sqlc.SocialMeeting, error)
	ListAttendeesByMeeting(ctx context.Context, meetingID pgtype.UUID) ([]sqlc.SocialMeetingAttendee, error)
}

// CalendarMeetingNotifier implementa MeetingCreatedNotifier con sync real a Google Calendar.
// Flujo (todo best-effort, en background):
//  1. Pide access token del creador a Auth (endpoint interno, con refresh automático).
//  2. Crea el evento en el calendario primario del creador.
//  3. Persiste el google_calendar_event_id en social.meetings.
//  4. Ante 401/403 de Google, reporta la revocación a Auth.
//
// Un fallo en cualquier paso solo se loguea: jamás revierte la reunión ya creada.
type CalendarMeetingNotifier struct {
	gateway CalendarTokenGateway
	client  calendar.Client
	store   MeetingCalendarStore
}

func NewCalendarMeetingNotifier(gateway CalendarTokenGateway, client calendar.Client, store MeetingCalendarStore) *CalendarMeetingNotifier {
	return &CalendarMeetingNotifier{gateway: gateway, client: client, store: store}
}

func (n *CalendarMeetingNotifier) OnMeetingCreated(ctx context.Context, meeting sqlc.SocialMeeting) {
	creatorID, err := uuid.FromBytes(meeting.CreatedByUserID.Bytes[:])
	if err != nil || !meeting.CreatedByUserID.Valid {
		log.Printf("calendar sync: created_by inválido en meeting, skip")
		return
	}
	userID := creatorID.String()

	token, err := n.gateway.GetToken(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrCalendarNotConnected) {
			log.Printf("calendar sync: usuario %s sin Calendar conectado, skip meeting", userID)
			return
		}
		log.Printf("calendar sync: no se pudo obtener token para %s: %v", userID, err)
		return
	}

	description := ""
	if meeting.Description.Valid {
		description = meeting.Description.String
	}
	start := time.Now().Add(5 * time.Minute)
	if meeting.ScheduledAt.Valid {
		start = meeting.ScheduledAt.Time
	}
	// Invitados persistidos al agendar (best-effort: si la lectura falla,
	// el evento se crea sin attendees en vez de abortar el sync).
	var attendees []string
	if rows, err := n.store.ListAttendeesByMeeting(ctx, meeting.ID); err != nil {
		log.Printf("calendar sync: no se pudieron leer invitados: %v", err)
	} else {
		for _, r := range rows {
			attendees = append(attendees, r.Email)
		}
	}
	eventID, err := n.client.CreateEvent(ctx, token, calendar.Event{
		Summary:     meeting.Title,
		Description: description,
		Start:       start,
		Attendees:   attendees,
	})
	if err != nil {
		if calendar.IsUnauthorized(err) {
			log.Printf("calendar sync: Google rechazó el token de %s, reportando revocación: %v", userID, err)
			if repErr := n.gateway.ReportRevoked(ctx, userID); repErr != nil {
				log.Printf("calendar sync: no se pudo reportar revocación de %s: %v", userID, repErr)
			}
			return
		}
		log.Printf("calendar sync: no se pudo crear evento: %v", err)
		return
	}

	ctxPersist, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := n.store.SetCalendarEventID(ctxPersist, meeting.ID, eventID); err != nil {
		log.Printf("calendar sync: evento %s creado pero no se pudo persistir: %v", eventID, err)
		return
	}
	log.Printf("calendar sync: meeting sincronizado con evento %s", eventID)
}
