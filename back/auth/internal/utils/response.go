package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error APIError `json:"error"`
}

func RespondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorResponse{
		Error: APIError{Code: code, Message: message},
	})
}

func RespondSuccess(c *gin.Context, status int, payload interface{}) {
	c.JSON(status, payload)
}

// Códigos de error según agentApiContract.md (vigente sin rol global)
const (
	ErrInvalidEmail       = "invalid_email"
	ErrWeakPassword       = "weak_password"
	ErrEmailTaken         = "email_taken"
	ErrInvalidDisplayName = "invalid_display_name"
	ErrInvalidVisibility  = "invalid_visibility"
	ErrInvalidCredentials = "invalid_credentials"
	ErrUnauthorized       = "unauthorized"
	ErrForbidden          = "forbidden"
	ErrInvalidToken       = "invalid_token"
	ErrTokenExpired       = "token_expired"
	ErrEmailMismatch      = "email_mismatch"
	ErrInvalidRedirect    = "invalid_redirect_uri"
)

func MessageForCode(code string) string {
	switch code {
	case ErrInvalidEmail:
		return "Formato de email inválido"
	case ErrWeakPassword:
		return "La contraseña no cumple la política mínima (mín 8 caracteres, al menos una letra y un dígito)"
	case ErrEmailTaken:
		return "Email ya registrado"
	case ErrInvalidDisplayName:
		return "display_name requerido (1-100 caracteres, no vacío)"
	case ErrInvalidVisibility:
		return "visibility debe ser public o private"
	case ErrInvalidCredentials:
		return "Credenciales inválidas"
	case ErrUnauthorized:
		return "No autorizado: token ausente, inválido o expirado"
	case ErrForbidden:
		return "Acceso denegado: rol insuficiente"
	case ErrInvalidToken:
		return "Token inválido"
	case ErrTokenExpired:
		return "Token expirado"
	case ErrEmailMismatch:
		return "El correo declarado no coincide con la cuenta de Google conectada"
	case ErrInvalidRedirect:
		return "redirect_uri no permitido para este cliente"
	default:
		return "Error"
	}
}

// Helper para mapear errores de Service a HTTP status
func StatusForCode(code string) int {
	switch code {
	case ErrInvalidEmail, ErrWeakPassword, ErrInvalidDisplayName, ErrInvalidVisibility,
		ErrEmailMismatch, ErrInvalidRedirect:
		return http.StatusBadRequest
	case ErrEmailTaken:
		return http.StatusConflict
	case ErrInvalidCredentials, ErrUnauthorized, ErrInvalidToken, ErrTokenExpired:
		return http.StatusUnauthorized
	case ErrForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}
