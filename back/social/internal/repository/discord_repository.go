package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type DiscordRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewDiscordRepository(pool *pgxpool.Pool) *DiscordRepository {
	return &DiscordRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *DiscordRepository) GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	return r.queries.GetGroupByID(ctx, id)
}

func (r *DiscordRepository) GetMemberRole(ctx context.Context, groupID, userID pgtype.UUID) (string, error) {
	return r.queries.GetGroupMemberRole(ctx, sqlc.GetGroupMemberRoleParams{
		GroupID: groupID,
		UserID:  userID,
	})
}

func (r *DiscordRepository) UpsertConfig(ctx context.Context, arg sqlc.UpsertDiscordConfigParams) (sqlc.SocialDiscordIntegration, error) {
	return r.queries.UpsertDiscordConfig(ctx, arg)
}

func (r *DiscordRepository) GetConfigByGroup(ctx context.Context, groupID pgtype.UUID) (sqlc.SocialDiscordIntegration, error) {
	return r.queries.GetDiscordConfigByGroup(ctx, groupID)
}
