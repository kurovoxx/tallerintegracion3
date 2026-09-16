package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

// OAuthRepository maneja identity.oauth_connections.
type OAuthRepository struct {
	pool *pgxpool.Pool
}

func NewOAuthRepository(pool *pgxpool.Pool) *OAuthRepository {
	return &OAuthRepository{pool: pool}
}

// UpsertGoogleDriveConnection inserta o actualiza la conexión de Google Drive para un usuario.
// En reconexión actualiza access_token, expires_at, external_account_email, limpia revoked_at, conserva refresh_token si el nuevo es nil/vacío.
func (r *OAuthRepository) UpsertGoogleDriveConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	if accessToken == "" {
		return fmt.Errorf("access_token vacío")
	}
	query := `
		INSERT INTO identity.oauth_connections (user_id, provider, access_token, refresh_token, expires_at, external_account_email, revoked_at, updated_at)
		VALUES ($1, 'google_drive', $2, $3, $4, $5, NULL, now())
		ON CONFLICT (user_id, provider) DO UPDATE SET
			access_token = EXCLUDED.access_token,
			refresh_token = COALESCE(EXCLUDED.refresh_token, identity.oauth_connections.refresh_token),
			expires_at = EXCLUDED.expires_at,
			external_account_email = EXCLUDED.external_account_email,
			revoked_at = NULL,
			updated_at = now()
	`
	// Si refreshToken es nil o vacío, pasar nil para que COALESCE conserve el anterior
	var rtArg interface{}
	if refreshToken != nil && *refreshToken != "" {
		rtArg = *refreshToken
	} else {
		rtArg = nil
	}
	_, err := r.pool.Exec(ctx, query, userID, accessToken, rtArg, expiresAt, externalEmail)
	if err != nil {
		return fmt.Errorf("upsert oauth: %w", err)
	}
	return nil
}

// GetByUserIDAndProvider obtiene la conexión por user_id y provider.
func (r *OAuthRepository) GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error) {
	var oc model.OAuthConnection
	query := `SELECT id, user_id, provider, access_token, refresh_token, expires_at, external_account_email, revoked_at, created_at, updated_at FROM identity.oauth_connections WHERE user_id=$1 AND provider=$2`
	err := r.pool.QueryRow(ctx, query, userID, provider).Scan(&oc.ID, &oc.UserID, &oc.Provider, &oc.AccessToken, &oc.RefreshToken, &oc.ExpiresAt, &oc.ExternalAccountEmail, &oc.RevokedAt, &oc.CreatedAt, &oc.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get oauth: %w", err)
	}
	return &oc, nil
}

// UpdateGoogleDriveAccessToken actualiza solo access_token, refresh_token (con COALESCE), expires_at y updated_at.
// No modifica revoked_at, no inserta fila. Si no se actualiza ninguna fila, retorna error.
func (r *OAuthRepository) UpdateGoogleDriveAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error {
	if accessToken == "" {
		return fmt.Errorf("access_token vacío")
	}
	query := `
		UPDATE identity.oauth_connections
		SET
			access_token = $1,
			refresh_token = COALESCE($2, refresh_token),
			expires_at = $3,
			updated_at = now()
		WHERE
			user_id = $4
			AND provider = 'google_drive'
	`
	var rtArg interface{}
	if refreshToken != nil && *refreshToken != "" {
		rtArg = *refreshToken
	} else {
		rtArg = nil
	}
	tag, err := r.pool.Exec(ctx, query, accessToken, rtArg, expiresAt, userID)
	if err != nil {
		return fmt.Errorf("update oauth access_token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
	}
	return nil
}
