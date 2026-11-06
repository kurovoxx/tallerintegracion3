package httpHandler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/model"
)

// HealthHandler responde al health check del servicio.
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

func (h *HealthHandler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, model.HealthResponse{
		Status:  "UP",
		Service: "ai-service",
	})
}
