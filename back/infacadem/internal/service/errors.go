package service

import (
	"strings"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// ServiceError representa un error de dominio mapeable a HTTP.
// El handler lo clasifica por Code (vía errors.As), sin heurísticas.
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string { return e.Code + ": " + e.Message }

// NewServiceError construye un error de dominio con el mensaje canónico.
func NewServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}

// ValidateName normaliza y valida nombres (curricula/subjects: 1..200).
func ValidateName(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" || len(v) > 200 {
		return "", NewServiceError(utils.ErrInvalidName)
	}
	return v, nil
}

// ValidateUUID valida formato UUID.
// Convención del servicio (documentada): UUID malformado → 400 invalid_uuid;
// bien formado pero inexistente/ajeno → 404 not_found.
func ValidateUUID(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if _, err := uuid.Parse(v); err != nil {
		return "", NewServiceError(utils.ErrInvalidUUID)
	}
	return v, nil
}
