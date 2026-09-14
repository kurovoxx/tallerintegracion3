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

type UserRepository interface {
	CreateUser(ctx context.Context, email, passwordHash, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
}

type RefreshTokenRepository interface {
	Create(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) (string, error)
	CountByUser(ctx context.Context, userID string) (int, error)
	FindByRawToken(ctx context.Context, raw string) (*repository.RefreshToken, error)
	Revoke(ctx context.Context, id string) error
	Rotate(ctx context.Context, oldID, userID, newHash string, newExpires time.Time) (string, error)
}

type AuthService struct {
	users         UserRepository
	refreshTokens RefreshTokenRepository
	jwt           *JWTService
	cfgAccessExp  int // segundos
	cfgRefreshExp int // segundos
}

func NewAuthService(users UserRepository, refreshTokens RefreshTokenRepository, jwtService *JWTService, accessExp, refreshExp int) *AuthService {
	return &AuthService{
		users:         users,
		refreshTokens: refreshTokens,
		jwt:           jwtService,
		cfgAccessExp:  accessExp,
		cfgRefreshExp: refreshExp,
	}
}

// Constructor legacy para compatibilidad con tests que solo usan register sin JWT
func NewAuthServiceLegacy(users UserRepository) *AuthService {
	return &AuthService{users: users}
}

// Register implementa POST /auth/register según contrato vigente {email,password} (agentApiContract.md:13).
// - Valida email y password.
// - display_name es NOT NULL en identity.profiles (agentSql.md:40) pero el contrato ya no lo recibe.
//   Para mantener la creación transaccional sin inventar un flujo nuevo, se deriva del email si no viene:
//   local-part del email, trim, fallback "Usuario", truncado a 100. Si el cliente lo envía (compatibilidad), se valida 1..100.
// - Hashea con bcrypt antes de persistir.
// - Inserta en identity.users + identity.profiles (visibility private por defecto, opcionales null).
func (s *AuthService) Register(ctx context.Context, email, password string, displayName *string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	email = strings.TrimSpace(email)
	// Validar email
	if !utils.ValidateEmail(email) {
		return nil, NewServiceError(utils.ErrInvalidEmail)
	}
	// Validar password
	if !utils.ValidatePassword(password) {
		return nil, NewServiceError(utils.ErrWeakPassword)
	}
	// Resolver display_name: si no viene o vacío, derivar del email (coherente con NOT NULL y sin agregar campo obligatorio)
	var displayNameVal string
	if displayName != nil {
		v := strings.TrimSpace(*displayName)
		if v != "" {
			if len(v) > 100 {
				return nil, NewServiceError(utils.ErrInvalidDisplayName)
			}
			displayNameVal = v
		}
	}
	if displayNameVal == "" {
		// Derivar del email: local-part antes de @
		local := email
		if idx := strings.Index(email, "@"); idx > 0 {
			local = email[:idx]
		}
		local = strings.TrimSpace(local)
		if local == "" {
			local = "Usuario"
		}
		if len(local) > 100 {
			local = local[:100]
		}
		displayNameVal = local
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
	user, err := s.users.CreateUser(ctx, email, string(hash), displayNameVal, photoURL, phone, institution, description, visibility)
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

type RefreshResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Refresh valida el refresh_token (hash bcrypt, no revocado, no expirado), lo rota y emite nuevo access_token.
// Contrato: POST /auth/refresh {refresh_token} -> 200 {access_token, expires_in} + nuevo refresh_token por rotación, 401 si revocado/expirado/inexistente.
func (s *AuthService) Refresh(ctx context.Context, rawToken string) (*RefreshResult, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, NewServiceError(utils.ErrInvalidToken)
	}
	if s.jwt == nil {
		return nil, &ServiceError{Code: "internal_error", Message: "jwt service no configurado"}
	}
	// Localizar fila por bcrypt (sin índice SHA256)
	rt, err := s.refreshTokens.FindByRawToken(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	if rt == nil {
		return nil, NewServiceError(utils.ErrInvalidToken)
	}
	if rt.Revoked {
		return nil, NewServiceError(utils.ErrInvalidToken)
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, NewServiceError(utils.ErrTokenExpired)
	}
	// Emitir nuevo access_token para el user_id del refresh_token
	jwtRes, err := s.jwt.GenerarAccessToken(UsuarioAutenticado{ID: rt.UserID})
	if err != nil {
		return nil, err
	}
	// Generar nuevo refresh_token y rotar (revocar viejo + crear nuevo en tx)
	newRaw, newHash, err := repository.GenerateRawToken()
	if err != nil {
		return nil, err
	}
	newExpires := time.Now().Add(time.Duration(s.cfgRefreshExp) * time.Second)
	if _, err := s.refreshTokens.Rotate(ctx, rt.ID, rt.UserID, newHash, newExpires); err != nil {
		// Mapear errores de Rotate a códigos de dominio
		msg := err.Error()
		if strings.Contains(msg, "revoked") {
			return nil, NewServiceError(utils.ErrInvalidToken)
		}
		if strings.Contains(msg, "expired") {
			return nil, NewServiceError(utils.ErrTokenExpired)
		}
		if strings.Contains(msg, "not_found") {
			return nil, NewServiceError(utils.ErrInvalidToken)
		}
		return nil, err
	}
	return &RefreshResult{
		AccessToken:  jwtRes.AccessToken,
		RefreshToken: newRaw,
		ExpiresIn:    s.cfgAccessExp,
	}, nil
}
