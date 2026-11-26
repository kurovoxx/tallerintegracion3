package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/config"
)

const (
	ContextUserIDKey = "user_id"
)

type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
)

// ClaimsPersonalizadas son los claims que espera el servicio IA del JWT.
type ClaimsPersonalizadas struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// JWTValidator valida tokens HS256 y devuelve el user_id.
type JWTValidator interface {
	ValidarAccessToken(tokenString string) (string, error)
}

// SimpleHS256Validator valida HS256 con secreto local.
type SimpleHS256Validator struct {
	Secret   []byte
	Issuer   string
	Audience string
}

func (v *SimpleHS256Validator) ValidarAccessToken(tokenString string) (string, error) {
	if strings.TrimSpace(tokenString) == "" {
		return "", errors.New("token vacío")
	}
	claims := &ClaimsPersonalizadas{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, errors.New("algoritmo inválido")
			}
			return v.Secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(v.Issuer),
		jwt.WithAudience(v.Audience),
	)
	if err != nil {
		return "", err
	}
	if !token.Valid {
		return "", errors.New("token inválido")
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return "", errors.New("user_id vacío")
	}
	return claims.UserID, nil
}

// AuthMiddleware valida el JWT de usuario en requests a IA.
type AuthMiddleware struct {
	validator JWTValidator
}

func NewAuthMiddleware(validator JWTValidator) *AuthMiddleware {
	return &AuthMiddleware{validator: validator}
}

// RequireAuth valida Authorization: Bearer {access_token} e inyecta user_id.
// Devuelve 401 si falta header, es malformado, firma inválida o expirado.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if strings.TrimSpace(auth) == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "Authorization header requerido"}})
			c.Abort()
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "Formato debe ser Bearer {token}"}})
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "Token vacío"}})
			c.Abort()
			return
		}

		userID, err := m.validator.ValidarAccessToken(token)
		if err != nil {
			msg := err.Error()
			if strings.Contains(strings.ToLower(msg), "expired") {
				c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "token_expired", "message": "Token expirado"}})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "invalid_token", "message": "Token inválido"}})
			}
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, userID)
		ctx := context.WithValue(c.Request.Context(), contextKeyUserID, userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// GetUserID obtiene el user_id inyectado por el middleware.
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

// GetUserIDFromContext obtiene user_id de un contexto estándar.
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	if v := ctx.Value(contextKeyUserID); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// NewConfigValidator construye un validator desde *config.Config.
func NewConfigValidator(cfg *config.Config) *SimpleHS256Validator {
	return &SimpleHS256Validator{
		Secret:   []byte(cfg.JWTSecret),
		Issuer:   "apuntes-auth",
		Audience: "apuntes-client",
	}
}
