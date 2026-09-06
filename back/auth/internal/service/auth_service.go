package service

import (
	"context"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// ServiceError representa error de dominio con código del contrato (agentApiContract.md).
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string { return e.Code + ": " + e.Message }

func NewServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}

func normalizeOptional(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

type AuthService struct {
	users *repository.UserRepository
}

func NewAuthService(users *repository.UserRepository) *AuthService {
	return &AuthService{users: users}
}

// Register implementa POST /auth/register según contrato + extensión display_name y campos opcionales nulables:
// - Valida email, password, role, display_name en Service para dar 400 legible
// - Hashea con bcrypt antes de persistir
// - Inserta en identity.users + identity.profiles (visibility private por defecto, opcionales null)
func (s *AuthService) Register(ctx context.Context, email, password, role, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	// Validar email
	if !utils.ValidateEmail(email) {
		return nil, NewServiceError(utils.ErrInvalidEmail)
	}
	// Validar role
	if !model.IsValidRole(role) {
		return nil, NewServiceError(utils.ErrInvalidRole)
	}
	// Validar password
	if !utils.ValidatePassword(password) {
		return nil, NewServiceError(utils.ErrWeakPassword)
	}
	// Validar display_name (requerido, no derivado del email)
	if displayName == "" || len(displayName) > 255 {
		return nil, NewServiceError(utils.ErrInvalidDisplayName)
	}
	// Validar visibility si viene (puede ser null/omitido → private)
	if visibility != nil {
		v := strings.TrimSpace(*visibility)
		if v != "public" && v != "private" {
			return nil, NewServiceError(utils.ErrInvalidVisibility)
		}
		*visibility = v
	}
	// Normalizar opcionales: trim, si vacío → nil (se persiste NULL)
	photoURL = normalizeOptional(photoURL)
	phone = normalizeOptional(phone)
	institution = normalizeOptional(institution)
	description = normalizeOptional(description)

	// Hash bcrypt (cost default 10)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, &ServiceError{Code: "internal_error", Message: "hash failed"}
	}

	// Persistir (Repository es única capa que toca SQL, tx atómica user+profile)
	user, err := s.users.CreateUser(ctx, email, string(hash), role, displayName, photoURL, phone, institution, description, visibility)
	if err != nil {
		if strings.Contains(err.Error(), "email_taken") {
			return nil, NewServiceError(utils.ErrEmailTaken)
		}
		return nil, err
	}
	return user, nil
}
