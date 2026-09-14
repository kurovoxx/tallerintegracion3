package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

type LikeRepository struct {
	pool *pgxpool.Pool
}

func NewLikeRepository(pool *pgxpool.Pool) *LikeRepository {
	return &LikeRepository{pool: pool}
}

func (r *LikeRepository) Create(ctx context.Context, db DBTX, noteID, userID string) (*model.NoteLike, error) {
	if db == nil {
		db = r.pool
	}
	var l model.NoteLike
	err := db.QueryRow(ctx, `INSERT INTO notes.note_likes (note_id, user_id) VALUES ($1,$2) RETURNING id, note_id, user_id, created_at`, noteID, userID).Scan(&l.ID, &l.NoteID, &l.UserID, &l.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("already_liked: %w", err)
		}
		return nil, fmt.Errorf("create like: %w", err)
	}
	return &l, nil
}

func (r *LikeRepository) Delete(ctx context.Context, db DBTX, noteID, userID string) (bool, error) {
	if db == nil {
		db = r.pool
	}
	tag, err := db.Exec(ctx, `DELETE FROM notes.note_likes WHERE note_id=$1 AND user_id=$2`, noteID, userID)
	if err != nil {
		return false, fmt.Errorf("delete like: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *LikeRepository) Exists(ctx context.Context, db DBTX, noteID, userID string) (bool, error) {
	if db == nil {
		db = r.pool
	}
	var exists bool
	err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes.note_likes WHERE note_id=$1 AND user_id=$2)`, noteID, userID).Scan(&exists)
	return exists, err
}

func (r *LikeRepository) CountByNote(ctx context.Context, db DBTX, noteID string) (int, error) {
	if db == nil {
		db = r.pool
	}
	var cnt int
	err := db.QueryRow(ctx, `SELECT count(*) FROM notes.note_likes WHERE note_id=$1`, noteID).Scan(&cnt)
	return cnt, err
}

var _ = pgx.ErrNoRows
