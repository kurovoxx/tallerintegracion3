package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// RequireInternalKey protege endpoints de servicio-a-servicio (Social → Auth)
// con un secreto compartido vía header X-Internal-Key.
// Si el secreto no está configurado (vacío), deniega todo y loguea el problema:
// estos endpoints entregan access tokens de Google y nunca deben quedar abiertos.
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
