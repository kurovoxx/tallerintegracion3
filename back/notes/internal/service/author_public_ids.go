package service

import (
	"context"
	"log"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// AuthorPublicIDResolver resuelve en lote el código público (Identity
// identity.users.public_id) de los autores de una página de apuntes, para que
// la interfaz muestre ese código y nunca el UUID interno. Lo implementa el
// cliente gRPC de back/notes/internal/identity sobre IdentityService.LookupUsers
// (el mismo mecanismo que usa Social para enriquecer miembros).
type AuthorPublicIDResolver interface {
	// LookupPublicIDs devuelve user_id -> public_id. Los usuarios sin código o
	// inexistentes se omiten del mapa.
	LookupPublicIDs(ctx context.Context, userIDs []string) (map[string]string, error)
}

// SetAuthorPublicIDResolver inyecta el resolvedor de códigos públicos de
// autores. Nil desactiva el enriquecimiento (author_public_id se omite).
func (s *NoteService) SetAuthorPublicIDResolver(r AuthorPublicIDResolver) {
	s.authorPublicIDs = r
}

// resolveAuthorPublicIDs hace UNA llamada por página (sin N+1) y es
// best-effort: si Identity falla, el listado se entrega igual sin
// author_public_id (la interfaz muestra «Autor» sin código), nunca con el
// UUID como sustituto. No registra identificadores ni datos personales.
func (s *NoteService) resolveAuthorPublicIDs(ctx context.Context, userIDs []string) map[string]string {
	out := map[string]string{}
	if s.authorPublicIDs == nil || len(userIDs) == 0 {
		return out
	}
	seen := make(map[string]struct{}, len(userIDs))
	ids := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		id = strings.TrimSpace(id)
		if !utils.ValidateUUID(id) {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return out
	}
	got, err := s.authorPublicIDs.LookupPublicIDs(ctx, ids)
	if err != nil {
		log.Printf("notes: códigos públicos de autores no disponibles (autores=%d)", len(ids))
		return out
	}
	for id, code := range got {
		code = utils.NormalizePublicID(code)
		if _, asked := seen[id]; asked && utils.ValidatePublicID(code) {
			out[id] = code
		}
	}
	return out
}
