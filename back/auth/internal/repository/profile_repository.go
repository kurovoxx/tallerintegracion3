package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

// ProfileRepository encapsula acceso a identity.profiles.
// Patrón: única capa que toca SQL (Handler → Service → Repository).
type ProfileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) *ProfileRepository {
	return &ProfileRepository{pool: pool}
}

// GetByUserID consulta identity.profiles por user_id.
// Retorna (nil,nil) si no existe — el Service decide 404.
func (r *ProfileRepository) GetByUserID(ctx context.Context, userID string) (*model.Profile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user_id vacío")
	}
	var p model.Profile
	query := `
		SELECT p.user_id, p.display_name, p.photo_url, p.photo_thumb_url, p.phone,
		       p.institution, p.description, p.visibility, p.updated_at, u.public_id
		FROM identity.profiles p
		JOIN identity.users u ON u.id = p.user_id
		WHERE p.user_id = $1`
	err := r.pool.QueryRow(ctx, query, userID).Scan(&p.UserID, &p.DisplayName, &p.PhotoURL, &p.PhotoThumbURL, &p.Phone, &p.Institution, &p.Description, &p.Visibility, &p.UpdatedAt, &p.PublicID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}

// GetPublicByPublicID resuelve un perfil público por identity.users.public_id.
// Si el usuario existe pero no tiene fila de perfil, retorna (nil,nil): la
// vista pública necesita los campos de profiles. No selecciona email, phone ni
// descripción.
func (r *ProfileRepository) GetPublicByPublicID(ctx context.Context, publicID string) (*model.PublicProfile, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return nil, fmt.Errorf("public_id vacío")
	}
	var p model.PublicProfile
	query := `
		SELECT p.user_id, p.display_name, p.photo_url, p.institution, p.visibility, u.public_id
		FROM identity.users u
		JOIN identity.profiles p ON p.user_id = u.id
		WHERE u.public_id = $1`
	err := r.pool.QueryRow(ctx, query, publicID).Scan(&p.UserID, &p.DisplayName, &p.PhotoURL, &p.Institution, &p.Visibility, &p.PublicID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get public profile by public_id: %w", err)
	}
	return &p, nil
}

// UpdateRequest representa campos parciales para PATCH /profile/me.
// Nil = no actualizar. *string con valor "" (trim) = set NULL (borrar).
// Visibility nil = no actualizar.
type ProfileUpdateRequest struct {
	DisplayName   *string
	PhotoURL      *string
	PhotoThumbURL *string
	Phone         *string
	Institution   *string
	Description   *string
	Visibility    *string
}

// Update aplica PATCH parcial. Solo actualiza campos no-nil.
// Retorna el perfil actualizado o (nil,nil) si no existe.
// Si req no tiene ningún campo, retorna error.
func (r *ProfileRepository) Update(ctx context.Context, userID string, req ProfileUpdateRequest) (*model.Profile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user_id vacío")
	}

	setClauses := []string{}
	args := []any{}
	argIdx := 1

	// Helper para añadir campo
	add := func(col string, val any) {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
		args = append(args, val)
		argIdx++
	}

	if req.DisplayName != nil {
		v := strings.TrimSpace(*req.DisplayName)
		add("display_name", v)
	}
	if req.PhotoURL != nil {
		v := strings.TrimSpace(*req.PhotoURL)
		if v == "" {
			add("photo_url", nil)
		} else {
			add("photo_url", v)
		}
	}
	if req.PhotoThumbURL != nil {
		v := strings.TrimSpace(*req.PhotoThumbURL)
		if v == "" {
			add("photo_thumb_url", nil)
		} else {
			add("photo_thumb_url", v)
		}
	}
	if req.Phone != nil {
		v := strings.TrimSpace(*req.Phone)
		if v == "" {
			add("phone", nil)
		} else {
			add("phone", v)
		}
	}
	if req.Institution != nil {
		v := strings.TrimSpace(*req.Institution)
		if v == "" {
			add("institution", nil)
		} else {
			add("institution", v)
		}
	}
	if req.Description != nil {
		v := strings.TrimSpace(*req.Description)
		if v == "" {
			add("description", nil)
		} else {
			add("description", v)
		}
	}
	if req.Visibility != nil {
		v := strings.TrimSpace(*req.Visibility)
		add("visibility", v)
	}

	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Siempre refrescar updated_at
	setClauses = append(setClauses, "updated_at = now()")

	query := fmt.Sprintf(`
		UPDATE identity.profiles p
		SET %s
		FROM identity.users u
		WHERE p.user_id = $%d AND u.id = p.user_id
		RETURNING p.user_id, p.display_name, p.photo_url, p.photo_thumb_url, p.phone,
		          p.institution, p.description, p.visibility, p.updated_at, u.public_id
	`, strings.Join(setClauses, ", "), argIdx)
	args = append(args, userID)

	var p model.Profile
	err := r.pool.QueryRow(ctx, query, args...).Scan(&p.UserID, &p.DisplayName, &p.PhotoURL, &p.PhotoThumbURL, &p.Phone, &p.Institution, &p.Description, &p.Visibility, &p.UpdatedAt, &p.PublicID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return &p, nil
}
