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
	ErrBadRequest         = "bad_request"
	ErrUnauthorized       = "unauthorized"
	ErrForbidden          = "forbidden"
	ErrNotFound           = "not_found"
	ErrConflict           = "conflict"
	ErrPayloadTooLarge    = "payload_too_large"
	ErrInternal           = "internal_error"
	ErrInvalidTitle       = "invalid_title"
	ErrInvalidVisibility  = "invalid_visibility"
	ErrInvalidSubjectID   = "invalid_subject_id"
	ErrInvalidToken       = "invalid_token"
	ErrTokenExpired       = "token_expired"
	ErrAlreadySaved       = "already_saved"
	ErrAlreadyLiked       = "already_liked"
	ErrNotLiked           = "not_liked"
	ErrNoteUnavailable    = "note_unavailable"
	ErrFileTooLarge       = "file_too_large"
	ErrInvalidAccessMode  = "invalid_access_mode"
	ErrRateLimited        = "rate_limited"
)

func MessageForCode(code string) string {
	switch code {
	case ErrInvalidTitle:
		return "Título requerido (1-255 caracteres)"
	case ErrInvalidVisibility:
		return "Visibilidad debe ser public o private"
	case ErrInvalidSubjectID:
		return "subject_id debe ser un UUID válido"
	case ErrInvalidAccessMode:
		return "access_mode debe ser link o restricted"
	case ErrUnauthorized:
		return "No autorizado: token ausente, inválido o expirado"
	case ErrForbidden:
		return "Acceso denegado"
	case ErrNotFound:
		return "Recurso no encontrado"
	case ErrConflict:
		return "Conflicto"
	case ErrAlreadySaved:
		return "Nota ya guardada"
	case ErrAlreadyLiked:
		return "Nota ya likeada"
	case ErrNoteUnavailable:
		return "Nota no disponible en almacenamiento remoto"
	case ErrFileTooLarge:
		return "Archivo muy grande"
	case ErrRateLimited:
		return "Demasiadas solicitudes"
	default:
		return "Error"
	}
}

func StatusForCode(code string) int {
	switch code {
	case ErrInvalidTitle, ErrInvalidVisibility, ErrInvalidSubjectID, ErrInvalidAccessMode, ErrBadRequest:
		return http.StatusBadRequest
	case ErrUnauthorized, ErrInvalidToken, ErrTokenExpired:
		return http.StatusUnauthorized
	case ErrForbidden:
		return http.StatusForbidden
	case ErrNotFound, ErrNoteUnavailable:
		return http.StatusNotFound
	case ErrConflict, ErrAlreadySaved, ErrAlreadyLiked:
		return http.StatusConflict
	case ErrFileTooLarge, ErrPayloadTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}
