package utils

import (
	"net/mail"
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail checks formato RFC básico + regex adicional.
// Retorna false si formato inválido.
func ValidateEmail(email string) bool {
	e := strings.TrimSpace(email)
	if e == "" || len(e) > 255 {
		return false
	}
	if _, err := mail.ParseAddress(e); err != nil {
		return false
	}
	return emailRegex.MatchString(e)
}

// ValidatePassword aplica política mínima definida para Sprint 1:
// - mínimo 8 caracteres
// - al menos una letra
// - al menos un dígito
// Esto cumple "weak_password" del contrato sin ser excesivamente estricto.
// Debe documentarse en PR como política elegida.
func ValidatePassword(pwd string) bool {
	if len(pwd) < 8 || len(pwd) > 72 { // bcrypt límite 72 bytes
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range pwd {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		if unicode.IsSpace(r) {
			return false // no espacios
		}
	}
	return hasLetter && hasDigit
}

// ValidateUUID verifica forma UUID (los ids de identity.* son uuid en PG:
// un valor malformado rompería el cast $1::uuid[] en vez de dar NotFound).
// Vacío o solo espacios también es inválido.
func ValidateUUID(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
