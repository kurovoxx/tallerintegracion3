package repository

import (
	"context"

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
// una fila en meeting_notifications por cada miembro del grupo excepto el creador.
// La creación local es síncrona y obligatoria; las integraciones externas
// (Calendar/Discord/Stream) se disparan después, en background, desde el service.
func (r *MeetingRepository) CreateMeetingWithNotifications(ctx context.Context, arg sqlc.CreateMeetingParams) (sqlc.SocialMeeting, error) {
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

	if err := tx.Commit(ctx); err != nil {
		return sqlc.SocialMeeting{}, err
	}
	return meeting, nil
}

func (r *MeetingRepository) GetMeetingByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialMeeting, error) {
	return r.queries.GetMeetingByID(ctx, id)
}

func (r *MeetingRepository) ListMeetingsByGroup(ctx context.Context, groupID pgtype.UUID) ([]sqlc.SocialMeeting, error) {
	return r.queries.ListMeetingsByGroup(ctx, groupID)
}
