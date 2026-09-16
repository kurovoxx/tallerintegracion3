package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
)

const (
	ContextUserIDKey = "user_id"
	ContextRoleKey   = "role"
)

type contextKey string

const (
	contextKeyUserID contextKey = "user_id"
	contextKeyRole   contextKey = "role"
)

type ClaimsPersonalizadas struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type JWTValidator interface {
	ValidarAccessToken(tokenString string) (string, string, error)
}

// SimpleHS256Validator valida HS256 con secreto local.
// Replica validación de auth service para no depender de gRPC en tests.
type SimpleHS256Validator struct {
	Secret   []byte
	Issuer   string
	Audience string
}

func (v *SimpleHS256Validator) ValidarAccessToken(tokenString string) (string, string, error) {
	if strings.TrimSpace(tokenString) == "" {
		return "", "", errors.New("token vacío")
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
		return "", "", err
	}
	if !token.Valid {
		return "", "", errors.New("token inválido")
	}
	if strings.TrimSpace(claims.UserID) == "" {
		return "", "", errors.New("user_id vacío")
	}
	return claims.UserID, claims.Role, nil
}

type AuthMiddleware struct {
	validator JWTValidator
}

func NewAuthMiddleware(v JWTValidator) *AuthMiddleware {
	return &AuthMiddleware{validator: v}
}

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
		userID, role, err := m.validator.ValidarAccessToken(token)
		if err != nil {
			msg := err.Error()
			if strings.Contains(strings.ToLower(msg), "expired") {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrTokenExpired, utils.MessageForCode(utils.ErrTokenExpired))
			} else {
				utils.RespondError(c, http.StatusUnauthorized, utils.ErrInvalidToken, utils.MessageForCode(utils.ErrInvalidToken))
			}
			c.Abort()
			return
		}
		c.Set(ContextUserIDKey, userID)
		c.Set(ContextRoleKey, role)
		ctx := context.WithValue(c.Request.Context(), contextKeyUserID, userID)
		ctx = context.WithValue(ctx, contextKeyRole, role)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

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
