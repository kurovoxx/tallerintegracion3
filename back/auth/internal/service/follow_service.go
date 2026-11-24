package service

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

// FollowRepo es la interfaz que el servicio necesita del repositorio.
// Permite usar mocks en tests sin tocar la BD.
type FollowRepo interface {
	Follow(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error)
	Unfollow(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error)
	IsFollowing(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error)
	ListFollowers(ctx context.Context, followingID pgtype.UUID) ([]model.FollowedUser, error)
	ListFollowing(ctx context.Context, followeeID pgtype.UUID) ([]model.FollowedUser, error)
	GetUserIDByPublicID(ctx context.Context, publicID string) (pgtype.UUID, error)
}

// FollowService gestiona relaciones de seguir/dejar de seguir usuarios.
type FollowService struct {
	repo FollowRepo
}

// ErrFollowSelf se devuelve cuando un usuario intenta seguirse a sí mismo.
var ErrFollowSelf = errors.New("no puedes seguirte a ti mismo")

// ErrUserNotFound se devuelve cuando el public_id no corresponde a un usuario.
var ErrUserNotFound = errors.New("usuario no encontrado")

func NewFollowService(repo FollowRepo) *FollowService {
	return &FollowService{repo: repo}
}

// resolvePublicID convierte un public_id en pgtype.UUID del usuario objetivo.
func (s *FollowService) resolvePublicID(ctx context.Context, publicID string) (pgtype.UUID, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return pgtype.UUID{}, ErrUserNotFound
	}
	id, err := s.repo.GetUserIDByPublicID(ctx, publicID)
	if err != nil {
		if strings.Contains(err.Error(), "usuario no encontrado") {
			return pgtype.UUID{}, ErrUserNotFound
		}
		return pgtype.UUID{}, err
	}
	return id, nil
}

// Follow hace que followerID siga a followingID (resuelto desde publicID).
// Retorna true si se creó la relación, false si ya existía.
func (s *FollowService) Follow(ctx context.Context, followerID pgtype.UUID, publicID string) (bool, error) {
	followingID, err := s.resolvePublicID(ctx, publicID)
	if err != nil {
		return false, err
	}
	if followerID == followingID {
		return false, ErrFollowSelf
	}
	return s.repo.Follow(ctx, followerID, followingID)
}

// Unfollow hace que followerID deje de seguir a followingID (resuelto desde publicID).
// Retorna true si se eliminó la relación, false si no existía.
func (s *FollowService) Unfollow(ctx context.Context, followerID pgtype.UUID, publicID string) (bool, error) {
	followingID, err := s.resolvePublicID(ctx, publicID)
	if err != nil {
		return false, err
	}
	if followerID == followingID {
		return false, ErrFollowSelf
	}
	return s.repo.Unfollow(ctx, followerID, followingID)
}

// IsFollowing verifica si followerID sigue al usuario con publicID.
func (s *FollowService) IsFollowing(ctx context.Context, followerID pgtype.UUID, publicID string) (bool, error) {
	followingID, err := s.resolvePublicID(ctx, publicID)
	if err != nil {
		return false, err
	}
	return s.repo.IsFollowing(ctx, followerID, followingID)
}

// ListFollowers devuelve los seguidores del usuario con publicID.
func (s *FollowService) ListFollowers(ctx context.Context, publicID string) ([]model.FollowedUser, error) {
	userID, err := s.resolvePublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListFollowers(ctx, userID)
}

// ListFollowing devuelve a quién sigue el usuario con publicID.
func (s *FollowService) ListFollowing(ctx context.Context, publicID string) ([]model.FollowedUser, error) {
	userID, err := s.resolvePublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListFollowing(ctx, userID)
}
