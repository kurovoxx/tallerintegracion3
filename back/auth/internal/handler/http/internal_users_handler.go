package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// InternalUsersHandler expone resolución servicio-a-servicio (Social → Auth).
// Protegido con RequireInternalKey (X-Internal-Key), NO con JWT de usuario:
// Social lo llama con el user_id del solicitante ya validado como miembro.
type InternalUsersHandler struct {
	svc *service.UsersService
}

func NewInternalUsersHandler(svc *service.UsersService) *InternalUsersHandler {
	return &InternalUsersHandler{svc: svc}
}

// Lookup maneja GET /internal/users/lookup?ids=a,b,c
// 200 [{user_id, email, display_name?}] — siempre array, nunca null.
// Sin ids → []. Inexistentes se omiten (no 404 parcial).
func (h *InternalUsersHandler) Lookup(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("ids"))
	var ids []string
	if raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if p := strings.TrimSpace(part); p != "" {
				ids = append(ids, p)
			}
		}
	}
	out, err := h.svc.Lookup(c.Request.Context(), ids)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	c.JSON(http.StatusOK, out)
}
