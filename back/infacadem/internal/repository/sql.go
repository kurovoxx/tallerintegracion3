package repository

import "strings"

// qualify prefija cada columna con el alias ("id, name" → "t.id, t.name").
func qualify(columns, alias string) string {
	parts := strings.Split(columns, ",")
	for i := range parts {
		parts[i] = alias + "." + strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ", ")
}

// joinSet une fragmentos SET con ", ".
func joinSet(set []string) string {
	return strings.Join(set, ", ")
}
