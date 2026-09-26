package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// InternalOAuthHandler expone endpoints de servicio-a-servicio (Social → Auth).
// Protegidos con RequireInternalKey (X-Internal-Key), NO con JWT de usuario:
// el sync de Calendar corre en background sin token de usuario disponible.
type InternalOAuthHandler struct {
	svc *service.CalendarOAuthService
}

func NewInternalOAuthHandler(svc *service.CalendarOAuthService) *InternalOAuthHandler {
	return &InternalOAuthHandler{svc: svc}
}

type calendarTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// GetCalendarToken maneja GET /internal/oauth/calendar-token?user_id=
// 200 {access_token} — el service refresca si está vencido.
// 400 user_id inválido · 404 not_connected · 403 conexión inválida/revocada · 502 Google no disponible
func (h *InternalOAuthHandler) GetCalendarToken(c *gin.Context) {
	userID := strings.TrimSpace(c.Query("user_id"))
	if userID == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id requerido")
		return
	}
	token, err := h.svc.GetValidAccessToken(c.Request.Context(), userID)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			switch se.Code {
			case "bad_request":
				utils.RespondError(c, http.StatusBadRequest, se.Code, se.Message)
				return
			case "not_connected":
				utils.RespondError(c, http.StatusNotFound, se.Code, se.Message)
				return
			case "calendar_connection_invalid":
				utils.RespondError(c, http.StatusForbidden, se.Code, se.Message)
				return
			case "google_unavailable":
				utils.RespondError(c, http.StatusBadGateway, se.Code, se.Message)
				return
			default:
				utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
				return
			}
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	c.JSON(http.StatusOK, calendarTokenResponse{AccessToken: token})
}

type reportRevokedRequest struct {
	UserID string `json:"user_id"`
}

// ReportCalendarRevoked maneja POST /internal/oauth/calendar-revoked
// Body {user_id} → 204. Lo llama Social al recibir 401/403 de Google Calendar.
// Idempotente: si no hay fila, igual responde 204 para no filtrar existencia.
func (h *InternalOAuthHandler) ReportCalendarRevoked(c *gin.Context) {
	var req reportRevokedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id requerido")
		return
	}
	_ = h.svc.ReportCalendarPermissionDenied(c.Request.Context(), strings.TrimSpace(req.UserID))
	// AbortWithStatus (igual que logout): vuelca el 204 también sin engine en tests.
	c.AbortWithStatus(http.StatusNoContent)
}
