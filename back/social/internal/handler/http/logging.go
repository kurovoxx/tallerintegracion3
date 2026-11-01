package http

import (
	"log"

	"github.com/gin-gonic/gin"
)

// logOp deja una línea greppable por operación:
// [social] POST /groups/<id>/todo op=CreateTodo 201 task_id=<uuid>
// [social] POST /groups/<id>/todo op=CreateTodo 400 code=invalid_body ...
// Gin ya loguea método/path/status/latencia; esto agrega operación,
// IDs de negocio y código de error para depurar sin reproducir con curl.
func logOp(c *gin.Context, op string, status int, details string) {
	if details != "" {
		details = " " + details
	}
	log.Printf("[social] %s %s op=%s %d%s",
		c.Request.Method, c.FullPath(), op, status, details)
}

// fail escribe la respuesta de error con el formato habitual del servicio
// {"error": msg, "code": code} y deja la línea de log correspondiente.
// El JSON que ve el cliente es byte-idéntico al de antes.
func fail(c *gin.Context, op string, status int, code, msg string) {
	logOp(c, op, status, "code="+code+" msg="+msg)
	c.JSON(status, gin.H{"error": msg, "code": code})
}
