package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// ProfileRepository define el contrato que el Service espera del Repository.
// Permite mockear en tests sin tocar la BD.
type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID string) (*model.Profile, error)
	Update(ctx context.Context, userID string, req repository.ProfileUpdateRequest) (*model.Profile, error)
}

// ProfileService contiene reglas de negocio para GET/PATCH /profile/me.
type ProfileService struct {
	repo ProfileRepository
}

func NewProfileService(repo ProfileRepository) *ProfileService {
	return &ProfileService{repo: repo}
}

// GetProfile retorna el perfil del usuario autenticado.
// 404 si no existe (caso excepcional: Register debería haberlo creado).
func (s *ProfileService) GetProfile(ctx context.Context, userID string) (*model.Profile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, NewServiceError(utils.ErrUnauthorized)
	}
	profile, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, &ServiceError{Code: "profile_not_found", Message: "Perfil no encontrado"}
	}
	return profile, nil
}

// UpdateProfile aplica PATCH parcial solo al perfil del user_id autenticado.
// Valida visibility, display_name y normaliza opcionales.
// Comportamientos documentados:
// - Campos omitidos (nil) no se modifican.
// - Campos enviados como "" o "   " se persisten como NULL (borrar), salvo display_name que debe ser 1..100 y no puede ser vacío/NULL.
// - display_name si se envía debe ser 1..100 caracteres tras trim, sino 400 invalid_display_name.
// - photo_url si se envía debe ser 0..500 (vacío→NULL), sino 400 invalid_photo_url.
// - phone si se envía debe ser 0..30 (vacío→NULL), sino 400 invalid_phone.
// - institution si se envía debe ser 0..200 (vacío→NULL), sino 400 invalid_institution.
// - description es text sin límite (agentSql.md:46) → no se valida longitud, vacío→NULL.
// - visibility si se envía debe ser public/private, sino 400 invalid_visibility.
// - Body vacío (ningún campo) → 400 bad_request.
// - Si perfil no existe → 404 profile_not_found (no se crea automáticamente; Register es quien lo crea).
// - Un PATCH inválido no modifica ningún campo (validación antes de UPDATE).
// - updated_at solo se actualiza cuando el UPDATE es válido.
func (s *ProfileService) UpdateProfile(ctx context.Context, userID string, req repository.ProfileUpdateRequest) (*model.Profile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, NewServiceError(utils.ErrUnauthorized)
	}

	// Detectar body vacío
	if req.DisplayName == nil && req.PhotoURL == nil && req.Phone == nil && req.Institution == nil && req.Description == nil && req.Visibility == nil {
		return nil, &ServiceError{Code: "bad_request", Message: "No hay campos para actualizar"}
	}

	// Validar display_name si viene: 1..100, vacío → 400
	if req.DisplayName != nil {
		v := strings.TrimSpace(*req.DisplayName)
		if v == "" || len(v) > 100 {
			return nil, NewServiceError(utils.ErrInvalidDisplayName)
		}
		clean := v
		req.DisplayName = &clean
	}
	// Validar visibility si viene
	if req.Visibility != nil {
		v := strings.TrimSpace(*req.Visibility)
		if !model.IsValidVisibility(v) {
			return nil, NewServiceError(utils.ErrInvalidVisibility)
		}
		clean := v
		req.Visibility = &clean
	}
	// Validar longitudes máximas SIN truncar (400 si excede)
	if req.PhotoURL != nil {
		v := strings.TrimSpace(*req.PhotoURL)
		if len(v) > 500 {
			return nil, &ServiceError{Code: "invalid_photo_url", Message: "photo_url excede 500 caracteres"}
		}
		req.PhotoURL = &v
	}
	if req.Phone != nil {
		v := strings.TrimSpace(*req.Phone)
		if len(v) > 30 {
			return nil, &ServiceError{Code: "invalid_phone", Message: "phone excede 30 caracteres"}
		}
		req.Phone = &v
	}
	if req.Institution != nil {
		v := strings.TrimSpace(*req.Institution)
		if len(v) > 200 {
			return nil, &ServiceError{Code: "invalid_institution", Message: "institution excede 200 caracteres"}
		}
		req.Institution = &v
	}
	// description: text sin límite → no validar longitud, solo trim para coherencia
	if req.Description != nil {
		v := strings.TrimSpace(*req.Description)
		req.Description = &v
	}

	// Verificar existencia previa para dar 404 consistente
	// (Update ya retorna nil si no existe, pero validamos antes para mensaje claro)
	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, &ServiceError{Code: "profile_not_found", Message: "Perfil no encontrado"}
	}

	updated, err := s.repo.Update(ctx, userID, req)
	if err != nil {
		// Si repo retorna "no fields to update" (no debería llegar aquí por chequeo vacío)
		if strings.Contains(err.Error(), "no fields to update") {
			return nil, &ServiceError{Code: "bad_request", Message: "No hay campos para actualizar"}
		}
		return nil, err
	}
	if updated == nil {
		return nil, &ServiceError{Code: "profile_not_found", Message: "Perfil no encontrado"}
	}
	return updated, nil
}
