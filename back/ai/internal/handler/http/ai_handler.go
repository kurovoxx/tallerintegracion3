package httpHandler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/ai/internal/service"
)

// AIHandler expone los endpoints de generación de contenido vía IA.
type AIHandler struct {
	svc service.AIService
}

func NewAIHandler(svc service.AIService) *AIHandler {
	return &AIHandler{svc: svc}
}

// Generate recibe una petición de generación y devuelve el resultado.
// En modo mock (scaffold) devuelve una respuesta simulada.
// En la tarea 4_5_6 se conectará con Gemini para PDF→Markdown.
func (h *AIHandler) Generate(c *gin.Context) {
	var req model.GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"code":    "invalid_body",
				"message": err.Error(),
			},
		})
		return
	}

	resp, err := h.svc.Generate(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"code":    "generation_failed",
				"message": err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}
