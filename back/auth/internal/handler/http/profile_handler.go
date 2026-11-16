package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// ProfileHandler maneja GET/PATCH /profile/me.
// Usa exclusivamente user_id del contexto inyectado por RequireAuth.
type ProfileHandler struct {
	svc *service.ProfileService
}

func NewProfileHandler(svc *service.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: svc}
}

type profileResponse struct {
	// PublicID vive en identity.users (no en identity.profiles) y es el único
	// identificador que se comparte fuera de la cuenta.
	PublicID      string  `json:"public_id"`
	DisplayName   string  `json:"display_name"`
	PhotoURL      *string `json:"photo_url"`
	PhotoThumbURL *string `json:"photo_thumb_url"`
	Phone         *string `json:"phone"`
	Institution   *string `json:"institution"`
	Description   *string `json:"description"`
	Visibility    string  `json:"visibility"`
	// Email (identity.users) agregado de forma backward-compatible:
	// clientes viejos lo ignoran; se omite si no se pudo resolver.
	Email *string `json:"email,omitempty"`
}

// publicProfileByPublicIDResponse es el DTO de GET /profile/public/:public_id.
// Expone nombre, institución, foto (solo imagen subida desde la app),
// public_id y visibility. No incluye user_id, email, phone ni la descripción,
// que es privada.
type publicProfileByPublicIDResponse struct {
	PublicID    string  `json:"public_id"`
	DisplayName string  `json:"display_name"`
	PhotoURL    *string `json:"photo_url"`
	Institution *string `json:"institution"`
	Visibility  string  `json:"visibility"`
}

// patchProfileRequest acepta actualización parcial.
// Punteros nil = campo no enviado (no actualizar).
type patchProfileRequest struct {
	DisplayName   *string `json:"display_name"`
	PhotoURL      *string `json:"photo_url"`
	PhotoThumbURL *string `json:"photo_thumb_url"`
	Phone         *string `json:"phone"`
	Institution   *string `json:"institution"`
	Description   *string `json:"description"`
	Visibility    *string `json:"visibility"`
	// Campos prohibidos: si el cliente intenta enviar user_id, lo ignoramos explícitamente
	UserID *string `json:"user_id"`
}

// profileFieldsDetail resume presencia de campos (has_*) sin valores: seguro
// para logs, nunca expone contenido del perfil.
func profileFieldsDetail(req patchProfileRequest) string {
	present := func(v *string) string {
		if v != nil {
			return "true"
		}
		return "false"
	}
	return "has_display_name=" + present(req.DisplayName) +
		" has_photo_url=" + present(req.PhotoURL) +
		" has_photo_thumb_url=" + present(req.PhotoThumbURL) +
		" has_phone=" + present(req.Phone) +
		" has_institution=" + present(req.Institution) +
		" has_description=" + present(req.Description) +
		" has_visibility=" + present(req.Visibility)
}

// GetProfile maneja GET /profile/me
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		logAuthOp(c, "GetProfile", http.StatusUnauthorized, "", started, "code="+utils.ErrUnauthorized)
		return
	}
	profile, err := h.svc.GetProfile(c.Request.Context(), userID)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			// profile_not_found → 404, unauthorized → 401
			status := http.StatusInternalServerError
			if se.Code == "profile_not_found" {
				status = http.StatusNotFound
			} else if se.Code == utils.ErrUnauthorized {
				status = http.StatusUnauthorized
			} else {
				status = utils.StatusForCode(se.Code)
			}
			utils.RespondError(c, status, se.Code, se.Message)
			logAuthOp(c, "GetProfile", status, userID, started, "code="+se.Code)
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		logAuthOp(c, "GetProfile", http.StatusInternalServerError, userID, started, "code=internal_error")
		return
	}
	logAuthOp(c, "GetProfile", http.StatusOK, userID, started, "")
	c.JSON(http.StatusOK, profileResponse{
		PublicID:      profile.PublicID,
		DisplayName:   profile.DisplayName,
		PhotoURL:      profile.PhotoURL,
		PhotoThumbURL: profile.PhotoThumbURL,
		Phone:         profile.Phone,
		Institution:   profile.Institution,
		Description:   profile.Description,
		Visibility:    profile.Visibility,
		Email:         profile.Email,
	})
}

// GetPublicProfileByPublicID maneja GET /profile/public/:public_id.
// El solicitante sale exclusivamente del JWT. La ruta aplica PrivateNoStore
// antes de RequireAuth y el handler reafirma Cache-Control: private, no-store
// para que ningún intermediario reutilice una respuesta autenticada.
func (h *ProfileHandler) GetPublicProfileByPublicID(c *gin.Context) {
	started := time.Now()
	c.Header("Cache-Control", "private, no-store")

	requesterID, ok := middleware.GetUserID(c)
	if !ok || requesterID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		logAuthOp(c, "GetPublicProfileByPublicID", http.StatusUnauthorized, "", started, "code="+utils.ErrUnauthorized)
		return
	}

	profile, err := h.svc.GetPublicProfileByPublicID(c.Request.Context(), requesterID, c.Param("public_id"))
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			if se.Code == "profile_not_found" {
				status = http.StatusNotFound
			}
			utils.RespondError(c, status, se.Code, se.Message)
			logAuthOp(c, "GetPublicProfileByPublicID", status, requesterID, started, "code="+se.Code)
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		logAuthOp(c, "GetPublicProfileByPublicID", http.StatusInternalServerError, requesterID, started, "code=internal_error")
		return
	}

	logAuthOp(c, "GetPublicProfileByPublicID", http.StatusOK, requesterID, started, "")
	c.JSON(http.StatusOK, publicProfileByPublicIDResponse{
		PublicID:    profile.PublicID,
		DisplayName: profile.DisplayName,
		PhotoURL:    profile.PhotoURL,
		Institution: profile.Institution,
		Visibility:  profile.Visibility,
	})
}

// PatchProfile maneja PATCH /profile/me
func (h *ProfileHandler) PatchProfile(c *gin.Context) {
	started := time.Now()
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		logAuthOp(c, "PatchProfile", http.StatusUnauthorized, "", started, "code="+utils.ErrUnauthorized)
		return
	}

	var req patchProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		logAuthOp(c, "PatchProfile", http.StatusBadRequest, userID, started, "code=bad_request")
		return
	}

	// Seguridad: nunca permitir modificar otro user_id, aunque venga en el body lo ignoramos
	// Si el cliente envía user_id, lo rechazamos explícitamente con 400 para evidenciar mal uso
	if req.UserID != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id no puede ser modificado")
		logAuthOp(c, "PatchProfile", http.StatusBadRequest, userID, started, "code=bad_request")
		return
	}

	// Detectar body vacío (todos nil) → 400
	if req.DisplayName == nil && req.PhotoURL == nil && req.PhotoThumbURL == nil && req.Phone == nil && req.Institution == nil && req.Description == nil && req.Visibility == nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "No hay campos para actualizar")
		logAuthOp(c, "PatchProfile", http.StatusBadRequest, userID, started, "code=bad_request")
		return
	}

	repoReq := repository.ProfileUpdateRequest{
		DisplayName:   req.DisplayName,
		PhotoURL:      req.PhotoURL,
		PhotoThumbURL: req.PhotoThumbURL,
		Phone:         req.Phone,
		Institution:   req.Institution,
		Description:   req.Description,
		Visibility:    req.Visibility,
	}

	updated, err := h.svc.UpdateProfile(c.Request.Context(), userID, repoReq)
	fields := profileFieldsDetail(req)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			// Mapear códigos específicos (404 vs 400)
			if se.Code == "profile_not_found" {
				status = http.StatusNotFound
			} else if se.Code == "bad_request" || se.Code == utils.ErrInvalidVisibility || se.Code == utils.ErrInvalidDisplayName || se.Code == "invalid_photo_url" || se.Code == "invalid_photo_thumb_url" || se.Code == "invalid_phone" || se.Code == "invalid_institution" || se.Code == "invalid_description" {
				status = http.StatusBadRequest
			} else if se.Code == utils.ErrUnauthorized {
				status = http.StatusUnauthorized
			} else if status == http.StatusInternalServerError {
				// Fallback: cualquier invalid_* debe ser 400
				if len(se.Code) >= 8 && se.Code[:8] == "invalid_" {
					status = http.StatusBadRequest
				}
			}
			utils.RespondError(c, status, se.Code, se.Message)
			logAuthOp(c, "PatchProfile", status, userID, started, "code="+se.Code+" "+fields)
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		logAuthOp(c, "PatchProfile", http.StatusInternalServerError, userID, started, "code=internal_error "+fields)
		return
	}
	logAuthOp(c, "PatchProfile", http.StatusOK, userID, started, fields)

	c.JSON(http.StatusOK, profileResponse{
		PublicID:      updated.PublicID,
		DisplayName:   updated.DisplayName,
		PhotoURL:      updated.PhotoURL,
		PhotoThumbURL: updated.PhotoThumbURL,
		Phone:         updated.Phone,
		Institution:   updated.Institution,
		Description:   updated.Description,
		Visibility:    updated.Visibility,
		Email:         updated.Email,
	})
}
