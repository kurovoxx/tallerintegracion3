package service

import (
	"context"
	"strings"
)

// GroupMemberDirectory abstrae la obtención de correos de miembros de un grupo.
//
// Fuente preferida: identity.oauth_connections.external_account_email con
// provider = google_drive; fallback documentado: identity.users.email. Los
// miembros sin correo válido se omiten.
//
// Notes nunca consulta esos schemas directamente: el adaptador real (gRPC a
// Social/Identity, futuro archivo grpc_member_directory.go) implementará esta
// interfaz. En producción se inyecta vía SetMemberDirectory; en tests se usa
// MemoryMemberDirectory.
type GroupMemberDirectory interface {
	// ListMemberEmails retorna los correos crudos de los miembros del grupo.
	// Puede contener duplicados, vacíos o mayúsculas: el servicio normaliza
	// con NormalizeEmails antes de crear permisos en Drive.
	ListMemberEmails(ctx context.Context, groupID string) ([]string, error)
}

// NormalizeEmails normaliza correos: trim, lowercase, omite vacíos y elimina
// duplicados preservando el orden de primera aparición.
func NormalizeEmails(emails []string) []string {
	seen := make(map[string]struct{}, len(emails))
	out := make([]string, 0, len(emails))
	for _, e := range emails {
		n := strings.ToLower(strings.TrimSpace(e))
		if n == "" {
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

// noopMemberDirectory retorna grupo sin miembros (adaptador aún no configurado).
type noopMemberDirectory struct{}

// NewNoopMemberDirectory crea el directorio nulo usado por defecto.
func NewNoopMemberDirectory() GroupMemberDirectory {
	return &noopMemberDirectory{}
}

func (noopMemberDirectory) ListMemberEmails(ctx context.Context, groupID string) ([]string, error) {
	return nil, nil
}
