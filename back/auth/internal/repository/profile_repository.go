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
	query := `SELECT user_id, display_name, photo_url, phone, institution, description, visibility, updated_at FROM identity.profiles WHERE user_id = $1`
	err := r.pool.QueryRow(ctx, query, userID).Scan(&p.UserID, &p.DisplayName, &p.PhotoURL, &p.Phone, &p.Institution, &p.Description, &p.Visibility, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}

// UpdateRequest representa campos parciales para PATCH /profile/me.
// Nil = no actualizar. *string con valor "" (trim) = set NULL (borrar).
// Visibility nil = no actualizar.
type ProfileUpdateRequest struct {
	DisplayName *string
	PhotoURL    *string
	Phone       *string
	Institution *string
	Description *string
	Visibility  *string
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
		UPDATE identity.profiles
		SET %s
		WHERE user_id = $%d
		RETURNING user_id, display_name, photo_url, phone, institution, description, visibility, updated_at
	`, strings.Join(setClauses, ", "), argIdx)
	args = append(args, userID)

	var p model.Profile
	err := r.pool.QueryRow(ctx, query, args...).Scan(&p.UserID, &p.DisplayName, &p.PhotoURL, &p.Phone, &p.Institution, &p.Description, &p.Visibility, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("update profile: %w", err)
	}
	return &p, nil
}
