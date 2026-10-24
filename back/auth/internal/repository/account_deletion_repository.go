package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const identityAccountLockPrefix = "identity.account-deletion:"

var ErrAccountNotFound = errors.New("account not found")

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

type AccountDeletionState struct {
	Status    string
	LastError string
	UpdatedAt time.Time
}

// AcquireRequest holds a shared session lock through the complete HTTP
// handler. BeginAccountDeletion takes the matching exclusive lock, so it waits
// for accepted requests to finish before making the durable tombstone visible.
func (r *AccountDeletionRepository) AcquireRequest(ctx context.Context, userID string) (func(), bool, error) {
	if r.gateErr != nil {
		return nil, false, r.gateErr
	}
	conn, err := r.gatePool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := identityAccountLockPrefix + strings.TrimSpace(userID)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1, 0))`, key); err != nil {
		discardAccountGateConn(conn)
		return nil, false, err
	}
	release := func() { releaseAccountGateConn(conn, key, true) }
	var active bool
	err = conn.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM identity.users WHERE id = $1::uuid)
		   AND NOT EXISTS (SELECT 1 FROM identity.account_deletions WHERE user_id = $1::uuid)
	`, userID).Scan(&active)
	if err != nil {
		release()
		return nil, false, err
	}
	if !active {
		release()
		return nil, false, nil
	}
	return release, true, nil
}

func (r *AccountDeletionRepository) WithExclusiveUserLock(ctx context.Context, userID string, fn func() error) error {
	if r.gateErr != nil {
		return r.gateErr
	}
	conn, err := r.gatePool.Acquire(ctx)
	if err != nil {
		return err
	}
	key := identityAccountLockPrefix + strings.TrimSpace(userID)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		discardAccountGateConn(conn)
		return err
	}
	defer releaseAccountGateConn(conn, key, false)
	return fn()
}

func releaseAccountGateConn(conn *pgxpool.Conn, key string, shared bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	query := `SELECT pg_advisory_unlock(hashtextextended($1, 0))`
	if shared {
		query = `SELECT pg_advisory_unlock_shared(hashtextextended($1, 0))`
	}
	var unlocked bool
	if err := conn.QueryRow(ctx, query, key).Scan(&unlocked); err != nil || !unlocked {
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

func (r *AccountDeletionRepository) Begin(ctx context.Context, userID string) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO identity.account_deletions (user_id, status)
		SELECT id, 'pending' FROM identity.users WHERE id = $1::uuid
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("begin account deletion: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.account_deletions WHERE user_id=$1::uuid)`, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrAccountNotFound
		}
	}
	return nil
}

func (r *AccountDeletionRepository) State(ctx context.Context, userID string) (AccountDeletionState, bool, error) {
	var state AccountDeletionState
	err := r.pool.QueryRow(ctx, `SELECT status, COALESCE(last_error,''), updated_at FROM identity.account_deletions WHERE user_id=$1::uuid`, userID).
		Scan(&state.Status, &state.LastError, &state.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountDeletionState{}, false, nil
	}
	if err != nil {
		return AccountDeletionState{}, false, err
	}
	return state, true, nil
}

func (r *AccountDeletionRepository) RecordError(ctx context.Context, userID string, failure error) error {
	message := "cleanup incomplete"
	if failure != nil {
		message = failure.Error()
		if len(message) > 500 {
			message = message[:500]
		}
	}
	_, err := r.pool.Exec(ctx, `UPDATE identity.account_deletions SET last_error=$2, updated_at=now() WHERE user_id=$1::uuid AND status='pending'`, userID, message)
	return err
}

// CompleteAndDelete keeps the anti-JWT tombstone while deleting user-owned
// Identity rows in one transaction. OAuth rows are retained until this point.
func (r *AccountDeletionRepository) CompleteAndDelete(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE identity.account_deletions SET status='complete', last_error=NULL, updated_at=now(), completed_at=now() WHERE user_id=$1::uuid`, userID); err != nil {
		return fmt.Errorf("mark account deletion complete: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM identity.users WHERE id=$1::uuid`, userID); err != nil {
		return fmt.Errorf("delete identity user: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit account deletion: %w", err)
	}
	return nil
}

func (r *AccountDeletionRepository) PendingUsers(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `SELECT user_id::text FROM identity.account_deletions WHERE status='pending' ORDER BY updated_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		users = append(users, userID)
	}
	return users, rows.Err()
}
