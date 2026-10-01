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
func (r *OAuthRepository) UpsertGoogleDriveConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	return r.upsertConnection(ctx, userID, model.ProviderGoogleDrive, accessToken, refreshToken, expiresAt, externalEmail)
}

// UpsertGoogleCalendarConnection inserta o actualiza la conexión de Google Calendar para un usuario.
func (r *OAuthRepository) UpsertGoogleCalendarConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	return r.upsertConnection(ctx, userID, model.ProviderGoogleCalendar, accessToken, refreshToken, expiresAt, externalEmail)
}

// UpdateGoogleCalendarAccessToken actualiza solo access_token/refresh_token/expires_at de Calendar.
func (r *OAuthRepository) UpdateGoogleCalendarAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error {
	return r.updateAccessToken(ctx, userID, model.ProviderGoogleCalendar, accessToken, refreshToken, expiresAt)
}

// MarkGoogleCalendarConnectionRevoked marca revoked_at de Calendar, idempotente.
func (r *OAuthRepository) MarkGoogleCalendarConnectionRevoked(ctx context.Context, userID string) error {
	return r.markConnectionRevoked(ctx, userID, model.ProviderGoogleCalendar)
}

// upsertConnection inserta o actualiza la conexión OAuth de un provider para un usuario.
// En reconexión actualiza access_token, expires_at, external_account_email, limpia revoked_at,
// conserva refresh_token si el nuevo es nil/vacío.
func (r *OAuthRepository) upsertConnection(ctx context.Context, userID, provider, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	if accessToken == "" {
		return fmt.Errorf("access_token vacío")
	}
	query := `
		INSERT INTO identity.oauth_connections (user_id, provider, access_token, refresh_token, expires_at, external_account_email, revoked_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NULL, now())
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
	_, err := r.pool.Exec(ctx, query, userID, provider, accessToken, rtArg, expiresAt, externalEmail)
	if err != nil {
		return fmt.Errorf("upsert oauth: %w", err)
	}
	return nil
}

// updateAccessToken actualiza solo access_token, refresh_token (con COALESCE), expires_at y updated_at.
// No modifica revoked_at, no inserta fila. Si no se actualiza ninguna fila, retorna error.
func (r *OAuthRepository) updateAccessToken(ctx context.Context, userID, provider, accessToken string, refreshToken *string, expiresAt time.Time) error {
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
			AND provider = $5
	`
	var rtArg interface{}
	if refreshToken != nil && *refreshToken != "" {
		rtArg = *refreshToken
	} else {
		rtArg = nil
	}
	tag, err := r.pool.Exec(ctx, query, accessToken, rtArg, expiresAt, userID, provider)
	if err != nil {
		return fmt.Errorf("update oauth access_token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
	}
	return nil
}

// markConnectionRevoked marca revoked_at si aún es NULL, de forma idempotente.
// No borra filas, no modifica tokens.
func (r *OAuthRepository) markConnectionRevoked(ctx context.Context, userID, provider string) error {
	query := `
		UPDATE identity.oauth_connections
		SET
			revoked_at = COALESCE(revoked_at, now()),
			updated_at = now()
		WHERE
			user_id = $1
			AND provider = $2
	`
	tag, err := r.pool.Exec(ctx, query, userID, provider)
	if err != nil {
		return fmt.Errorf("mark revoked: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
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

// MarkGoogleDriveConnectionRevoked marca revoked_at si aún es NULL, de forma idempotente.
// No borra filas, no modifica tokens.
func (r *OAuthRepository) MarkGoogleDriveConnectionRevoked(ctx context.Context, userID string) error {
	query := `
		UPDATE identity.oauth_connections
		SET
			revoked_at = COALESCE(revoked_at, now()),
			updated_at = now()
		WHERE
			user_id = $1
			AND provider = 'google_drive'
	`
	tag, err := r.pool.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("mark revoked: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("not_found")
	}
	return nil
}

// DeleteGoogleDriveConnection borra la fila de conexión (desconexión explícita
// desde la app). No falla si no existe (idempotente).
func (r *OAuthRepository) DeleteGoogleDriveConnection(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM identity.oauth_connections WHERE user_id = $1 AND provider = 'google_drive'`, userID)
	if err != nil {
		return fmt.Errorf("delete oauth: %w", err)
	}
	return nil
}
