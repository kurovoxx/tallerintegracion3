package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type SprintRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewSprintRepository(pool *pgxpool.Pool) *SprintRepository {
	return &SprintRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *SprintRepository) GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	return r.queries.GetGroupByID(ctx, id)
}

func (r *SprintRepository) IsMember(ctx context.Context, groupID, userID pgtype.UUID) (bool, error) {
	return r.queries.IsGroupMember(ctx, sqlc.IsGroupMemberParams{
		GroupID: groupID,
		UserID:  userID,
	})
}

func (r *SprintRepository) GetSheetByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheet, error) {
	return r.queries.GetSheetByID(ctx, id)
}

func (r *SprintRepository) ListSheetsByGroup(ctx context.Context, groupID pgtype.UUID) ([]sqlc.SocialSprintSheet, error) {
	return r.queries.ListSheetsByGroup(ctx, groupID)
}

func (r *SprintRepository) CreateSheet(ctx context.Context, arg sqlc.CreateSheetParams) (sqlc.SocialSprintSheet, error) {
	return r.queries.CreateSheet(ctx, arg)
}

func (r *SprintRepository) GetTaskByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error) {
	return r.queries.GetSprintSheetTaskByID(ctx, id)
}

func (r *SprintRepository) CreateTask(ctx context.Context, arg sqlc.CreateSprintSheetTaskParams) (sqlc.SocialSprintSheetTask, error) {
	return r.queries.CreateSprintSheetTask(ctx, arg)
}

func (r *SprintRepository) ListTasksByGroup(ctx context.Context, arg sqlc.ListSprintSheetTasksByGroupParams) ([]sqlc.ListSprintSheetTasksByGroupRow, error) {
	return r.queries.ListSprintSheetTasksByGroup(ctx, arg)
}

func (r *SprintRepository) UpdateTask(ctx context.Context, arg sqlc.UpdateSprintSheetTaskParams) (sqlc.SocialSprintSheetTask, error) {
	return r.queries.UpdateSprintSheetTask(ctx, arg)
}

func (r *SprintRepository) DeleteTask(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error) {
	return r.queries.DeleteSprintSheetTask(ctx, id)
}

func (r *SprintRepository) UpdateSheet(ctx context.Context, arg sqlc.UpdateSheetParams) (sqlc.SocialSprintSheet, error) {
	return r.queries.UpdateSheet(ctx, arg)
}
