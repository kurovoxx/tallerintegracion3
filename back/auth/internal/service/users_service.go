package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

// UserDirectory define el contrato que el servicio interno de usuarios
// espera del repositorio. Permite mockear en tests sin tocar la BD.
type UserDirectory interface {
	GetPublicByIDs(ctx context.Context, userIDs []string) ([]model.PublicUser, error)
}

// UsersService resuelve datos públicos mínimos para servicio-a-servicio
// (p. ej. nombres de miembros de un grupo en Social).
type UsersService struct {
	repo UserDirectory
}

func NewUsersService(repo UserDirectory) *UsersService {
	return &UsersService{repo: repo}
}

const maxLookupIDs = 200

// Lookup valida y resuelve. Sin ids → lista vacía (200, no 400).
// Nunca expone hashes ni tokens: solo user_id/email/display_name.
func (s *UsersService) Lookup(ctx context.Context, rawIDs []string) ([]model.PublicUser, error) {
	ids := make([]string, 0, len(rawIDs))
	for _, id := range rawIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return []model.PublicUser{}, nil
	}
	if len(ids) > maxLookupIDs {
		ids = ids[:maxLookupIDs]
	}
	out, err := s.repo.GetPublicByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []model.PublicUser{}
	}
	return out, nil
}
