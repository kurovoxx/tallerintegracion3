package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
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

// LikeAtomic inserta like y incrementa likes_count en una única transacción PG.
// Maneja 23505 como already_liked.
func (r *LikeRepository) LikeAtomic(ctx context.Context, noteID, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `INSERT INTO notes.note_likes (note_id, user_id) VALUES ($1,$2)`, noteID, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("already_liked: %w", err)
		}
		if strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate key") {
			return fmt.Errorf("already_liked: %w", err)
		}
		return fmt.Errorf("insert like: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE notes.notes SET likes_count = likes_count + 1, updated_at = now() WHERE id = $1`, noteID)
	if err != nil {
		return fmt.Errorf("inc likes: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit like: %w", err)
	}
	return nil
}

// UnlikeAtomic elimina like y decrementa likes_count de forma atómica.
// Si no se borró ninguna fila, no decrementa (idempotente).
func (r *LikeRepository) UnlikeAtomic(ctx context.Context, noteID, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `DELETE FROM notes.note_likes WHERE note_id=$1 AND user_id=$2`, noteID, userID)
	if err != nil {
		return fmt.Errorf("delete like: %w", err)
	}
	if tag.RowsAffected() > 0 {
		_, err = tx.Exec(ctx, `UPDATE notes.notes SET likes_count = GREATEST(likes_count - 1, 0), updated_at = now() WHERE id = $1`, noteID)
		if err != nil {
			return fmt.Errorf("dec likes: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit unlike: %w", err)
	}
	return nil
}

var _ = pgx.ErrNoRows
