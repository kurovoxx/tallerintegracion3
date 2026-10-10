package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type MeetingRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewMeetingRepository(pool *pgxpool.Pool) *MeetingRepository {
	return &MeetingRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *MeetingRepository) GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	return r.queries.GetGroupByID(ctx, id)
}

func (r *MeetingRepository) IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error) {
	return r.queries.IsGroupMember(ctx, sqlc.IsGroupMemberParams{
		GroupID: groupID,
		UserID:  userID,
	})
}

// CreateMeetingWithNotifications crea la reunión y, en la misma transacción,
// una fila en meeting_notifications por cada miembro del grupo excepto el creador,
// más una fila en meeting_attendees por cada email invitado (ya validados y
// deduplicados por el service; el ON CONFLICT es red de seguridad anti-carrera).
// La creación local es síncrona y obligatoria; las integraciones externas
// (Calendar/Discord/Stream) se disparan después, en background, desde el service.
func (r *MeetingRepository) CreateMeetingWithNotifications(ctx context.Context, arg sqlc.CreateMeetingParams, attendeeEmails []string) (sqlc.SocialMeeting, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.queries.WithTx(tx)

	meeting, err := qtx.CreateMeeting(ctx, arg)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}

	memberIDs, err := qtx.ListGroupMemberUserIDs(ctx, arg.GroupID)
	if err != nil {
		return sqlc.SocialMeeting{}, err
	}

	for _, memberID := range memberIDs {
		if !memberID.Valid || !arg.CreatedByUserID.Valid {
			continue
		}
		if memberID.Bytes == arg.CreatedByUserID.Bytes {
			continue // no notificar al creador
		}
		if _, err := qtx.CreateMeetingNotification(ctx, sqlc.CreateMeetingNotificationParams{
			MeetingID: meeting.ID,
			UserID:    memberID,
		}); err != nil {
			return sqlc.SocialMeeting{}, err
		}
	}

	for _, email := range attendeeEmails {
		if _, err := qtx.CreateMeetingAttendee(ctx, sqlc.CreateMeetingAttendeeParams{
			MeetingID: meeting.ID,
			Email:     email,
		}); err != nil {
			// ON CONFLICT DO NOTHING no devuelve fila en carrera: pgx.ErrNoRows
			// significa que otro writer ganó; no es error.
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return sqlc.SocialMeeting{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return sqlc.SocialMeeting{}, err
	}
	return meeting, nil
}

func (r *MeetingRepository) ListAttendeesByMeeting(ctx context.Context, meetingID pgtype.UUID) ([]sqlc.SocialMeetingAttendee, error) {
	return r.queries.ListMeetingAttendees(ctx, meetingID)
}

func (r *MeetingRepository) GetMeetingByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialMeeting, error) {
	return r.queries.GetMeetingByID(ctx, id)
}

func (r *MeetingRepository) ListMeetingsByGroup(ctx context.Context, groupID pgtype.UUID) ([]sqlc.SocialMeeting, error) {
	return r.queries.ListMeetingsByGroup(ctx, groupID)
}

// SetCalendarEventID guarda el google_calendar_event_id tras un sync exitoso.
// Se llama desde el notifier en background; un fallo aquí solo se loguea.
func (r *MeetingRepository) SetCalendarEventID(ctx context.Context, meetingID pgtype.UUID, eventID string) (sqlc.SocialMeeting, error) {
	var event pgtype.Text
	if err := event.Scan(eventID); err != nil {
		return sqlc.SocialMeeting{}, err
	}
	return r.queries.UpdateMeetingCalendarEventID(ctx, sqlc.UpdateMeetingCalendarEventIDParams{
		ID:                    meetingID,
		GoogleCalendarEventID: event,
	})
}
