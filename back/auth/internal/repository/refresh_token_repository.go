package repository

import (
	"context"
	"crypto/rand"
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
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
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

// CountByUser helper para tests
func (r *RefreshTokenRepository) CountByUser(ctx context.Context, userID string) (int, error) {
	var c int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id=$1`, userID).Scan(&c)
	return c, err
}

// ListAll devuelve todos los refresh_tokens (para búsqueda por bcrypt sin índice).
// En producción con muchos tokens, se debería añadir token_sha256 para O(1).
func (r *RefreshTokenRepository) ListAll(ctx context.Context) ([]*RefreshToken, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, token_hash, expires_at, revoked, created_at FROM identity.refresh_tokens`)
	if err != nil {
		return nil, fmt.Errorf("list refresh_tokens: %w", err)
	}
	defer rows.Close()
	var out []*RefreshToken
	for rows.Next() {
		var rt RefreshToken
		if err := rows.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.Revoked, &rt.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan refresh_token: %w", err)
		}
		out = append(out, &rt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// FindByRawToken busca el refresh_token cuyo hash bcrypt corresponde al raw recibido.
// Retorna nil si no se encuentra. Itera y usa bcrypt.CompareHashAndPassword.
// No filtra por revoked/expires_at — el Service decide el error específico.
func (r *RefreshTokenRepository) FindByRawToken(ctx context.Context, raw string) (*RefreshToken, error) {
	// Para evitar scan completo en cada refresh, podríamos filtrar revoked=false y expires_at>now(),
	// pero para distinguir "revocado" vs "expirado" vs "inexistente" necesitamos todos.
	rows, err := r.pool.Query(ctx, `SELECT id, user_id, token_hash, expires_at, revoked, created_at FROM identity.refresh_tokens`)
	if err != nil {
		return nil, fmt.Errorf("list for find: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rt RefreshToken
		if err := rows.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.Revoked, &rt.CreatedAt); err != nil {
			return nil, err
		}
		if err := bcrypt.CompareHashAndPassword([]byte(rt.TokenHash), []byte(raw)); err == nil {
			// match
			cp := rt
			return &cp, nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
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
func (r *RefreshTokenRepository) Rotate(ctx context.Context, oldID, userID, newHash string, newExpires time.Time) (string, error) {
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
	err = tx.QueryRow(ctx, `INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at, revoked) VALUES ($1,$2,$3,false) RETURNING id`, userID, newHash, newExpires).Scan(&newID)
	if err != nil {
		return "", fmt.Errorf("create new: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return newID, nil
}
