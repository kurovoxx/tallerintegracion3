package service

import (
	"context"
	"net/mail"
	"strings"
)

// GroupMemberDirectory abstrae la obtención de correos de miembros de un grupo.
//
// Fuente preferida: identity.oauth_connections.external_account_email con
// provider = google_drive; fallback documentado: identity.users.email. Los
// miembros sin correo válido se omiten.
//
// Notes nunca consulta esos schemas directamente: el adaptador de producción es
// el cliente HTTP a Social (social_adapter.go, /internal/groups/{id}/member-emails)
// inyectado en main.go vía SetMemberDirectory; en tests se usa
// MemoryMemberDirectory. NewNoopMemberDirectory queda solo como default seguro
// de construcción (grupo sin miembros), nunca como adaptador de producción.
type GroupMemberDirectory interface {
	// ListMemberEmails retorna los correos crudos de los miembros del grupo.
	// Puede contener duplicados, vacíos o mayúsculas: el servicio normaliza
	// con NormalizeEmails antes de crear permisos en Drive.
	ListMemberEmails(ctx context.Context, groupID string) ([]string, error)
}

// NormalizeEmails normaliza correos: trim, lowercase, descarta los malformados
// (sin cancelar la operación: si quedan correos válidos, se otorgan igual),
// omite vacíos y elimina duplicados preservando el orden de primera aparición.
// Un miembro con correo inválido no debe romper el share restricted completo.
func NormalizeEmails(emails []string) []string {
	seen := make(map[string]struct{}, len(emails))
	out := make([]string, 0, len(emails))
	for _, e := range emails {
		n := strings.ToLower(strings.TrimSpace(e))
		if n == "" || !isValidEmail(n) {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// isValidEmail valida un correo ya normalizado (lowercase, sin espacios).
// Usa net/mail y exige además un dominio con punto: descarta entradas como
// "no-es-un-email", "a@b" o direcciones con display name ("Foo <a@b.com>"),
// que Drive rechazaría al crear el permiso nominal.
func isValidEmail(normalized string) bool {
	addr, err := mail.ParseAddress(normalized)
	if err != nil {
		return false
	}
	// Sin parte display: el valor parseado debe ser exactamente el correo.
	if !strings.EqualFold(addr.Address, normalized) {
		return false
	}
	at := strings.LastIndex(addr.Address, "@")
	if at <= 0 || at == len(addr.Address)-1 {
		return false
	}
	domain := addr.Address[at+1:]
	return strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// noopMemberDirectory retorna grupo sin miembros: default seguro de
// construcción para tests; producción inyecta SocialHTTPAdapter.
type noopMemberDirectory struct{}

// NewNoopMemberDirectory crea el directorio nulo usado por defecto.
func NewNoopMemberDirectory() GroupMemberDirectory {
	return &noopMemberDirectory{}
}

func (noopMemberDirectory) ListMemberEmails(ctx context.Context, groupID string) ([]string, error) {
	return nil, nil
}
