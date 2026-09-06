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

// Códigos de error según agentApiContract.md + extensión para display_name
const (
	ErrInvalidEmail       = "invalid_email"
	ErrWeakPassword       = "weak_password"
	ErrInvalidRole        = "invalid_role"
	ErrEmailTaken         = "email_taken"
	ErrInvalidDisplayName = "invalid_display_name"
	ErrInvalidVisibility  = "invalid_visibility"
)

func MessageForCode(code string) string {
	switch code {
	case ErrInvalidEmail:
		return "Formato de email inválido"
	case ErrWeakPassword:
		return "La contraseña no cumple la política mínima (mín 8 caracteres, al menos una letra y un dígito)"
	case ErrInvalidRole:
		return "Rol debe ser student o teacher"
	case ErrEmailTaken:
		return "Email ya registrado"
	case ErrInvalidDisplayName:
		return "display_name requerido (1-255 caracteres, no vacío)"
	case ErrInvalidVisibility:
		return "visibility debe ser public o private"
	default:
		return "Error"
	}
}

// Helper para mapear errores de Service a HTTP status
func StatusForCode(code string) int {
	switch code {
	case ErrInvalidEmail, ErrWeakPassword, ErrInvalidRole, ErrInvalidDisplayName, ErrInvalidVisibility:
		return http.StatusBadRequest
	case ErrEmailTaken:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
