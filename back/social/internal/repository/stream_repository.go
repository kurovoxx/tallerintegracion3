package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type StreamRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewStreamRepository(pool *pgxpool.Pool) *StreamRepository {
	return &StreamRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *StreamRepository) GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	return r.queries.GetGroupByID(ctx, id)
}

func (r *StreamRepository) GetMemberRole(ctx context.Context, groupID, userID pgtype.UUID) (string, error) {
	return r.queries.GetGroupMemberRole(ctx, sqlc.GetGroupMemberRoleParams{
		GroupID: groupID,
		UserID:  userID,
	})
}

func (r *StreamRepository) GetChannelByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialStreamChannel, error) {
	return r.queries.GetStreamChannelByGroup(ctx, groupID)
}

func (r *StreamRepository) CreateChannel(ctx context.Context, arg sqlc.CreateStreamChannelParams) (sqlc.SocialStreamChannel, error) {
	return r.queries.CreateStreamChannel(ctx, arg)
}
