package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type HoursRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewHoursRepository(pool *pgxpool.Pool) *HoursRepository {
	return &HoursRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *HoursRepository) GetTaskByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialSprintSheetTask, error) {
	return r.queries.GetSprintSheetTaskByID(ctx, id)
}

func (r *HoursRepository) GetByTaskAndDate(ctx context.Context, arg sqlc.GetDailyHoursByTaskAndDateParams) (sqlc.SocialSprintSheetDailyHour, error) {
	return r.queries.GetDailyHoursByTaskAndDate(ctx, arg)
}

func (r *HoursRepository) ListByTask(ctx context.Context, arg sqlc.ListDailyHoursByTaskParams) ([]sqlc.SocialSprintSheetDailyHour, error) {
	return r.queries.ListDailyHoursByTask(ctx, arg)
}

func (r *HoursRepository) Create(ctx context.Context, arg sqlc.CreateDailyHoursParams) (sqlc.SocialSprintSheetDailyHour, error) {
	return r.queries.CreateDailyHours(ctx, arg)
}

func (r *HoursRepository) Update(ctx context.Context, arg sqlc.UpdateDailyHoursParams) (sqlc.SocialSprintSheetDailyHour, error) {
	return r.queries.UpdateDailyHours(ctx, arg)
}
