package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// DriveHandler maneja POST /auth/google-drive/connect
type DriveHandler struct {
	svc *service.DriveOAuthService
}

func NewDriveHandler(svc *service.DriveOAuthService) *DriveHandler {
	return &DriveHandler{svc: svc}
}

type driveConnectRequest struct {
	OAuthCode string `json:"oauth_code"`
	// expected_email opcional: correo declarado en la app; si difiere del
	// email real de Google se rechaza con 400 email_mismatch.
	ExpectedEmail *string `json:"expected_email"`
}

type driveConnectResponse struct {
	Connected bool `json:"connected"`
}

// Connect maneja POST /auth/google-drive/connect
// Requiere Authorization: Bearer <access_token> (via middleware)
// Body {oauth_code}
func (h *DriveHandler) Connect(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	var req driveConnectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		return
	}
	// Trim y validar oauth_code
	if req.OAuthCode == "" {
		// ShouldBindJSON con binding no tiene required, validamos manual
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "oauth_code requerido")
		return
	}
	// El Service hace TrimSpace y valida vacío
	err := h.svc.Connect(c.Request.Context(), userID, req.OAuthCode, req.ExpectedEmail)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			switch se.Code {
			case "bad_request", "email_mismatch":
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
	c.JSON(http.StatusOK, driveConnectResponse{Connected: true})
}
