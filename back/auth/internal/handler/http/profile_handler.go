package http

import (
	"net/http"

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
	DisplayName string  `json:"display_name"`
	PhotoURL    *string `json:"photo_url"`
	Phone       *string `json:"phone"`
	Institution *string `json:"institution"`
	Description *string `json:"description"`
	Visibility  string  `json:"visibility"`
}

// patchProfileRequest acepta actualización parcial.
// Punteros nil = campo no enviado (no actualizar).
type patchProfileRequest struct {
	DisplayName *string `json:"display_name"`
	PhotoURL    *string `json:"photo_url"`
	Phone       *string `json:"phone"`
	Institution *string `json:"institution"`
	Description *string `json:"description"`
	Visibility  *string `json:"visibility"`
	// Campos prohibidos: si el cliente intenta enviar user_id, lo ignoramos explícitamente
	UserID *string `json:"user_id"`
}

// GetProfile maneja GET /profile/me
func (h *ProfileHandler) GetProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
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
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}
	c.JSON(http.StatusOK, profileResponse{
		DisplayName: profile.DisplayName,
		PhotoURL:    profile.PhotoURL,
		Phone:       profile.Phone,
		Institution: profile.Institution,
		Description: profile.Description,
		Visibility:  profile.Visibility,
	})
}

// PatchProfile maneja PATCH /profile/me
func (h *ProfileHandler) PatchProfile(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok || userID == "" {
		utils.RespondError(c, http.StatusUnauthorized, utils.ErrUnauthorized, utils.MessageForCode(utils.ErrUnauthorized))
		return
	}

	var req patchProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Request inválido")
		return
	}

	// Seguridad: nunca permitir modificar otro user_id, aunque venga en el body lo ignoramos
	// Si el cliente envía user_id, lo rechazamos explícitamente con 400 para evidenciar mal uso
	if req.UserID != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "user_id no puede ser modificado")
		return
	}

	// Detectar body vacío (todos nil) → 400
	if req.DisplayName == nil && req.PhotoURL == nil && req.Phone == nil && req.Institution == nil && req.Description == nil && req.Visibility == nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "No hay campos para actualizar")
		return
	}

	repoReq := repository.ProfileUpdateRequest{
		DisplayName: req.DisplayName,
		PhotoURL:    req.PhotoURL,
		Phone:       req.Phone,
		Institution: req.Institution,
		Description: req.Description,
		Visibility:  req.Visibility,
	}

	updated, err := h.svc.UpdateProfile(c.Request.Context(), userID, repoReq)
	if err != nil {
		if se, ok := err.(*service.ServiceError); ok {
			status := utils.StatusForCode(se.Code)
			// Mapear códigos específicos (404 vs 400)
			if se.Code == "profile_not_found" {
				status = http.StatusNotFound
			} else if se.Code == "bad_request" || se.Code == utils.ErrInvalidVisibility || se.Code == utils.ErrInvalidDisplayName || se.Code == "invalid_photo_url" || se.Code == "invalid_phone" || se.Code == "invalid_institution" || se.Code == "invalid_description" {
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
			return
		}
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "Error interno")
		return
	}

	c.JSON(http.StatusOK, profileResponse{
		DisplayName: updated.DisplayName,
		PhotoURL:    updated.PhotoURL,
		Phone:       updated.Phone,
		Institution: updated.Institution,
		Description: updated.Description,
		Visibility:  updated.Visibility,
	})
}
