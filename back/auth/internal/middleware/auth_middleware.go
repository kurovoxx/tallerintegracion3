package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// Claves para Gin context (c.Set / c.Get) y para context.Context estándar.
const (
	ContextUserIDKey = "user_id"
)

type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
)

// AuthMiddleware valida firma + expiración del access token en cada request protegida.
// Inyecta solo user_id en el contexto. 401 token inválido/expirado.
type AuthMiddleware struct {
	jwt  *service.JWTService
	gate AccountAccessGate
}

// AccountAccessGate serializa el request completo con el inicio de borrado y
// rechaza el sujeto mientras exista un tombstone o la fila local haya sido borrada.
type AccountAccessGate interface {
	AcquireRequest(ctx context.Context, userID string) (release func(), active bool, err error)
}

func NewAuthMiddleware(jwtSvc *service.JWTService, gates ...AccountAccessGate) *AuthMiddleware {
	m := &AuthMiddleware{jwt: jwtSvc}
	if len(gates) > 0 {
		m.gate = gates[0]
	}
	return m
}

// RequireAuth valida Authorization: Bearer {access_token}.
// 401 si falta header, malformado, firma inválida o expirado.
// Inyecta user_id en gin.Context y en request.Context.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return m.require(true)
}

// RequireIdentity valida el JWT y extrae su identidad sin exigir que la cuenta
// siga activa. Se reserva para consultar/reanudar el borrado idempotente.
func (m *AuthMiddleware) RequireIdentity() gin.HandlerFunc {
	return m.require(false)
}

func (m *AuthMiddleware) require(checkAccountState bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if strings.TrimSpace(auth) == "" {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Authorization header requerido")
			c.Abort()
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Formato de Authorization debe ser Bearer {token}")
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Token vacío")
			c.Abort()
			return
		}

		user, err := m.jwt.ValidarAccessToken(token)
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) || containsIgnoreCase(err.Error(), "expired") {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrTokenExpired, utils.MessageForCode(utils.ErrTokenExpired))
			} else if errors.Is(err, service.ErrTokenVacio) || errors.Is(err, service.ErrAlgoritmoInvalido) || errors.Is(err, service.ErrTokenInvalido) {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrInvalidToken, utils.MessageForCode(utils.ErrInvalidToken))
			} else {
				if containsIgnoreCase(err.Error(), "expired") {
					utils.RespondError(c, http.StatusUnauthorized, utils.ErrTokenExpired, utils.MessageForCode(utils.ErrTokenExpired))
				} else {
					utils.RespondError(c, http.StatusUnauthorized, utils.ErrInvalidToken, utils.MessageForCode(utils.ErrInvalidToken))
				}
			}
			c.Abort()
			return
		}
		if checkAccountState && m.gate != nil {
			release, active, gateErr := m.gate.AcquireRequest(c.Request.Context(), user.ID)
			if gateErr != nil {
				utils.RespondError(c, http.StatusServiceUnavailable, "account_status_unavailable", "No se pudo comprobar el estado de la cuenta")
				c.Abort()
				return
			}
			if !active {
				utils.RespondError(c, http.StatusLocked, "account_deletion_in_progress", "La cuenta está en proceso de eliminación")
				c.Abort()
				return
			}
			defer release()
		}

		// Inyectar solo user_id
		c.Set(ContextUserIDKey, user.ID)
		ctx := context.WithValue(c.Request.Context(), contextKeyUserID, user.ID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// Helpers para handlers

func GetUserID(c *gin.Context) (string, bool) {
	if v, exists := c.Get(ContextUserIDKey); exists {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	if v := c.Request.Context().Value(contextKeyUserID); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

func GetUserIDFromContext(ctx context.Context) (string, bool) {
	if v := ctx.Value(contextKeyUserID); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
