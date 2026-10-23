package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

const socialAccountLockPrefix = "social.account-deletion:"

type AccountDeletionGroup struct {
	GroupID           string
	MembershipRemoved bool
	GroupDeleted      bool
}

type AccountDeletionRepository struct {
	pool           *pgxpool.Pool
	gatePool       *pgxpool.Pool
	targetGatePool *pgxpool.Pool
	gateErr        error
	groups         *GroupRepository
}

func NewAccountDeletionRepository(pool *pgxpool.Pool) *AccountDeletionRepository {
	gatePool, err := newSocialGatePool(pool)
	var targetGatePool *pgxpool.Pool
	if err == nil {
		targetGatePool, err = newSocialGatePool(pool)
	}
	return &AccountDeletionRepository{pool: pool, gatePool: gatePool, targetGatePool: targetGatePool, gateErr: err, groups: NewGroupRepository(pool)}
}

func newSocialGatePool(pool *pgxpool.Pool) (*pgxpool.Pool, error) {
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
	if r.targetGatePool != nil {
		r.targetGatePool.Close()
	}
}

func (r *AccountDeletionRepository) AcquireRequest(ctx context.Context, userID string) (func(), bool, error) {
	return r.acquireRequestFrom(ctx, userID, r.gatePool)
}

// AcquireTargetRequest uses a second lock pool because an authenticated Social
// request already holds the actor lock while it checks an affected user. Both
// pools use the same advisory key as Begin, so deletion still drains writes.
func (r *AccountDeletionRepository) AcquireTargetRequest(ctx context.Context, userID string) (func(), bool, error) {
	return r.acquireRequestFrom(ctx, userID, r.targetGatePool)
}

func (r *AccountDeletionRepository) acquireRequestFrom(ctx context.Context, userID string, gatePool *pgxpool.Pool) (func(), bool, error) {
	if r.gateErr != nil {
		return nil, false, r.gateErr
	}
	conn, err := gatePool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	key := socialAccountLockPrefix + strings.TrimSpace(userID)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock_shared(hashtextextended($1, 0))`, key); err != nil {
		discardSocialGateConn(conn)
		return nil, false, err
	}
	release := func() { releaseSocialGateConn(conn, key, true) }
	var active bool
	if err := conn.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM social.account_deletion_users WHERE user_id=$1::uuid)`, userID).Scan(&active); err != nil {
		release()
		return nil, false, err
	}
	if !active {
		release()
		return nil, false, nil
	}
	return release, true, nil
}

// Begin serializes against accepted user requests, writes the tombstone and
// snapshots memberships before any membership row can disappear.
func (r *AccountDeletionRepository) Begin(ctx context.Context, userID string) error {
	if r.gateErr != nil {
		return r.gateErr
	}
	conn, err := r.gatePool.Acquire(ctx)
	if err != nil {
		return err
	}
	key := socialAccountLockPrefix + strings.TrimSpace(userID)
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
		discardSocialGateConn(conn)
		return err
	}
	defer releaseSocialGateConn(conn, key, false)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, `INSERT INTO social.account_deletion_users(user_id,status) VALUES($1::uuid,'pending') ON CONFLICT(user_id) DO NOTHING`, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO social.account_deletion_groups(user_id,group_id)
		SELECT $1::uuid, group_id FROM social.group_memberships WHERE user_id=$1::uuid
		ON CONFLICT(user_id,group_id) DO NOTHING`, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func releaseSocialGateConn(conn *pgxpool.Conn, key string, shared bool) {
	unlockCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	query := `SELECT pg_advisory_unlock(hashtextextended($1, 0))`
	if shared {
		query = `SELECT pg_advisory_unlock_shared(hashtextextended($1, 0))`
	}
	var unlocked bool
	if err := conn.QueryRow(unlockCtx, query, key).Scan(&unlocked); err != nil || !unlocked {
		discardSocialGateConn(conn)
		return
	}
	conn.Release()
}

func discardSocialGateConn(conn *pgxpool.Conn) {
	if conn == nil {
		return
	}
	raw := conn.Hijack()
	if raw != nil {
		_ = raw.Close(context.Background())
	}
}

func (r *AccountDeletionRepository) PendingGroups(ctx context.Context, userID string) ([]AccountDeletionGroup, error) {
	rows, err := r.pool.Query(ctx, `SELECT group_id::text,membership_removed,group_deleted FROM social.account_deletion_groups WHERE user_id=$1::uuid AND status='pending' ORDER BY group_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccountDeletionGroup
	for rows.Next() {
		var item AccountDeletionGroup
		if err := rows.Scan(&item.GroupID, &item.MembershipRemoved, &item.GroupDeleted); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *AccountDeletionRepository) RemoveMembership(ctx context.Context, userID, groupID string) (model.SuccessionResult, error) {
	return r.groups.removeUserFromGroup(ctx, groupID, userID, false)
}

func (r *AccountDeletionRepository) MarkMembershipRemoved(ctx context.Context, userID, groupID string, groupDeleted bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE social.account_deletion_groups SET membership_removed=true,group_deleted=$3,updated_at=now() WHERE user_id=$1::uuid AND group_id=$2::uuid AND status='pending'`, userID, groupID, groupDeleted)
	return err
}

func (r *AccountDeletionRepository) CompleteGroup(ctx context.Context, userID, groupID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE social.account_deletion_groups SET status='complete',updated_at=now(),completed_at=now() WHERE user_id=$1::uuid AND group_id=$2::uuid AND membership_removed=true AND status='pending'`, userID, groupID)
	return err
}

func (r *AccountDeletionRepository) Complete(ctx context.Context, userID string) error {
	var pending bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM social.account_deletion_groups WHERE user_id=$1::uuid AND status<>'complete')`, userID).Scan(&pending); err != nil {
		return err
	}
	if pending {
		return fmt.Errorf("account deletion groups still pending")
	}
	_, err := r.pool.Exec(ctx, `UPDATE social.account_deletion_users SET status='complete',updated_at=now(),completed_at=now() WHERE user_id=$1::uuid`, userID)
	return err
}

func (r *AccountDeletionRepository) IsComplete(ctx context.Context, userID string) (bool, error) {
	var complete bool
	err := r.pool.QueryRow(ctx, `SELECT status='complete' FROM social.account_deletion_users WHERE user_id=$1::uuid`, userID).Scan(&complete)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return complete, err
}

func (r *AccountDeletionRepository) IsActive(ctx context.Context, userID string) (bool, error) {
	var active bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM social.account_deletion_users WHERE user_id=$1::uuid)`, userID).Scan(&active)
	return active, err
}

var _ interface {
	AcquireRequest(context.Context, string) (func(), bool, error)
} = (*AccountDeletionRepository)(nil)
