package service

import (
	"context"
	"strings"
	"time"

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
	users         *repository.UserRepository
	refreshTokens *repository.RefreshTokenRepository
	jwt           *JWTService
	cfgAccessExp  int // segundos
	cfgRefreshExp int // segundos
}

func NewAuthService(users *repository.UserRepository, refreshTokens *repository.RefreshTokenRepository, jwtService *JWTService, accessExp, refreshExp int) *AuthService {
	return &AuthService{
		users:         users,
		refreshTokens: refreshTokens,
		jwt:           jwtService,
		cfgAccessExp:  accessExp,
		cfgRefreshExp: refreshExp,
	}
}

// Constructor legacy para compatibilidad con tests que solo usan register sin JWT
func NewAuthServiceLegacy(users *repository.UserRepository) *AuthService {
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

type LoginResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Login verifica credenciales con bcrypt, genera access JWT (vida corta) y refresh token hasheado.
// Respeta contrato: 401 invalid_credentials mismo mensaje para email inexistente o password incorrecta.
func (s *AuthService) Login(ctx context.Context, email, password string) (*LoginResult, error) {
	email = strings.TrimSpace(email)
	// Buscar usuario (normalizado)
	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, NewServiceError(utils.ErrInvalidCredentials)
	}
	// Comparar bcrypt (constant time)
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, NewServiceError(utils.ErrInvalidCredentials)
	}
	if s.jwt == nil {
		return nil, &ServiceError{Code: "internal_error", Message: "jwt service no configurado"}
	}
	// Generar access token (JWT) sin role global — solo user_id + iss/aud/iat/exp
	jwtRes, err := s.jwt.GenerarAccessToken(UsuarioAutenticado{ID: user.ID})
	if err != nil {
		return nil, err
	}
	// Generar refresh token aleatorio + hash bcrypt + persistir con expires_at
	raw, hash, err := repository.GenerateRawToken()
	if err != nil {
		return nil, err
	}
	expiresAt := jwtRes.ExpiraEn.Add(time.Duration(s.cfgRefreshExp - s.cfgAccessExp) * time.Second)
	// Si jwt duración es 900s y refresh 7d, refresh expira 7d desde ahora = now + refreshExp
	// jwtRes.ExpiraEn = now + 900, entonces refresh = now + 604800
	// Ajuste: si jwt no está ligado, usar now directamente
	if s.cfgRefreshExp > 0 {
		expiresAt = time.Now().Add(time.Duration(s.cfgRefreshExp) * time.Second)
	}
	if _, err := s.refreshTokens.Create(ctx, user.ID, hash, expiresAt); err != nil {
		return nil, err
	}
	return &LoginResult{
		AccessToken:  jwtRes.AccessToken,
		RefreshToken: raw,
		ExpiresIn:    s.cfgAccessExp,
	}, nil
}
