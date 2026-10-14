package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
)

// RequireInternalKey protege endpoints de servicio-a-servicio (Notes → Social)
// con un secreto compartido vía header X-Internal-Key.
// Copia local a propósito: los microservicios no dependen entre sí
// (equivale a back/auth/internal/middleware/internal_middleware.go).
// Sin secreto configurado deniega todo (503): estos endpoints exponen
// membresía y emails, nunca deben quedar abiertos.
func RequireInternalKey(expectedKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(expectedKey) == "" {
			utils.RespondError(c, http.StatusServiceUnavailable, "internal_key_not_configured", "Internal key no configurada")
			c.Abort()
			return
		}
		got := strings.TrimSpace(c.GetHeader("X-Internal-Key"))
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expectedKey)) != 1 {
			utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, "Internal key inválida")
			c.Abort()
			return
		}
		c.Next()
	}
}
