package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

type PasswordResetHandler struct{ svc *service.PasswordResetService }

func NewPasswordResetHandler(svc *service.PasswordResetService) *PasswordResetHandler {
	return &PasswordResetHandler{svc: svc}
}

func (h *PasswordResetHandler) Forgot(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Ingresa un correo válido.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := h.svc.Forgot(ctx, req.Email); err != nil {
		respondResetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Si el correo está registrado, recibirás un código para recuperar tu contraseña. Revisa también la carpeta de spam."})
}

func (h *PasswordResetHandler) Reset(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req struct {
		Email    string `json:"email" binding:"required"`
		Code     string `json:"code" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Completa el correo, código y contraseña.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.svc.Reset(ctx, req.Email, req.Code, req.Password); err != nil {
		respondResetError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Contraseña actualizada. Ya puedes iniciar sesión."})
}

func respondResetError(c *gin.Context, err error) {
	var se *service.ServiceError
	if errors.As(err, &se) {
		status := utils.StatusForCode(se.Code)
		if se.Code == "invalid_reset_code" {
			status = http.StatusBadRequest
		}
		if se.Code == "mail_unavailable" {
			status = http.StatusServiceUnavailable
		}
		utils.RespondError(c, status, se.Code, se.Message)
		return
	}
	log.Print("password recovery: internal operation failed")
	utils.RespondError(c, http.StatusInternalServerError, "internal_error", "No se pudo completar la recuperación. Inténtalo más tarde.")
}
