package middleware

import "github.com/gin-gonic/gin"

// PrivateNoStore fija Cache-Control: private, no-store antes de ejecutar el
// resto de la cadena. Se aplica únicamente a rutas autenticadas que necesitan
// cubrir también los rechazos tempranos de RequireAuth (401/423/503), sin
// afectar al resto del router. Copia deliberada de Auth: los servicios no se
// importan entre sí.
func PrivateNoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}
}
