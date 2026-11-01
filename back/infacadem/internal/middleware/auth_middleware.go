package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// Issuer y Audience deben coincidir con los del emisor (auth):
// back/auth emite iss "apuntes-auth", aud "apuntes-client".
const (
	TokenIssuer   = "apuntes-auth"
	TokenAudience = "apuntes-client"
)

const ContextUserIDKey = "user_id"

type contextKey string

const contextKeyUserID contextKey = "user_id"

// Claims del access token (solo user_id: Academic es 100% personal, sin roles).
type Claims struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

// SimpleHS256Validator valida HS256 con el secreto compartido.
// Espeja back/notes SimpleHS256Validator: sin gRPC, validación local.
type SimpleHS256Validator struct {
	Secret   []byte
	Issuer   string
	Audience string
}

func (v *SimpleHS256Validator) ValidarAccessToken(tokenString string) (string, error) {
	if strings.TrimSpace(tokenString) == "" {
		return "", errors.New("token vacío")
	}
	claims := &Claims{}
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

// AuthMiddleware valida el Bearer en rutas protegidas e inyecta user_id.
// 401 si falta header, malformado, firma inválida o expirado.
type AuthMiddleware struct {
	validator *SimpleHS256Validator
}

func NewAuthMiddleware(v *SimpleHS256Validator) *AuthMiddleware {
	return &AuthMiddleware{validator: v}
}

// RequireAuth valida Authorization: Bearer {access_token}.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if strings.TrimSpace(auth) == "" {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Authorization header requerido")
			c.Abort()
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Formato debe ser Bearer {token}")
			c.Abort()
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Token vacío")
			c.Abort()
			return
		}
		userID, err := m.validator.ValidarAccessToken(token)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "expired") {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrTokenExpired, utils.MessageForCode(utils.ErrTokenExpired))
			} else {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrInvalidToken, utils.MessageForCode(utils.ErrInvalidToken))
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

// GetUserID retorna el user_id inyectado por RequireAuth.
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

// GetUserIDFromContext retorna el user_id desde un context estándar.
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	if v := ctx.Value(contextKeyUserID); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}
