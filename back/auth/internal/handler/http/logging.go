package http

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// logAuthOp deja una línea greppable por operación sin secretos:
//
// [auth] GET /profile/me op=GetProfile status=200 user_id=<uuid> duration_ms=4
// [auth] PATCH /profile/me op=PatchProfile status=400 user_id=<uuid> code=invalid_visibility has_display_name=false ... duration_ms=2
//
// userID "-" cuando no hay sesión (401). details son pares k=v ya
// sanitizados por el llamador: jamás tokens, emails, bodies ni secretos.
// La ruta usa FullPath (patrón canónico) con fallback al path del request.
func logAuthOp(c *gin.Context, op string, status int, userID string, started time.Time, details string) {
	route := c.FullPath()
	if route == "" && c.Request != nil && c.Request.URL != nil {
		route = c.Request.URL.Path
	}
	if userID == "" {
		userID = "-"
	}
	details = strings.TrimSpace(details)
	if details != "" {
		details += " "
	}
	log.Printf("[auth] %s %s op=%s status=%d user_id=%s %sduration_ms=%d",
		c.Request.Method, route, op, status, userID, details, time.Since(started).Milliseconds())
}

// boolDetail formatea un flag seguro como k=v para logs.
func boolDetail(key string, value bool) string {
	if value {
		return key + "=true"
	}
	return key + "=false"
}
