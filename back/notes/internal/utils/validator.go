package utils

import (
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// publicIDRegex es Crockford Base32 en minúsculas, 12 caracteres (60 bits).
// Copia deliberada de la validación de Auth (los servicios no se importan entre
// sí); excluye l, i, o y u para evitar confusiones visuales.
var publicIDRegex = regexp.MustCompile(`^[0-9a-hj-km-np-tv-z]{12}$`)

// NormalizePublicID recorta espacios y pasa a minúsculas. No corrige
// caracteres ambiguos (l/o no se convierten en 1/0).
func NormalizePublicID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidatePublicID valida estrictamente el formato ya normalizado.
func ValidatePublicID(s string) bool {
	return publicIDRegex.MatchString(strings.TrimSpace(s))
}

func ValidateTitle(title string) bool {
	t := strings.TrimSpace(title)
	return len(t) >= 1 && len(t) <= 255
}

func ValidateVisibility(v string) bool {
	return v == "public" || v == "private"
}

func ValidateAccessMode(m string) bool {
	return m == "link" || m == "restricted"
}

func NormalizeTitle(t string) string {
	return strings.TrimSpace(t)
}

func ValidateUUID(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
