package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
)

// ServiceError representa error de dominio mapeable a HTTP (mismo patrón que notes).
type ServiceError struct {
	Code    string
	Message string
}

var (
	ErrNotFound                = errors.New("group_not_found")
	ErrUnauthorized            = errors.New("unauthorized")
	ErrForbidden               = errors.New("forbidden")
	ErrBanned                  = errors.New("user_banned")
	ErrInvalidInviteToken      = errors.New("invalid_invite_token")
	ErrCannotModifySelf        = errors.New("cannot_modify_self")
	ErrTargetIsAdminOrNotFound = errors.New("target_not_found_or_admin")
	ErrTargetNotFound          = errors.New("target_not_found") 
	ErrInvalidRole             = errors.New("invalid_role")    
	ErrCannotLeaveOnlyAdmin    = errors.New("cannot_leave_only_admin")
)

func (e *ServiceError) Error() string { return e.Code + ": " + e.Message }

func newServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}

type GroupStore interface {
	CreateWithOwner(ctx context.Context, name string, description *string, ownerUserID string) (*model.Group, error)
	GetByID(ctx context.Context, id string) (*model.Group, error)
	ListMyGroups(ctx context.Context, userID string) ([]*model.MyGroup, error)
	JoinWithInviteToken(ctx context.Context, groupID string, inviteToken string, userID string) error
	RegenerateInviteToken(ctx context.Context, groupID string, userID string) (string, error)
	IsMember(ctx context.Context, groupID string, userID string) (bool, error)        
	ListMembers(ctx context.Context, groupID string) ([]*model.GroupMembership, error) 
	KickMember(ctx context.Context, groupID, adminID, targetUserID string) error
	BanMember(ctx context.Context, groupID, adminID, targetUserID string) error 
	ChangeMemberRole(ctx context.Context, groupID, adminID, targetUserID, newRole string) error
	TransferAdmin(ctx context.Context, groupID, currentAdminID, newAdminID string) error
	GetMemberRole(ctx context.Context, groupID, userID string) (string, error) // <--- Añadido
	CountAdmins(ctx context.Context, groupID string) (int, error)              // <--- Añadido
	RemoveMember(ctx context.Context, groupID, userID string) error            // <--- Añadido
}

// GroupCreatedNotifier dispara integraciones best-effort al crear un grupo
type GroupCreatedNotifier interface {
	OnGroupCreated(ctx context.Context, groupID string)
}

type NoopGroupCreatedNotifier struct{}

func (NoopGroupCreatedNotifier) OnGroupCreated(ctx context.Context, groupID string) {}

type GroupService struct {
	groups   GroupStore
	notifier GroupCreatedNotifier
}

func NewGroupService(groups GroupStore, notifier GroupCreatedNotifier) *GroupService {
	if notifier == nil {
		notifier = NoopGroupCreatedNotifier{}
	}
	return &GroupService{groups: groups, notifier: notifier}
}

func validateName(name string) error {
	if !utils.ValidateName(name) {
		return newServiceError(utils.ErrInvalidName)
	}
	return nil
}

// Create valida el payload, crea el grupo con el creador como admin
func (s *GroupService) Create(ctx context.Context, userID string, name string, description *string) (*model.Group, error) {
	name = utils.NormalizeName(name)
	if err := validateName(name); err != nil {
		return nil, err
	}
	if description != nil {
		trimmed := strings.TrimSpace(*description)
		if trimmed == "" {
			description = nil
		} else {
			description = &trimmed
		}
	}

	group, err := s.groups.CreateWithOwner(ctx, name, description, userID)
	if err != nil {
		log.Printf("social: error creando grupo: %v", err)
		return nil, newServiceError(utils.ErrInternal)
	}

	go s.notifier.OnGroupCreated(context.Background(), group.ID)

	return group, nil
}

func (s *GroupService) GetGroup(ctx context.Context, groupID string) (*model.Group, error) {
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrNotFound
	}
	return group, nil
}

func (s *GroupService) ListMyGroups(ctx context.Context, userID string) ([]*model.MyGroup, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	
	groups, err := s.groups.ListMyGroups(ctx, userID)
	if err != nil {
		return nil, err
	}
	
	if groups == nil {
		return []*model.MyGroup{}, nil
	}
	return groups, nil
}

func (s *GroupService) RegenerateInvite(ctx context.Context, groupID string, userID string) (string, error) {
	if userID == "" {
		return "", ErrUnauthorized
	}
	token, err := s.groups.RegenerateInviteToken(ctx, groupID, userID)
	if err != nil {
		if err.Error() == "forbidden" {
			return "", ErrForbidden
		}
		if err.Error() == "group_not_found" {
			return "", ErrNotFound
		}
		return "", err
	}
	return token, nil
}

func (s *GroupService) JoinGroup(ctx context.Context, groupID string, inviteToken string, userID string) error {
	if userID == "" {
		return ErrUnauthorized
	}
	err := s.groups.JoinWithInviteToken(ctx, groupID, inviteToken, userID)
	if err != nil {
		if err.Error() == "user_banned" {
			return ErrBanned
		}
		if err.Error() == "invalid_invite_token" {
			return ErrInvalidInviteToken
		}
		return err
	}
	return nil
}

func (s *GroupService) ListMembers(ctx context.Context, groupID string, userID string) ([]*model.GroupMembership, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}

	isMember, err := s.groups.IsMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, ErrForbidden
	}

	members, err := s.groups.ListMembers(ctx, groupID)
	if err != nil {
		if err.Error() == "group_not_found" {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return members, nil
}

func (s *GroupService) KickMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	if adminID == "" {
		return ErrUnauthorized
	}
	err := s.groups.KickMember(ctx, groupID, adminID, targetUserID)
	if err != nil {
		switch err.Error() {
		case "forbidden": return ErrForbidden
		case "cannot_modify_self": return ErrCannotModifySelf
		case "target_not_found_or_is_admin": return ErrTargetIsAdminOrNotFound
		default: return err
		}
	}
	return nil
}

func (s *GroupService) BanMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	if adminID == "" {
		return ErrUnauthorized
	}
	err := s.groups.BanMember(ctx, groupID, adminID, targetUserID)
	if err != nil {
		switch err.Error() {
		case "forbidden": return ErrForbidden
		case "cannot_modify_self": return ErrCannotModifySelf
		default: return err
		}
	}
	return nil
}

func (s *GroupService) ChangeRole(ctx context.Context, groupID, adminID, targetUserID, newRole string) error {
	if adminID == "" {
		return ErrUnauthorized
	}
	err := s.groups.ChangeMemberRole(ctx, groupID, adminID, targetUserID, newRole)
	if err != nil {
		switch err.Error() {
		case "forbidden": return ErrForbidden
		case "cannot_modify_self": return ErrCannotModifySelf
		case "invalid_role": return ErrInvalidRole
		case "target_not_found": return ErrTargetNotFound
		default: return err
		}
	}
	return nil
}

func (s *GroupService) TransferAdmin(ctx context.Context, groupID, currentAdminID, newAdminID string) error {
	if currentAdminID == "" {
		return ErrUnauthorized
	}
	err := s.groups.TransferAdmin(ctx, groupID, currentAdminID, newAdminID)
	if err != nil {
		switch err.Error() {
		case "forbidden": return ErrForbidden
		case "cannot_modify_self": return ErrCannotModifySelf
		case "target_not_found": return ErrTargetNotFound
		default: return err
		}
	}
	return nil
}

// LeaveGroup permite abandonar el grupo, validando orfandad y limpiando apuntes compartidos
func (s *GroupService) LeaveGroup(ctx context.Context, groupID string, userID string, cleanupSharedNotes bool, token string) error {
	role, err := s.groups.GetMemberRole(ctx, groupID, userID)
	if err != nil {
		if err.Error() == "not_member" {
			return ErrForbidden
		}
		return err
	}

	// 1. Validar Orfandad (si es admin, debe haber más de uno)
	if role == "admin" {
		adminsCount, err := s.groups.CountAdmins(ctx, groupID)
		if err != nil {
			return err
		}
		if adminsCount <= 1 {
			return ErrCannotLeaveOnlyAdmin
		}
	}

	// 2. Limpieza de apuntes (Llamada asíncrona hacia el microservicio Notes)
	if cleanupSharedNotes {
		go func() {
			payload := fmt.Sprintf(`{"user_id":"%s", "group_id":"%s"}`, userID, groupID)
			req, err := http.NewRequest("POST", "http://localhost:8082/notes/unshare-all", strings.NewReader(payload))
			if err == nil {
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set("Authorization", "Bearer "+token) // Pasamos el JWT para pasar el middleware de Notes
				}
				client := &http.Client{Timeout: 5 * time.Second}
				resp, err := client.Do(req)
				if err != nil {
					log.Printf("social: error llamando a notes/unshare-all: %v", err)
				} else if resp != nil {
					resp.Body.Close()
				}
			}
		}()
	}

	// 3. Eliminar membresía
	err = s.groups.RemoveMember(ctx, groupID, userID)
	if err != nil {
		return err
	}

	return nil
}