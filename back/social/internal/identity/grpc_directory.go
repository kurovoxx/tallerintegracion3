package identity

import (
	"context"
	"errors"
	"log"
	"strings"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// GRPCUserDirectory resuelve datos públicos vía IdentityService.LookupUsers.
// Implementa service.UserDirectory con la misma semántica que
// HTTPUserDirectory: LookupUsers best-effort (ante cualquier fallo, mapa
// vacío sin error) y LookupEmailsStrict fail-closed (cualquier fallo es
// error, para no calcular permisos con listas incompletas).
type GRPCUserDirectory struct {
	client *GRPCClient
}

var _ service.UserDirectory = (*GRPCUserDirectory)(nil)

func NewGRPCUserDirectory(client *GRPCClient) *GRPCUserDirectory {
	return &GRPCUserDirectory{client: client}
}

func (d *GRPCUserDirectory) LookupUsers(ctx context.Context, userIDs []string) (map[string]service.UserPublic, error) {
	out, err := d.lookup(ctx, userIDs)
	if err != nil {
		log.Printf("social: user directory gRPC no disponible: %v", err)
		return map[string]service.UserPublic{}, nil
	}
	if out == nil {
		out = map[string]service.UserPublic{}
	}
	return out, nil
}

func (d *GRPCUserDirectory) LookupEmailsStrict(ctx context.Context, userIDs []string) ([]string, error) {
	// Sin cliente configurado es error de programación: fail-closed en vez
	// de devolver una lista vacía que validaría permisos contra nada.
	if d == nil || d.client == nil {
		return nil, errors.New("identity grpc: cliente no configurado")
	}
	got, err := d.lookup(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	emails := []string{}
	for _, id := range userIDs {
		u, ok := got[strings.TrimSpace(id)]
		if !ok {
			continue
		}
		email := strings.ToLower(strings.TrimSpace(u.Email))
		if email == "" {
			continue
		}
		if _, dup := seen[email]; dup {
			continue
		}
		seen[email] = struct{}{}
		emails = append(emails, email)
	}
	if emails == nil {
		emails = []string{}
	}
	return emails, nil
}

func (d *GRPCUserDirectory) lookup(ctx context.Context, userIDs []string) (map[string]service.UserPublic, error) {
	out := map[string]service.UserPublic{}
	if d == nil || d.client == nil || len(userIDs) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(userIDs))
	for _, id := range userIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	ctx, cancel := d.client.ctx(ctx)
	defer cancel()
	resp, err := d.client.client.LookupUsers(ctx, &identityv1.LookupUsersRequest{UserIds: ids})
	if err != nil {
		return nil, err
	}
	for _, u := range resp.GetUsers() {
		if strings.TrimSpace(u.GetUserId()) != "" {
			out[u.GetUserId()] = service.UserPublic{
				UserID:      u.GetUserId(),
				Email:       u.GetEmail(),
				DisplayName: u.DisplayName,
			}
		}
	}
	return out, nil
}
