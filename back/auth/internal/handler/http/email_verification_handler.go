package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

const (
	ErrInvalidVerificationToken = "invalid_verification_token"

	msgVerificationRequested = "Si el correo está registrado y aún no fue confirmado, recibirás un enlace de confirmación. Revisa también la carpeta de spam."
	msgVerificationInvalid   = "El enlace de confirmación es inválido, expiró o ya fue reemplazado por uno más reciente. Solicita uno nuevo."
)

type EmailVerificationHandler struct {
	svc *service.EmailVerificationService
}

func NewEmailVerificationHandler(svc *service.EmailVerificationService) *EmailVerificationHandler {
	return &EmailVerificationHandler{svc: svc}
}

// Send atiende POST /auth/send-verification-email: solicitud y reenvío del
// correo de confirmación. Respuesta genérica, exista o no la cuenta.
func (h *EmailVerificationHandler) Send(c *gin.Context) {
	const op = "SendVerificationEmail"
	started := time.Now()
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req struct {
		Email string `json:"email" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logAuthOp(c, op, http.StatusBadRequest, "", started, "step=bind code=bad_request")
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Ingresa un correo válido.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.svc.Request(ctx, req.Email); err != nil {
		var se *service.ServiceError
		if errors.As(err, &se) {
			status := http.StatusBadRequest
			if se.Code == "mail_unavailable" {
				status = http.StatusServiceUnavailable
			}
			logAuthOp(c, op, status, "", started, "step=service code="+se.Code)
			utils.RespondError(c, status, se.Code, se.Message)
			return
		}
		logAuthOp(c, op, http.StatusInternalServerError, "", started, "step=service code=internal_error")
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "No se pudo procesar la solicitud. Inténtalo más tarde.")
		return
	}
	// accepted=true no implica que exista la cuenta ni que se envíe un
	// correo: el detalle queda en la línea "email-verification" del servicio.
	logAuthOp(c, op, http.StatusOK, "", started, "accepted=true")
	c.JSON(http.StatusOK, gin.H{"message": msgVerificationRequested})
}

// Verify atiende POST /auth/verify-email: canjea el token del correo.
func (h *EmailVerificationHandler) Verify(c *gin.Context) {
	const op = "VerifyEmail"
	started := time.Now()
	c.Header("Cache-Control", "no-store")
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		logAuthOp(c, op, http.StatusBadRequest, "", started, "step=bind code=bad_request")
		utils.RespondError(c, http.StatusBadRequest, "bad_request", "Falta el código de confirmación.")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	outcome, err := h.svc.Confirm(ctx, req.Token)
	if err != nil {
		logAuthOp(c, op, http.StatusInternalServerError, "", started, "step=confirm code=internal_error")
		utils.RespondError(c, http.StatusInternalServerError, "internal_error", "No se pudo confirmar el correo. Inténtalo más tarde.")
		return
	}
	switch outcome {
	case repository.ConfirmVerified:
		logAuthOp(c, op, http.StatusOK, "", started, "outcome=verified")
		c.JSON(http.StatusOK, gin.H{"status": "verified", "message": "Tu correo quedó confirmado."})
	case repository.ConfirmAlreadyVerified:
		logAuthOp(c, op, http.StatusOK, "", started, "outcome=already_verified")
		c.JSON(http.StatusOK, gin.H{"status": "already_verified", "message": "Tu correo ya estaba confirmado."})
	default:
		logAuthOp(c, op, http.StatusBadRequest, "", started, "code="+ErrInvalidVerificationToken+" outcome="+string(outcome))
		utils.RespondError(c, http.StatusBadRequest, ErrInvalidVerificationToken, msgVerificationInvalid)
	}
}
