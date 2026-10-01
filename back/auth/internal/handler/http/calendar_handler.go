package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// CalendarHandler maneja POST /auth/google-calendar/connect
type CalendarHandler struct {
	svc *service.CalendarOAuthService
}

func NewCalendarHandler(svc *service.CalendarOAuthService) *CalendarHandler {
	return &CalendarHandler{svc: svc}
}

type calendarConnectRequest struct {
	OAuthCode string `json:"oauth_code"`
	// redirect_uri opcional: flujo desktop RFC 8252 con puerto efímero.
	// Se valida con allowlist; vacío = redirect configurado del servidor.
	RedirectURI *string `json:"redirect_uri"`
}

type calendarConnectResponse struct {
	Connected bool `json:"connected"`
}

// Connect maneja POST /auth/google-calendar/connect
// Requiere Authorization: Bearer <access_token> (via middleware)
// Body {oauth_code} → 200 {connected:true}
func (h *CalendarHandler) Connect(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	var req calendarConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		return
	}
	if req.OAuthCode == "" {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "oauth_code requerido")
		return
	}
	var redirectURI string
	if req.RedirectURI != nil {
		redirectURI = *req.RedirectURI
	}
	err := h.svc.ConnectWithRedirect(c.Request.Context(), userID, req.OAuthCode, redirectURI)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			switch se.Code {
			case "bad_request", "invalid_redirect_uri":
				utils.RespondError(c, http.StatusBadRequest, se.Code, se.Message)
				return
			case "invalid_oauth_code":
				utils.RespondError(c, http.StatusBadRequest, se.Code, se.Message)
				return
			case "google_unavailable":
				utils.RespondError(c, http.StatusBadGateway, se.Code, se.Message)
				return
			case utils.ErrUnauthorized:
				utils.RespondError(c, http.StatusUnauthorized, se.Code, se.Message)
				return
			default:
				utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
				return
			}
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	c.JSON(http.StatusOK, calendarConnectResponse{Connected: true})
}

// Status responde GET /auth/google-calendar/status con {connected,
// external_email?}. Nunca expone access/refresh tokens ni secretos.
func (h *CalendarHandler) Status(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	status, err := h.svc.GetCalendarConnectionStatus(c.Request.Context(), userID)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "No se pudo consultar Calendar")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	if status.ExternalEmail != nil && *status.ExternalEmail != "" {
		c.JSON(http.StatusOK, gin.H{"connected": status.Connected, "external_email": *status.ExternalEmail})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": status.Connected})
}
