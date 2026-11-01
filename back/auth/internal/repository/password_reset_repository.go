package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type PasswordResetRepository struct{ pool *pgxpool.Pool }

func NewPasswordResetRepository(pool *pgxpool.Pool) *PasswordResetRepository {
	return &PasswordResetRepository{pool: pool}
}

// Upsert and cooldown are atomic, including requests handled by different replicas.
func (r *PasswordResetRepository) Issue(ctx context.Context, userID, hash string) (bool, error) {
	result, err := r.pool.Exec(ctx, `
 INSERT INTO identity.password_resets (user_id, code_hash, expires_at)
 VALUES ($1, $2, now() + interval '15 minutes')
 ON CONFLICT (user_id) DO UPDATE SET code_hash = EXCLUDED.code_hash,
 expires_at = EXCLUDED.expires_at, requested_at = now(), attempts = 0
 WHERE identity.password_resets.requested_at <= now() - interval '60 seconds'`, userID, hash)
	return result.RowsAffected() == 1, err
}

// Consume, change password and revoke refresh tokens in a single transaction.
// Locking the reset row prevents concurrent redemption and races with resends.
func (r *PasswordResetRepository) Complete(ctx context.Context, email, code, passwordHash string) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var userID, codeHash string
	var expires time.Time
	var attempts int
	err = tx.QueryRow(ctx, `SELECT p.user_id, p.code_hash, p.expires_at, p.attempts
 FROM identity.password_resets p JOIN identity.users u ON u.id = p.user_id
 WHERE u.email = $1 FOR UPDATE OF p`, email).Scan(&userID, &codeHash, &expires, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !time.Now().Before(expires) || attempts >= 5 {
		return false, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(code)) != nil {
		if _, err = tx.Exec(ctx, `UPDATE identity.password_resets SET attempts = attempts + 1 WHERE user_id = $1`, userID); err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.users SET password_hash = $2 WHERE id = $1`, userID, passwordHash); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE identity.refresh_tokens SET revoked = true WHERE user_id = $1 AND NOT revoked`, userID); err != nil {
		return false, err
	}
	// Keep the row to retain the resend cooldown after successful redemption.
	if _, err = tx.Exec(ctx, `UPDATE identity.password_resets SET attempts = 5, expires_at = now() WHERE user_id = $1`, userID); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
