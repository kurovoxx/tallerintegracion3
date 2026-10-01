package http

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// DriveHandler maneja POST /auth/google-drive/connect y la config pública
// de Google OAuth (GET /auth/google-config).
type DriveHandler struct {
	svc               *service.DriveOAuthService
	googleClientID    string
	googleRedirectURI string
}

func NewDriveHandler(svc *service.DriveOAuthService) *DriveHandler {
	return &DriveHandler{svc: svc}
}

// NewDriveHandlerWithGoogleConfig inyecta los valores públicos de OAuth que
// expone GET /auth/google-config. El client_id es público por diseño; el
// secret jamás sale del backend.
func NewDriveHandlerWithGoogleConfig(svc *service.DriveOAuthService, clientID, redirectURI string) *DriveHandler {
	return &DriveHandler{svc: svc, googleClientID: clientID, googleRedirectURI: redirectURI}
}

type googleConfigResponse struct {
	ClientID    string `json:"client_id"`
	RedirectURI string `json:"redirect_uri"`
}

// GetGoogleConfig maneja GET /auth/google-config (público, sin middleware de auth).
// Responde 200 con client_id y redirect_uri para que el front no necesite
// --dart-define ni IDs quemados.
func (h *DriveHandler) GetGoogleConfig(c *gin.Context) {
	c.JSON(http.StatusOK, googleConfigResponse{
		ClientID:    h.googleClientID,
		RedirectURI: h.googleRedirectURI,
	})
}

type driveConnectRequest struct {
	OAuthCode string `json:"oauth_code"`
	// expected_email opcional: correo declarado en la app; si difiere del
	// email real de Google se rechaza con 400 email_mismatch.
	ExpectedEmail *string `json:"expected_email"`
	// redirect_uri opcional: flujo desktop RFC 8252 con puerto efímero.
	// Se valida con allowlist; vacío = redirect configurado del servidor.
	RedirectURI *string `json:"redirect_uri"`
}

type driveConnectResponse struct {
	Connected bool `json:"connected"`
}

// Disconnect maneja DELETE /auth/google-drive/connection
// Desvincula por completo: revoca en Google (best-effort) y borra la fila.
// Idempotente: sin conexión previa responde 204 igual.
func (h *DriveHandler) Disconnect(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	if err := h.svc.Disconnect(c.Request.Context(), userID); err != nil {
		if se, ok := err.(*service.ServiceError); ok && se.Code == utils.ErrUnauthorized {
			utils.RespondError(c, http.StatusUnauthorized, se.Code, se.Message)
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	// AbortWithStatus (igual que logout): vuelca el 204 también sin engine en tests.
	c.AbortWithStatus(http.StatusNoContent)
}

// Status consulta únicamente la conexión del usuario autenticado; nunca expone tokens.
func (h *DriveHandler) Status(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}
	status, err := h.svc.GetGoogleDriveConnectionStatus(c.Request.Context(), userID)
	if err != nil {
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "No se pudo consultar Drive")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"connected": status.Connected, "reconnect_required": status.ReconnectRequired})
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
	var redirectURI string
	if req.RedirectURI != nil {
		redirectURI = *req.RedirectURI
	}
	err := h.svc.Connect(c.Request.Context(), userID, req.OAuthCode, req.ExpectedEmail, redirectURI)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			switch se.Code {
			case "bad_request", "email_mismatch", "invalid_redirect_uri":
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
