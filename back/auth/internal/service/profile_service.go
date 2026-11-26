package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

const (
	maxPhotoURLLength              = 500
	maxProfilePhotoDataURLLen      = 2_000_000
	maxProfilePhotoThumbDataURLLen = 120_000
	profilePhotoDataURLPrefix      = "data:image/"
	profilePhotoDataURLBase64      = ";base64,"
)

// validProfilePhotoURL acepta URLs http(s) cortas o data URLs de imagen
// recortadas por el cliente. El límite evita cuerpos PATCH desmedidos y se
// ajusta según sea la foto completa o su miniatura.
func validProfilePhotoURL(v string, maxDataURLLen int) bool {
	if strings.HasPrefix(v, profilePhotoDataURLPrefix) {
		return strings.Contains(v, profilePhotoDataURLBase64) &&
			len(v) <= maxDataURLLen
	}
	return len(v) <= maxPhotoURLLength
}

// ProfileRepository define el contrato que el Service espera del Repository.
// Permite mockear en tests sin tocar la BD.
type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID string) (*model.Profile, error)
	GetPublicByPublicID(ctx context.Context, publicID string) (*model.PublicProfile, error)
	Update(ctx context.Context, userID string, req repository.ProfileUpdateRequest) (*model.Profile, error)
}

// ProfileService contiene reglas de negocio para GET/PATCH /profile/me.
type ProfileService struct {
	repo ProfileRepository
	// users es opcional: fuente de identity.users.email para completar el
	// perfil propio. Nil = comportamiento anterior (sin email).
	users UserEmailLookup
}

// UserEmailLookup resuelve el email de un usuario por id.
type UserEmailLookup interface {
	GetEmailByID(ctx context.Context, userID string) (string, error)
}

func NewProfileService(repo ProfileRepository) *ProfileService {
	return &ProfileService{repo: repo}
}

// NewProfileServiceWithUsers igual que NewProfileService pero con email.
func NewProfileServiceWithUsers(repo ProfileRepository, users UserEmailLookup) *ProfileService {
	return &ProfileService{repo: repo, users: users}
}

// fillEmail completa Email de forma best-effort (nunca falla la operación).
func (s *ProfileService) fillEmail(ctx context.Context, p *model.Profile) {
	if p == nil || s.users == nil {
		return
	}
	email, err := s.users.GetEmailByID(ctx, p.UserID)
	if err != nil || strings.TrimSpace(email) == "" {
		return
	}
	p.Email = &email
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
	s.fillEmail(ctx, profile)
	return profile, nil
}

// GetPublicProfileByPublicID resuelve GET /profile/public/:public_id.
// El public_id se normaliza a minúsculas y se valida estrictamente; un UUID,
// un nombre o cualquier otro dato personal no son aceptados. Reglas:
//   - requesterID vacío → 401 (defensa en profundidad; lo exige el middleware).
//   - public_id inválido o inexistente → 404 profile_not_found.
//   - Perfil público → 200 con nombre, foto, descripción, correo, teléfono e
//     institución.
//   - Perfil privado consultado por un tercero → 200 SOLO con nombre y foto
//     (más public_id y visibility): el resto se descarta aquí.
//   - Perfil privado consultado por su dueño → 200 completo (son sus datos).
//   - Cualquier visibilidad distinta de "public" se trata como privada.
//   - La foto solo se entrega si es una imagen subida desde la app (data URL
//     de imagen en base64). Una URL remota se omite: mostrarla haría que la
//     app de quien consulta contactara a un servidor ajeno.
func (s *ProfileService) GetPublicProfileByPublicID(ctx context.Context, requesterID, rawPublicID string) (*model.PublicProfile, error) {
	requesterID = strings.TrimSpace(requesterID)
	if requesterID == "" {
		return nil, NewServiceError(utils.ErrUnauthorized)
	}
	publicID := utils.NormalizePublicID(rawPublicID)
	if !utils.ValidatePublicID(publicID) {
		return nil, &ServiceError{Code: "profile_not_found", Message: "Perfil no encontrado"}
	}
	profile, err := s.repo.GetPublicByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, &ServiceError{Code: "profile_not_found", Message: "Perfil no encontrado"}
	}
	profile.PhotoURL = publicPhotoURL(profile.PhotoURL)
	if profile.Visibility != model.VisibilityPublic {
		// Cualquier valor desconocido se presenta como "private".
		profile.Visibility = model.VisibilityPrivate
		if requesterID != profile.UserID {
			return privateProfileView(profile), nil
		}
	}
	profile.Institution = nonBlank(profile.Institution)
	profile.Description = nonBlank(profile.Description)
	profile.Email = nonBlank(profile.Email)
	profile.Phone = nonBlank(profile.Phone)
	return profile, nil
}

// privateProfileView construye una vista NUEVA con únicamente lo que un perfil
// privado muestra a terceros (nombre y foto). Se copia campo a campo en lugar
// de anular los demás para que un campo agregado al modelo en el futuro no se
// filtre por omisión.
func privateProfileView(p *model.PublicProfile) *model.PublicProfile {
	return &model.PublicProfile{
		UserID:      p.UserID,
		PublicID:    p.PublicID,
		DisplayName: p.DisplayName,
		PhotoURL:    p.PhotoURL,
		Visibility:  model.VisibilityPrivate,
	}
}

// nonBlank devuelve nil para valores ausentes o compuestos solo de espacios.
func nonBlank(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return &t
}

// publicPhotoURL devuelve la foto solo si es una data URL de imagen en base64
// (la que guarda el recorte de foto de la app); cualquier otro valor se omite.
func publicPhotoURL(photo *string) *string {
	if photo == nil {
		return nil
	}
	v := strings.TrimSpace(*photo)
	if !strings.HasPrefix(v, profilePhotoDataURLPrefix) || !strings.Contains(v, profilePhotoDataURLBase64) {
		return nil
	}
	return &v
}

// UpdateProfile aplica PATCH parcial solo al perfil del user_id autenticado.
// Valida visibility, display_name y normaliza opcionales.
// Comportamientos documentados:
//   - Campos omitidos (nil) no se modifican.
//   - Campos enviados como "" o "   " se persisten como NULL (borrar), salvo display_name que debe ser 1..100 y no puede ser vacío/NULL.
//   - display_name si se envía debe ser 1..100 caracteres tras trim, sino 400 invalid_display_name.
//   - photo_url si se envía debe ser 0..500 para URLs remotas, o una data URL
//     de imagen de hasta 2.000.000 caracteres (vacío→NULL), sino 400 invalid_photo_url.
//   - photo_thumb_url si se envía debe ser una URL remota corta o data URL de
//     hasta 120.000 caracteres (vacío→NULL), sino 400 invalid_photo_thumb_url.
//     Si se actualiza photo_url sin enviar la miniatura, la miniatura anterior
//     se borra para no mostrar un recorte viejo.
//   - phone si se envía debe ser 0..30 (vacío→NULL), sino 400 invalid_phone.
//   - institution si se envía debe ser 0..200 (vacío→NULL), sino 400 invalid_institution.
//   - description es text sin límite (agentSql.md:46) → no se valida longitud, vacío→NULL.
//   - visibility si se envía debe ser public/private, sino 400 invalid_visibility.
//   - Body vacío (ningún campo) → 400 bad_request.
//   - Si perfil no existe → 404 profile_not_found (no se crea automáticamente; Register es quien lo crea).
//   - Un PATCH inválido no modifica ningún campo (validación antes de UPDATE).
//   - updated_at solo se actualiza cuando el UPDATE es válido.
func (s *ProfileService) UpdateProfile(ctx context.Context, userID string, req repository.ProfileUpdateRequest) (*model.Profile, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, NewServiceError(utils.ErrUnauthorized)
	}

	// Detectar body vacío
	if req.DisplayName == nil && req.PhotoURL == nil && req.PhotoThumbURL == nil && req.Phone == nil && req.Institution == nil && req.Description == nil && req.Visibility == nil {
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
		if !validProfilePhotoURL(v, maxProfilePhotoDataURLLen) {
			return nil, &ServiceError{Code: "invalid_photo_url", Message: "photo_url inválida o excede el tamaño permitido"}
		}
		req.PhotoURL = &v
		// Una foto nueva invalida la miniatura anterior salvo que el cliente
		// mande explícitamente el mismo recorte actualizado.
		if req.PhotoThumbURL == nil {
			empty := ""
			req.PhotoThumbURL = &empty
		}
	}
	if req.PhotoThumbURL != nil {
		v := strings.TrimSpace(*req.PhotoThumbURL)
		if !validProfilePhotoURL(v, maxProfilePhotoThumbDataURLLen) {
			return nil, &ServiceError{Code: "invalid_photo_thumb_url", Message: "photo_thumb_url inválida o excede el tamaño permitido"}
		}
		req.PhotoThumbURL = &v
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
	s.fillEmail(ctx, updated)
	return updated, nil
}
