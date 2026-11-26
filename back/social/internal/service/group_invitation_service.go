package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
)

// GroupInvitationService gestiona invitaciones a grupos: invitar, aceptar,
// rechazar y listar pendientes.
type GroupInvitationService struct {
	repo *repository.GroupInvitationRepository
}

// ErrUserAlreadyMember se devuelve cuando el invitado ya es miembro del grupo.
var ErrUserAlreadyMember = errors.New("el usuario ya es miembro del grupo")

// ErrUserBanned se devuelve cuando el invitado está baneado del grupo.
var ErrUserBanned = errors.New("el usuario está baneado del grupo")

// ErrNotAdmin se devuelve cuando quien invita no es admin del grupo.
var ErrNotAdmin = errors.New("solo los administradores pueden invitar a miembros")

// ErrUserNotFollowed se devuelve si quien invita no sigue al destinatario.
var ErrUserNotFollowed = errors.New("solo puedes invitar a personas que sigues")

// ErrInvitationAlreadyPending se devuelve si ya existe una invitación vigente.
var ErrInvitationAlreadyPending = repository.ErrInvitationAlreadyPending

func NewGroupInvitationService(repo *repository.GroupInvitationRepository) *GroupInvitationService {
	return &GroupInvitationService{repo: repo}
}

// InviteToGroup invita a un usuario seguido a un grupo. Solo el admin del
// grupo puede invitar. El invitado no debe estar ya en el grupo ni baneado.
func (s *GroupInvitationService) InviteToGroup(ctx context.Context, inviterID, groupID, invitedUserID pgtype.UUID) (string, error) {
	// Validar que el inviter es admin del grupo.
	isAdmin, err := s.repo.IsAdmin(ctx, groupID, inviterID)
	if err != nil {
		return "", err
	}
	if !isAdmin {
		return "", ErrNotAdmin
	}

	isFollowing, err := s.repo.IsFollowing(ctx, inviterID, invitedUserID)
	if err != nil {
		return "", err
	}
	if !isFollowing {
		return "", ErrUserNotFollowed
	}

	// El invitado no puede estar ya en el grupo.
	isMember, err := s.repo.IsMember(ctx, groupID, invitedUserID)
	if err != nil {
		return "", err
	}
	if isMember {
		return "", ErrUserAlreadyMember
	}

	// El invitado no puede estar baneado.
	isBanned, err := s.repo.IsBanned(ctx, groupID, invitedUserID)
	if err != nil {
		return "", err
	}
	if isBanned {
		return "", ErrUserBanned
	}

	id, err := s.repo.CreateInvitation(ctx, groupID, invitedUserID, inviterID)
	if err != nil {
		if errors.Is(err, repository.ErrInvitationAlreadyPending) {
			return "", ErrInvitationAlreadyPending
		}
		return "", err
	}
	return id, nil
}

// AcceptInvitation acepta una invitación pendiente. El usuario se une al
// grupo como member regular. Devuelve false si la invitación no existe o no
// está pendiente.
func (s *GroupInvitationService) AcceptInvitation(ctx context.Context, id, userID pgtype.UUID) (bool, error) {
	return s.repo.AcceptInvitation(ctx, id, userID)
}

// RejectInvitation rechaza una invitación pendiente. Devuelve false si la
// invitación no existe o no está pendiente.
func (s *GroupInvitationService) RejectInvitation(ctx context.Context, id, userID pgtype.UUID) (bool, error) {
	return s.repo.RejectInvitation(ctx, id, userID)
}

// ListPendingInvitations devuelve las invitaciones pendientes para un usuario,
// con datos del grupo y del invitador.
func (s *GroupInvitationService) ListPendingInvitations(ctx context.Context, userID pgtype.UUID) ([]model.GroupInvitationView, error) {
	return s.repo.ListPendingInvitations(ctx, userID)
}
