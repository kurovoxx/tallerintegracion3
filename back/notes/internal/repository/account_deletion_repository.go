package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

const notesAccountLockPrefix = "notes.account-deletion:"

type AccountDeletionRepository struct {
	pool     *pgxpool.Pool
	gatePool *pgxpool.Pool
	gateErr  error
}

func NewAccountDeletionRepository(pool *pgxpool.Pool) *AccountDeletionRepository {
	gatePool, err := newAccountGatePool(pool)
	return &AccountDeletionRepository{pool: pool, gatePool: gatePool, gateErr: err}
}

func newAccountGatePool(pool *pgxpool.Pool) (*pgxpool.Pool, error) {
	if pool == nil {
		return nil, errors.New("account deletion pool is required")
	}
	cfg := pool.Config().Copy()
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	return pgxpool.NewWithConfig(context.Background(), cfg)
}

func (r *AccountDeletionRepository) Close() {
	if r.gatePool != nil {
		r.gatePool.Close()
	}
}

func (r *AccountDeletionRepository) AcquireRequest(ctx context.Context, userID string) (func(), bool, error) {
	if r.gateErr != nil {
		return nil, false, r.gateErr
	}
	conn, err := r.gatePool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := notesAccountLockPrefix + strings.TrimSpace(userID)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1, 0))`, key); err != nil {
		discardAccountGateConn(conn)
		return nil, false, err
	}
	release := func() { releaseAccountGateConn(conn, key, true) }
	var active bool
	if err := conn.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM notes.account_deletion_users WHERE user_id=$1::uuid)`, userID).Scan(&active); err != nil {
		release()
		return nil, false, err
	}
	if !active {
		release()
		return nil, false, nil
	}
	return release, true, nil
}

func (r *AccountDeletionRepository) Begin(ctx context.Context, userID string) (bool, error) {
	if r.gateErr != nil {
		return false, r.gateErr
	}
	conn, err := r.gatePool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	key := notesAccountLockPrefix + strings.TrimSpace(userID)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		discardAccountGateConn(conn)
		return false, err
	}
	defer releaseAccountGateConn(conn, key, false)
	_, err = conn.Exec(ctx, `INSERT INTO notes.account_deletion_users(user_id,status) VALUES($1::uuid,'pending') ON CONFLICT(user_id) DO NOTHING`, userID)
	if err != nil {
		return false, err
	}
	var complete bool
	err = conn.QueryRow(ctx, `SELECT status='complete' FROM notes.account_deletion_users WHERE user_id=$1::uuid`, userID).Scan(&complete)
	return complete, err
}

func (r *AccountDeletionRepository) Complete(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE notes.account_deletion_users SET status='complete',updated_at=now(),completed_at=now() WHERE user_id=$1::uuid`, userID)
	return err
}

func (r *AccountDeletionRepository) IsActive(ctx context.Context, userID string) (bool, error) {
	var active bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM notes.account_deletion_users WHERE user_id=$1::uuid)`, userID).Scan(&active)
	return active, err
}

func (r *AccountDeletionRepository) SaveGroupWork(ctx context.Context, userID, groupID string, work []model.AccountDeletionGroupNote, groupDeleted bool) error {
	for _, item := range work {
		deleted := groupDeleted || item.GroupDeleted
		_, err := r.pool.Exec(ctx, `
			INSERT INTO notes.account_deletion_group_notes(user_id,group_id,note_id,group_deleted)
			VALUES($1::uuid,$2::uuid,$3::uuid,$4)
			ON CONFLICT(user_id,group_id,note_id) DO UPDATE
			SET group_deleted=notes.account_deletion_group_notes.group_deleted OR EXCLUDED.group_deleted,
			    updated_at=now()`, userID, groupID, item.NoteID, deleted)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *AccountDeletionRepository) PendingGroupWork(ctx context.Context, userID, groupID string) ([]model.AccountDeletionGroupNote, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT note_id::text,group_deleted FROM notes.account_deletion_group_notes
		WHERE user_id=$1::uuid AND group_id=$2::uuid ORDER BY note_id`, userID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.AccountDeletionGroupNote
	for rows.Next() {
		var item model.AccountDeletionGroupNote
		if err := rows.Scan(&item.NoteID, &item.GroupDeleted); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *AccountDeletionRepository) CompleteGroupWork(ctx context.Context, userID, groupID, noteID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notes.account_deletion_group_notes WHERE user_id=$1::uuid AND group_id=$2::uuid AND note_id=$3::uuid`, userID, groupID, noteID)
	return err
}

func releaseAccountGateConn(conn *pgxpool.Conn, key string, shared bool) {
	unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	query := `SELECT pg_advisory_unlock(hashtextextended($1, 0))`
	if shared {
		query = `SELECT pg_advisory_unlock_shared(hashtextextended($1, 0))`
	}
	var unlocked bool
	if err := conn.QueryRow(unlockCtx, query, key).Scan(&unlocked); err != nil || !unlocked {
		discardAccountGateConn(conn)
		return
	}
	conn.Release()
}

func discardAccountGateConn(conn *pgxpool.Conn) {
	if conn == nil {
		return
	}
	raw := conn.Hijack()
	if raw != nil {
		_ = raw.Close(context.Background())
	}
}
