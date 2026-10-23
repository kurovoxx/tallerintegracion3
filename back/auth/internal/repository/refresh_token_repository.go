package repository

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type RefreshTokenRepository struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenRepository(pool *pgxpool.Pool) *RefreshTokenRepository {
	return &RefreshTokenRepository{pool: pool}
}

// RefreshToken representa una fila de identity.refresh_tokens.
type RefreshToken struct {
	ID               string
	UserID           string
	TokenHash        string
	TokenFingerprint []byte
	ExpiresAt        time.Time
	Revoked          bool
	CreatedAt        time.Time
}

// FingerprintRawToken returns the deterministic lookup key for a raw refresh
// token. The raw token has 256 bits of entropy, so storing its SHA-256 digest
// does not make guessing practical; bcrypt remains stored for legacy clients
// and rollback compatibility.
func FingerprintRawToken(raw string) []byte {
	fingerprint := sha256.Sum256([]byte(raw))
	return append([]byte(nil), fingerprint[:]...)
}

// GenerateRawToken genera un refresh token crudo (64 hex chars = 32 bytes) y su hash bcrypt.
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

// Create guarda el hash del refresh token con expires_at.
func (r *RefreshTokenRepository) Create(ctx context.Context, userID string, tokenHash string, fingerprint []byte, expiresAt time.Time) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, `
		INSERT INTO identity.refresh_tokens (user_id, token_hash, token_fingerprint, expires_at, revoked)
		VALUES ($1, $2, $3, $4, false)
		RETURNING id
	`, userID, tokenHash, fingerprint, expiresAt).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create refresh_token: %w", err)
	}
	return id, nil
}

// CountByUser helper para tests
func (r *RefreshTokenRepository) CountByUser(ctx context.Context, userID string) (int, error) {
	var c int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id=$1`, userID).Scan(&c)
	return c, err
}

// ListAll devuelve todos los refresh_tokens, incluyendo sus huellas para diagnóstico.
func (r *RefreshTokenRepository) ListAll(ctx context.Context) ([]*RefreshToken, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, token_hash, token_fingerprint, expires_at, revoked, created_at FROM identity.refresh_tokens`)
	if err != nil {
		return nil, fmt.Errorf("list refresh_tokens: %w", err)
	}
	defer rows.Close()
	var out []*RefreshToken
	for rows.Next() {
		var rt RefreshToken
		if err := rows.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.TokenFingerprint, &rt.ExpiresAt, &rt.Revoked, &rt.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan refresh_token: %w", err)
		}
		out = append(out, &rt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FindByRawToken first uses the indexed SHA-256 fingerprint. Tokens created
// before fingerprints were introduced are found through a bounded legacy
// fallback that considers only active, unrevoked rows. The fallback still uses
// bcrypt and may be slow while valid legacy sessions remain.
func (r *RefreshTokenRepository) FindByRawToken(ctx context.Context, raw string) (*RefreshToken, error) {
	fingerprint := FingerprintRawToken(raw)
	var matched RefreshToken
	err := r.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, token_fingerprint, expires_at, revoked, created_at
		FROM identity.refresh_tokens
		WHERE token_fingerprint = $1
		LIMIT 1
	`, fingerprint).Scan(
		&matched.ID, &matched.UserID, &matched.TokenHash, &matched.TokenFingerprint,
		&matched.ExpiresAt, &matched.Revoked, &matched.CreatedAt,
	)
	if err == nil {
		return &matched, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("find refresh token by fingerprint: %w", err)
	}

	// Legacy bcrypt candidates are materialized first. Closing rows releases
	// the pooled connection before the expensive bcrypt comparisons begin.
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, token_hash, token_fingerprint, expires_at, revoked, created_at
		FROM identity.refresh_tokens
		WHERE token_fingerprint IS NULL AND revoked = false AND expires_at > now()
	`)
	if err != nil {
		return nil, fmt.Errorf("list legacy refresh tokens: %w", err)
	}
	candidates := make([]RefreshToken, 0)
	for rows.Next() {
		var rt RefreshToken
		if err := rows.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.TokenFingerprint, &rt.ExpiresAt, &rt.Revoked, &rt.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, rt)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	for _, rt := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if bcrypt.CompareHashAndPassword([]byte(rt.TokenHash), []byte(raw)) == nil {
			return &rt, nil
		}
	}
	return nil, nil
}

// Revoke marca revoked=true para el id dado. Idempotente.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE identity.refresh_tokens SET revoked=true WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("revoke: %w", err)
	}
	return nil
}

// Rotate revoca el token viejo y crea uno nuevo en una transacción.
// Usa SELECT FOR UPDATE para evitar doble uso concurrente.
func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldID, userID, newHash string, newFingerprint []byte, newExpires time.Time) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Bloquear fila vieja
	// Serialize rotation with password changes so a concurrent reset also
	// revokes any refresh token created by this transaction.
	var lockedUserID string
	if err := tx.QueryRow(ctx, `SELECT id FROM identity.users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedUserID); err != nil {
		return "", fmt.Errorf("lock user: %w", err)
	}
	var revoked bool
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `SELECT revoked, expires_at FROM identity.refresh_tokens WHERE id=$1 FOR UPDATE`, oldID).Scan(&revoked, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", fmt.Errorf("not_found")
		}
		return "", fmt.Errorf("select for update: %w", err)
	}
	if revoked {
		return "", fmt.Errorf("revoked")
	}
	if time.Now().After(expiresAt) {
		return "", fmt.Errorf("expired")
	}
	// Revocar viejo
	if _, err := tx.Exec(ctx, `UPDATE identity.refresh_tokens SET revoked=true WHERE id=$1`, oldID); err != nil {
		return "", fmt.Errorf("revoke: %w", err)
	}
	// Crear nuevo
	var newID string
	err = tx.QueryRow(ctx, `INSERT INTO identity.refresh_tokens (user_id, token_hash, token_fingerprint, expires_at, revoked) VALUES ($1,$2,$3,$4,false) RETURNING id`, userID, newHash, newFingerprint, newExpires).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("create new: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return newID, nil
}
