package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

type TodoRepository struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
}

func NewTodoRepository(pool *pgxpool.Pool) *TodoRepository {
	return &TodoRepository{
		pool:    pool,
		queries: sqlc.New(pool),
	}
}

func (r *TodoRepository) GetGroupByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialGroup, error) {
	return r.queries.GetGroupByID(ctx, id)
}

func (r *TodoRepository) GetBoardByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialTodoBoard, error) {
	return r.queries.GetBoardByID(ctx, id)
}

func (r *TodoRepository) ListBoardsByGroup(ctx context.Context, groupID pgtype.UUID) ([]sqlc.SocialTodoBoard, error) {
	return r.queries.ListBoardsByGroup(ctx, groupID)
}

func (r *TodoRepository) CreateBoard(ctx context.Context, arg sqlc.CreateBoardParams) (sqlc.SocialTodoBoard, error) {
	return r.queries.CreateBoard(ctx, arg)
}

func (r *TodoRepository) GetTaskByID(ctx context.Context, id pgtype.UUID) (sqlc.SocialTodoTask, error) {
	return r.queries.GetTodoTaskByID(ctx, id)
}

func (r *TodoRepository) CreateTask(ctx context.Context, arg sqlc.CreateTodoTaskParams) (sqlc.SocialTodoTask, error) {
	return r.queries.CreateTodoTask(ctx, arg)
}

func (r *TodoRepository) ListTasksByGroup(ctx context.Context, arg sqlc.ListTodoTasksByGroupParams) ([]sqlc.ListTodoTasksByGroupRow, error) {
	return r.queries.ListTodoTasksByGroup(ctx, arg)
}

func (r *TodoRepository) UpdateTask(ctx context.Context, arg sqlc.UpdateTodoTaskParams) (sqlc.SocialTodoTask, error) {
	return r.queries.UpdateTodoTask(ctx, arg)
}

func (r *TodoRepository) DeleteTask(ctx context.Context, id pgtype.UUID) (sqlc.SocialTodoTask, error) {
	return r.queries.DeleteTodoTask(ctx, id)
}
