package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

// GenerateRawToken genera un refresh token crudo (64 hex chars = 32 bytes) y su hash bcrypt.
// Retorna raw, hash, error.
func GenerateRawToken() (raw string, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("rand: %w", err)
	}
	raw = hex.EncodeToString(b)
	h, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", "", fmt.Errorf("hash refresh: %w", err)
	}
	return raw, string(h), nil
}

// Create guarda el hash del refresh token con expires_at. Usa SimpleProtocol ya configurado en pool.
func (r *RefreshTokenRepository) Create(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at, revoked)
		VALUES ($1, $2, $3, false)
		RETURNING id
	`, userID, tokenHash, expiresAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create refresh_token: %w", err)
	}
	return id, nil
}

// Exists helper para tests (no usado en login)
func (r *RefreshTokenRepository) CountByUser(ctx context.Context, userID string) (int, error) {
	var c int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id=$1`, userID).Scan(&c)
	return c, err
}
