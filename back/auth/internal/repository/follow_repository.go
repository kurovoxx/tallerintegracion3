package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

// FollowRepository gestiona relaciones de "seguir" en identity.user_follows.
type FollowRepository struct {
	pool *pgxpool.Pool
}

func NewFollowRepository(pool *pgxpool.Pool) *FollowRepository {
	return &FollowRepository{pool: pool}
}

// parseUserID convierte un string a pgtype.UUID para consultas.
func parseUserID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("user_id inválido: %w", err)
	}
	var pgID pgtype.UUID
	if err := pgID.Scan(id); err != nil {
		return pgtype.UUID{}, fmt.Errorf("user_id inválido: %w", err)
	}
	return pgID, nil
}

// Follow crea una relación follower_id sigue a following_id.
// Retorna false si ya existía (no duplica).
func (r *FollowRepository) Follow(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO identity.user_follows (follower_id, following_id)
		VALUES ($1, $2)
		ON CONFLICT (follower_id, following_id) DO NOTHING`,
		followerID, followingID)
	if err != nil {
		return false, fmt.Errorf("follow: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Unfollow elimina una relación follower_id sigue a following_id.
// Retorna false si no existía (no hay nada que eliminar).
func (r *FollowRepository) Unfollow(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM identity.user_follows
		WHERE follower_id = $1 AND following_id = $2`,
		followerID, followingID)
	if err != nil {
		return false, fmt.Errorf("unfollow: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// IsFollowing verifica si follower_id sigue a following_id.
func (r *FollowRepository) IsFollowing(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM identity.user_follows
			WHERE follower_id = $1 AND following_id = $2
		)`,
		followerID, followingID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("is_following: %w", err)
	}
	return exists, nil
}

// ListFollowers devuelve los usuarios que siguen a followingID.
// Incluye datos públicos mínimos para mostrar en la UI.
func (r *FollowRepository) ListFollowers(ctx context.Context, followingID pgtype.UUID) ([]model.FollowedUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uf.follower_id, u.public_id, u.email, p.display_name, p.photo_thumb_url, uf.created_at
		FROM identity.user_follows uf
		JOIN identity.users u ON u.id = uf.follower_id
		LEFT JOIN identity.profiles p ON p.user_id = u.id
		WHERE uf.following_id = $1
		ORDER BY uf.created_at DESC`,
		followingID)
	if err != nil {
		return nil, fmt.Errorf("list_followers: %w", err)
	}
	defer rows.Close()

	out := []model.FollowedUser{}
	for rows.Next() {
		var u model.FollowedUser
		var publicID pgtype.Text
		var email pgtype.Text
		var displayName sql.NullString
		var photoThumb pgtype.Text
		if err := rows.Scan(&u.UserID, &publicID, &email, &displayName, &photoThumb, &u.FollowedAt); err != nil {
			return nil, fmt.Errorf("list_followers scan: %w", err)
		}
		if publicID.Valid {
			u.PublicID = &publicID.String
		}
		if displayName.Valid {
			u.DisplayName = displayName.String
		}
		if photoThumb.Valid {
			u.PhotoThumbURL = &photoThumb.String
		}
		_ = email // email no se expone en la lista pública de seguidores
		out = append(out, u)
	}
	return out, rows.Err()
}

// ListFollowing devuelve los usuarios que followeeID sigue.
func (r *FollowRepository) ListFollowing(ctx context.Context, followeeID pgtype.UUID) ([]model.FollowedUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT uf.following_id, u.public_id, u.email, p.display_name, p.photo_thumb_url, uf.created_at
		FROM identity.user_follows uf
		JOIN identity.users u ON u.id = uf.following_id
		LEFT JOIN identity.profiles p ON p.user_id = u.id
		WHERE uf.follower_id = $1
		ORDER BY uf.created_at DESC`,
		followeeID)
	if err != nil {
		return nil, fmt.Errorf("list_following: %w", err)
	}
	defer rows.Close()

	out := []model.FollowedUser{}
	for rows.Next() {
		var u model.FollowedUser
		var publicID pgtype.Text
		var email pgtype.Text
		var displayName sql.NullString
		var photoThumb pgtype.Text
		if err := rows.Scan(&u.UserID, &publicID, &email, &displayName, &photoThumb, &u.FollowedAt); err != nil {
			return nil, fmt.Errorf("list_following scan: %w", err)
		}
		if publicID.Valid {
			u.PublicID = &publicID.String
		}
		if displayName.Valid {
			u.DisplayName = displayName.String
		}
		if photoThumb.Valid {
			u.PhotoThumbURL = &photoThumb.String
		}
		_ = email
		out = append(out, u)
	}
	return out, rows.Err()
}

// getUserIDByPublicID resuelve un public_id a su UUID interno (minúscula para uso interno del paquete).
func (r *FollowRepository) getUserIDByPublicID(ctx context.Context, publicID string) (pgtype.UUID, error) {
	publicID = strings.TrimSpace(publicID)
	var id pgtype.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM identity.users WHERE public_id = $1`, publicID).Scan(&id)
	if err != nil {
		if err == pgx.ErrNoRows {
			return pgtype.UUID{}, fmt.Errorf("usuario no encontrado")
		}
		return pgtype.UUID{}, fmt.Errorf("get user by public_id: %w", err)
	}
	return id, nil
}

// GetUserIDByPublicID resuelve un public_id a su UUID interno (exportado).
func (r *FollowRepository) GetUserIDByPublicID(ctx context.Context, publicID string) (pgtype.UUID, error) {
	return r.getUserIDByPublicID(ctx, publicID)
}
