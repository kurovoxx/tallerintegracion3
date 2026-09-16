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

const (
	ErrBadRequest      = "bad_request"
	ErrUnauthorized    = "unauthorized"
	ErrForbidden       = "forbidden"
	ErrNotFound        = "not_found"
	ErrConflict        = "conflict"
	ErrInternal        = "internal_error"
	ErrInvalidToken    = "invalid_token"
	ErrTokenExpired    = "token_expired"
	ErrInvalidName     = "invalid_name"
	ErrInvalidRole     = "invalid_role"
	ErrAlreadyMember   = "already_member"
	ErrBanned          = "banned"
	ErrInvalidInvite   = "invalid_invite_token"
	ErrLastAdmin       = "last_admin"
	ErrCannotKickAdmin = "cannot_kick_admin"
)

func MessageForCode(code string) string {
	switch code {
	case ErrInvalidName:
		return "name requerido (1-200 caracteres)"
	case ErrInvalidRole:
		return "role debe ser admin o member"
	case ErrUnauthorized:
		return "No autorizado: token ausente, inválido o expirado"
	case ErrForbidden:
		return "Acceso denegado"
	case ErrNotFound:
		return "Recurso no encontrado"
	case ErrConflict:
		return "Conflicto"
	case ErrAlreadyMember:
		return "Ya eres miembro de este grupo"
	case ErrBanned:
		return "Has sido baneado de este grupo"
	case ErrInvalidInvite:
		return "Token de invitación inválido"
	case ErrLastAdmin:
		return "Eres el único admin, transfiere el rol antes de salir"
	case ErrCannotKickAdmin:
		return "No puedes expulsar a otro admin"
	default:
		return "Error"
	}
}

func StatusForCode(code string) int {
	switch code {
	case ErrInvalidName, ErrInvalidRole, ErrBadRequest, ErrCannotKickAdmin, ErrLastAdmin:
		return http.StatusBadRequest
	case ErrUnauthorized, ErrInvalidToken, ErrTokenExpired:
		return http.StatusUnauthorized
	case ErrForbidden, ErrBanned:
		return http.StatusForbidden
	case ErrNotFound, ErrInvalidInvite:
		return http.StatusNotFound
	case ErrConflict, ErrAlreadyMember:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
