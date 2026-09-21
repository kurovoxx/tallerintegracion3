package utils

import (
	"strings"

	"github.com/google/uuid"
)

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
