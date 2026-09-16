package utils

import (
	"strings"

	"github.com/google/uuid"
)

func ValidateName(name string) bool {
	n := strings.TrimSpace(name)
	return n != "" && len(n) <= 200
}

func ValidateRole(r string) bool {
	return r == "admin" || r == "member"
}

func NormalizeName(n string) string {
	return strings.TrimSpace(n)
}

func ValidateUUID(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}
