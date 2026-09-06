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
// Se exponen para que handlers y tests puedan leerlos.
const (
	ContextUserIDKey = "user_id"
	ContextRoleKey   = "role"
)

type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
	contextKeyRole   contextKey = "role"
)

// AuthMiddleware valida firma + expiración del access token en cada request protegida
// (masterprompt 3.1, agentApiContract convenciones).
// Inyecta user_id y role en el contexto. 401 token inválido/expirado, 403 rol insuficiente.
type AuthMiddleware struct {
	jwt *service.JWTService
}

func NewAuthMiddleware(jwtSvc *service.JWTService) *AuthMiddleware {
	return &AuthMiddleware{jwt: jwtSvc}
}

// RequireAuth valida Authorization: Bearer {access_token}.
// 401 si falta header, malformado, firma inválida o expirado.
// Inyecta user_id y role en gin.Context y en request.Context.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
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
			// Diferenciar expirado vs inválido para mensaje más claro, pero siempre 401
			if errors.Is(err, jwt.ErrTokenExpired) || containsIgnoreCase(err.Error(), "expired") {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrTokenExpired, utils.MessageForCode(utils.ErrTokenExpired))
			} else if errors.Is(err, service.ErrTokenVacio) || errors.Is(err, service.ErrAlgoritmoInvalido) || errors.Is(err, service.ErrTokenInvalido) {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrInvalidToken, utils.MessageForCode(utils.ErrInvalidToken))
			} else {
				// Fallback: cualquier error de validación es 401 (no 500) para no filtrar detalles
				// Si el error contiene "expired" lo tratamos como expirado
				if containsIgnoreCase(err.Error(), "expired") {
					utils.RespondError(c, http.StatusUnauthorized, utils.ErrTokenExpired, utils.MessageForCode(utils.ErrTokenExpired))
				} else {
					utils.RespondError(c, http.StatusUnauthorized, utils.ErrInvalidToken, utils.MessageForCode(utils.ErrInvalidToken))
				}
			}
			c.Abort()
			return
		}

		// Inyectar en gin.Context (handler) y en request.Context (service/gRPC)
		c.Set(ContextUserIDKey, user.ID)
		c.Set(ContextRoleKey, user.Role)
		ctx := context.WithValue(c.Request.Context(), contextKeyUserID, user.ID)
		ctx = context.WithValue(ctx, contextKeyRole, user.Role)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// RequireRole verifica que el rol inyectado por RequireAuth esté en la lista permitida.
// Debe usarse DESPUÉS de RequireAuth. 403 si rol insuficiente, 401 si no autenticado.
func (m *AuthMiddleware) RequireRole(allowedRoles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[strings.TrimSpace(r)] = struct{}{}
	}
	return func(c *gin.Context) {
		role, ok := GetRole(c)
		if !ok || strings.TrimSpace(role) == "" {
			// También intentar desde context.Context
			if v := c.Request.Context().Value(contextKeyRole); v != nil {
				if s, ok2 := v.(string); ok2 {
					role = s
					ok = true
				}
			}
		}
		if !ok {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
			c.Abort()
			return
		}
		if _, exists := allowed[role]; !exists {
			utils.RespondError(c, http.StatusForbidden, utils.ErrForbidden, utils.MessageForCode(utils.ErrForbidden))
			c.Abort()
			return
		}
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

func GetRole(c *gin.Context) (string, bool) {
	if v, exists := c.Get(ContextRoleKey); exists {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	if v := c.Request.Context().Value(contextKeyRole); v != nil {
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

func GetRoleFromContext(ctx context.Context) (string, bool) {
	if v := ctx.Value(contextKeyRole); v != nil {
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
