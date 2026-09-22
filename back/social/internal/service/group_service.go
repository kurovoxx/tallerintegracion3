package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
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
	ErrNotFound             = errors.New("group_not_found")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrForbidden            = errors.New("forbidden")
	ErrBanned               = errors.New("user_banned")
	ErrInvalidInviteToken   = errors.New("invalid_invite_token")
	ErrCannotModifySelf     = errors.New("cannot_modify_self")
	ErrTargetIsAdmin        = errors.New("target_is_admin")
	ErrTargetNotFound       = errors.New("target_not_found")
	ErrInvalidRole          = errors.New("invalid_role")
	ErrCannotLeaveOnlyAdmin = errors.New("cannot_leave_only_admin")
)

func (e *ServiceError) Error() string { return e.Code + ": " + e.Message }

func newServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}

// GroupStore es la persistencia de grupos. Las implementaciones (Postgres y
// memoria) aplican las reglas de autorización de cada operación y devuelven
// errores con los textos de storeErrors.
type GroupStore interface {
	CreateWithOwner(ctx context.Context, name string, description *string, ownerUserID string) (*model.Group, error)
	GetByID(ctx context.Context, id string) (*model.Group, error)
	ListMyGroups(ctx context.Context, userID string) ([]*model.MyGroup, error)
	ListMyGroupsDetailed(ctx context.Context, userID string) ([]*model.GroupCard, error)
	JoinWithInviteToken(ctx context.Context, groupID string, inviteToken string, userID string) error
	RegenerateInviteToken(ctx context.Context, groupID string, userID string) (string, error)
	IsMember(ctx context.Context, groupID string, userID string) (bool, error)
	ListMembers(ctx context.Context, groupID string) ([]*model.GroupMembership, error)
	KickMember(ctx context.Context, groupID, adminID, targetUserID string) error
	BanMember(ctx context.Context, groupID, adminID, targetUserID string) error
	ChangeMemberRole(ctx context.Context, groupID, adminID, targetUserID, newRole string) error
	TransferAdmin(ctx context.Context, groupID, currentAdminID, newAdminID string) error
	GetMemberRole(ctx context.Context, groupID, userID string) (string, error)
	CountAdmins(ctx context.Context, groupID string) (int, error)
	RemoveMember(ctx context.Context, groupID, userID string) error
	DeleteGroup(ctx context.Context, groupID string) error
	// HandleAccountDeletion saca al usuario de todos sus grupos aplicando la
	// sucesión automática de admin. Es idempotente.
	HandleAccountDeletion(ctx context.Context, userID string) ([]model.SuccessionResult, error)
}

// storeErrors traduce los errores de texto de la capa de persistencia a los
// errores de dominio del servicio.
var storeErrors = map[string]error{
	"forbidden":            ErrForbidden,
	"not_member":           ErrForbidden,
	"group_not_found":      ErrNotFound,
	"user_banned":          ErrBanned,
	"invalid_invite_token": ErrInvalidInviteToken,
	"cannot_modify_self":   ErrCannotModifySelf,
	"target_not_found":     ErrTargetNotFound,
	"target_is_admin":      ErrTargetIsAdmin,
	"invalid_role":         ErrInvalidRole,
}

func mapStoreErr(err error) error {
	if err == nil {
		return nil
	}
	if mapped, ok := storeErrors[err.Error()]; ok {
		return mapped
	}
	return err
}

// GroupCreatedNotifier dispara integraciones best-effort al crear un grupo
type GroupCreatedNotifier interface {
	OnGroupCreated(ctx context.Context, groupID string)
}

type NoopGroupCreatedNotifier struct{}

func (NoopGroupCreatedNotifier) OnGroupCreated(ctx context.Context, groupID string) {}

const defaultNotesBaseURL = "http://localhost:8082"

type GroupService struct {
	groups       GroupStore
	notifier     GroupCreatedNotifier
	notesBaseURL string
}

func NewGroupService(groups GroupStore, notifier GroupCreatedNotifier) *GroupService {
	if notifier == nil {
		notifier = NoopGroupCreatedNotifier{}
	}
	notesURL := strings.TrimRight(strings.TrimSpace(os.Getenv("NOTES_URL")), "/")
	if notesURL == "" {
		notesURL = defaultNotesBaseURL
	}
	return &GroupService{groups: groups, notifier: notifier, notesBaseURL: notesURL}
}

// SetNotesBaseURL cambia la URL base del servicio Notes (usado por tests).
func (s *GroupService) SetNotesBaseURL(url string) {
	s.notesBaseURL = strings.TrimRight(url, "/")
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

// GetGroup devuelve el grupo solo a sus miembros. El invite_token se incluye
// únicamente para admins (ver model.GroupView).
func (s *GroupService) GetGroup(ctx context.Context, groupID, userID string) (*model.GroupView, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return nil, ErrNotFound
	}
	group, err := s.groups.GetByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, ErrNotFound
	}
	role, err := s.groups.GetMemberRole(ctx, groupID, userID)
	if err != nil {
		return nil, mapStoreErr(err)
	}

	view := &model.GroupView{
		ID:                     group.ID,
		Name:                   group.Name,
		Description:            group.Description,
		OwnerUserID:            group.OwnerUserID,
		NotesRestrictedToStaff: group.NotesRestrictedToStaff,
		CreatedAt:              group.CreatedAt,
		Role:                   role,
	}
	if role == model.RoleAdmin {
		view.InviteToken = group.InviteToken
	}
	return view, nil
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

// MyOverview arma el payload de la vista principal (grupos + barra lateral).
// Los slices salen siempre vacíos y nunca nil para que el JSON sea [] y no null.
func (s *GroupService) MyOverview(ctx context.Context, userID string) (*model.MyOverview, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	cards, err := s.groups.ListMyGroupsDetailed(ctx, userID)
	if err != nil {
		return nil, err
	}
	ov := &model.MyOverview{
		UserID:  userID,
		Sidebar: model.OverviewSidebar{Groups: []model.SidebarGroup{}},
		Groups:  []model.GroupCard{},
	}
	for _, c := range cards {
		ov.Groups = append(ov.Groups, *c)
		ov.Sidebar.Groups = append(ov.Sidebar.Groups, model.SidebarGroup{GroupID: c.GroupID, Name: c.Name, Role: c.Role})
		if c.Role == model.RoleAdmin {
			ov.Stats.AdminGroupsCount++
		}
	}
	ov.Stats.GroupsCount = len(ov.Groups)
	return ov, nil
}

// MemberRole devuelve el rol del usuario en el grupo o ErrForbidden si no es
// miembro. Lo usan los payloads de vistas para exigir membresía.
func (s *GroupService) MemberRole(ctx context.Context, groupID, userID string) (string, error) {
	if userID == "" {
		return "", ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return "", ErrNotFound
	}
	role, err := s.groups.GetMemberRole(ctx, groupID, userID)
	if err != nil {
		return "", mapStoreErr(err)
	}
	return role, nil
}

func (s *GroupService) RegenerateInvite(ctx context.Context, groupID string, userID string) (string, error) {
	if userID == "" {
		return "", ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return "", ErrNotFound
	}
	token, err := s.groups.RegenerateInviteToken(ctx, groupID, userID)
	if err != nil {
		return "", mapStoreErr(err)
	}
	return token, nil
}

func (s *GroupService) JoinGroup(ctx context.Context, groupID string, inviteToken string, userID string) error {
	if userID == "" {
		return ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return ErrNotFound
	}
	// Un token mal formado nunca puede ser válido: evita que llegue a Postgres
	// como cast a uuid (500) y responde 404 como cualquier token inválido.
	if !utils.ValidateUUID(inviteToken) {
		return ErrInvalidInviteToken
	}
	return mapStoreErr(s.groups.JoinWithInviteToken(ctx, groupID, inviteToken, userID))
}

func (s *GroupService) ListMembers(ctx context.Context, groupID string, userID string) ([]*model.GroupMembership, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return nil, ErrNotFound
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
		return nil, mapStoreErr(err)
	}
	return members, nil
}

// checkAdminAction valida los ids comunes a las acciones administrativas sobre
// un miembro (expulsar, banear, cambiar rol, transferir).
func checkAdminAction(groupID, adminID, targetUserID string) error {
	if adminID == "" {
		return ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return ErrNotFound
	}
	if !utils.ValidateUUID(targetUserID) {
		return ErrTargetNotFound
	}
	return nil
}

// KickMember expulsa a un miembro. Solo admins; no se puede expulsar a otro admin.
func (s *GroupService) KickMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	if err := checkAdminAction(groupID, adminID, targetUserID); err != nil {
		return err
	}
	return mapStoreErr(s.groups.KickMember(ctx, groupID, adminID, targetUserID))
}

// BanMember expulsa y bloquea el reingreso de un miembro. Solo admins.
func (s *GroupService) BanMember(ctx context.Context, groupID, adminID, targetUserID string) error {
	if err := checkAdminAction(groupID, adminID, targetUserID); err != nil {
		return err
	}
	return mapStoreErr(s.groups.BanMember(ctx, groupID, adminID, targetUserID))
}

func (s *GroupService) ChangeRole(ctx context.Context, groupID, adminID, targetUserID, newRole string) error {
	if err := checkAdminAction(groupID, adminID, targetUserID); err != nil {
		return err
	}
	return mapStoreErr(s.groups.ChangeMemberRole(ctx, groupID, adminID, targetUserID, newRole))
}

func (s *GroupService) TransferAdmin(ctx context.Context, groupID, currentAdminID, newAdminID string) error {
	if err := checkAdminAction(groupID, currentAdminID, newAdminID); err != nil {
		return err
	}
	return mapStoreErr(s.groups.TransferAdmin(ctx, groupID, currentAdminID, newAdminID))
}

// LeaveGroup permite abandonar el grupo, validando orfandad y limpiando apuntes compartidos.
// Un admin único no puede irse si quedan otros miembros (debe transferir antes);
// si es el último miembro, el grupo se elimina.
func (s *GroupService) LeaveGroup(ctx context.Context, groupID string, userID string, cleanupSharedNotes bool, token string) error {
	if userID == "" {
		return ErrUnauthorized
	}
	if !utils.ValidateUUID(groupID) {
		return ErrNotFound
	}
	role, err := s.groups.GetMemberRole(ctx, groupID, userID)
	if err != nil {
		return mapStoreErr(err)
	}

	lastMember := false
	if role == model.RoleAdmin {
		// ponytail: conteo y borrado no son atómicos; dos admins saliendo a la vez
		// podrían dejar el grupo sin admin. Mover a una transacción con FOR UPDATE
		// (como HandleAccountDeletion) si se vuelve un caso real.
		admins, err := s.groups.CountAdmins(ctx, groupID)
		if err != nil {
			return err
		}
		if admins <= 1 {
			members, err := s.groups.ListMembers(ctx, groupID)
			if err != nil {
				return mapStoreErr(err)
			}
			if len(members) > 1 {
				return ErrCannotLeaveOnlyAdmin
			}
			lastMember = true
		}
	}

	if cleanupSharedNotes {
		go s.unshareNotes(userID, groupID, token)
	}

	if lastMember {
		return mapStoreErr(s.groups.DeleteGroup(ctx, groupID))
	}
	return mapStoreErr(s.groups.RemoveMember(ctx, groupID, userID))
}

// unshareNotes pide a Notes retirar los apuntes que el usuario compartió en el
// grupo. Es best-effort: un fallo solo se registra. Reenvía el JWT del usuario
// para pasar el middleware de Notes.
func (s *GroupService) unshareNotes(userID, groupID, token string) {
	payload, err := json.Marshal(map[string]string{"user_id": userID, "group_id": groupID})
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, s.notesBaseURL+"/notes/unshare-all", bytes.NewReader(payload))
	if err != nil {
		log.Printf("social: error armando notes/unshare-all: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		log.Printf("social: error llamando a notes/unshare-all: %v", err)
		return
	}
	resp.Body.Close()
}

// HandleAccountDeletion aplica la sucesión automática de admin cuando el
// usuario elimina su cuenta: en cada grupo donde es el único admin se promueve
// al miembro más antiguo; si era el último miembro, el grupo se elimina.
// Es idempotente: sin membresías restantes no hace nada.
func (s *GroupService) HandleAccountDeletion(ctx context.Context, userID string) ([]model.SuccessionResult, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	results, err := s.groups.HandleAccountDeletion(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		switch {
		case r.PromotedUserID != nil:
			log.Printf("social: grupo %s: admin %s eliminó su cuenta, se promovió a %s", r.GroupID, userID, *r.PromotedUserID)
		case r.GroupDeleted:
			log.Printf("social: grupo %s eliminado: %s era el último miembro", r.GroupID, userID)
		}
	}
	if results == nil {
		results = []model.SuccessionResult{}
	}
	return results, nil
}
