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

// Códigos de error del servicio Academic (envoltorio {"error":{code,message}}).
const (
	ErrBadRequest          = "bad_request"
	ErrUnauthorized        = "unauthorized"
	ErrNotFound            = "not_found"
	ErrInternal            = "internal_error"
	ErrInvalidToken        = "invalid_token"
	ErrTokenExpired        = "token_expired"
	ErrInvalidName         = "invalid_name"
	ErrInvalidStatus       = "invalid_status"
	ErrInvalidDecimal      = "invalid_decimal_places"
	ErrInvalidDayOfWeek    = "invalid_day_of_week"
	ErrInvalidTimeRange    = "invalid_time_range"
	ErrInvalidTimeFormat   = "invalid_time_format"
	ErrInvalidUUID         = "invalid_uuid"
	ErrInvalidSemester     = "invalid_semester"
	ErrInvalidCredits      = "invalid_credits"
	ErrInvalidTotalClasses = "invalid_total_classes"
	ErrInvalidAttendance   = "invalid_attendance"
	ErrInvalidScore        = "invalid_score"
	ErrInvalidWeight       = "invalid_weight"
	ErrInvalidDate         = "invalid_date"
	ErrInvalidRequirement  = "invalid_requirement"
	ErrInvalidComparator   = "invalid_comparator"
	ErrInvalidAction       = "invalid_action"
	ErrConflict            = "conflict"
)

func MessageForCode(code string) string {
	switch code {
	case ErrInvalidName:
		return "Nombre requerido (1-200 caracteres)"
	case ErrInvalidStatus:
		return "status debe ser in_progress, approved, failed o pending"
	case ErrInvalidDecimal:
		return "average_decimal_places debe estar entre 0 y 2"
	case ErrInvalidDayOfWeek:
		return "day_of_week debe estar entre 0 y 6"
	case ErrInvalidTimeRange:
		return "end_time debe ser mayor que start_time"
	case ErrInvalidTimeFormat:
		return "Formato de hora inválido (usar HH:MM o HH:MM:SS)"
	case ErrInvalidUUID:
		return "Identificador inválido (UUID esperado)"
	case ErrInvalidScore:
		return "score debe estar entre 10 y 70 o ser null (programada)"
	case ErrInvalidWeight:
		return "weight debe ser una fracción entre 0 y 1"
	case ErrInvalidDate:
		return "Formato de fecha inválido (usar YYYY-MM-DD)"
	case ErrInvalidRequirement:
		return "Requisito inválido: exactamente una fuente (promedio final o nota)"
	case ErrInvalidComparator:
		return "comparator debe ser >, >=, <, <= o ="
	case ErrInvalidAction:
		return "action debe ser force_exam, force_fail o force_pass"
	case ErrConflict:
		return "Conflicto: el recurso ya existe"
	case ErrUnauthorized:
		return "No autorizado: token ausente, inválido o expirado"
	case ErrNotFound:
		return "Recurso no encontrado"
	default:
		return "Error"
	}
}

// StatusForCode mapea códigos de dominio a HTTP.
func StatusForCode(code string) int {
	switch code {
	case ErrInvalidName, ErrInvalidStatus, ErrInvalidDecimal, ErrInvalidDayOfWeek,
		ErrInvalidTimeRange, ErrInvalidTimeFormat, ErrInvalidUUID, ErrBadRequest,
		ErrInvalidSemester, ErrInvalidCredits, ErrInvalidTotalClasses, ErrInvalidAttendance,
		ErrInvalidScore, ErrInvalidWeight, ErrInvalidDate, ErrInvalidRequirement,
		ErrInvalidComparator, ErrInvalidAction:
		return http.StatusBadRequest
	case ErrUnauthorized, ErrInvalidToken, ErrTokenExpired:
		return http.StatusUnauthorized
	case ErrNotFound:
		return http.StatusNotFound
	case ErrConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
