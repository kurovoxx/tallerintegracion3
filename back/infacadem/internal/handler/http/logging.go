package http

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// logAcademicOp deja una línea greppable por operación sin secretos:
//
// [academic] POST /curricula op=CreateCurriculum status=201 user_id=<uuid> duration_ms=4
//
// userID "-" cuando no hay sesión (401). details son pares k=v ya
// sanitizados por el llamador: jamás tokens, bodies ni secretos.
// La ruta usa FullPath (patrón canónico) con fallback al path del request.
func logAcademicOp(c *gin.Context, op string, status int, userID string, started time.Time, details string) {
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
	log.Printf("[academic] %s %s op=%s status=%d user_id=%s %sduration_ms=%d",
		c.Request.Method, route, op, status, userID, details, time.Since(started).Milliseconds())
}
