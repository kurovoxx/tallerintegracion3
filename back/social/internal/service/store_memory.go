package service

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

// MemoryGroupStore es una implementación en memoria de GroupStore para tests.
// Replica las reglas y los textos de error de repository.GroupRepository (las
// reglas de autorización viven en el SQL), de modo que los tests del servicio
// y de los handlers ejercitan la misma lógica sin base de datos. El test de
// integración del repositorio mantiene ambas implementaciones alineadas.
type MemoryGroupStore struct {
	mu     sync.Mutex
	clock  func() time.Time
	groups map[string]*model.Group
	order  []string                            // ids de grupos en orden de creación
	member map[string][]*model.GroupMembership // por grupo, en orden de ingreso
	bans   map[string]map[string]string        // grupo -> usuario baneado -> quién baneó
}

// NewMemoryGroupStore crea el store. El reloj interno avanza un segundo por
// llamada para que el orden por joined_at sea determinista.
func NewMemoryGroupStore() *MemoryGroupStore {
	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &MemoryGroupStore{
		clock: func() time.Time {
			tick = tick.Add(time.Second)
			return tick
		},
		groups: map[string]*model.Group{},
		member: map[string][]*model.GroupMembership{},
		bans:   map[string]map[string]string{},
	}
}

func (m *MemoryGroupStore) CreateWithOwner(_ context.Context, name string, description *string, ownerUserID string) (*model.Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := &model.Group{
		ID:          uuid.NewString(),
		Name:        name,
		Description: description,
		OwnerUserID: ownerUserID,
		InviteToken: uuid.NewString(),
		CreatedAt:   m.clock(),
	}
	m.groups[g.ID] = g
	m.order = append(m.order, g.ID)
	m.addMemberLocked(g.ID, ownerUserID, model.RoleAdmin)
	cp := *g
	return &cp, nil
}

func (m *MemoryGroupStore) GetByID(_ context.Context, id string) (*model.Group, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.groups[id]
	if !ok {
		return nil, nil
	}
	cp := *g
	return &cp, nil
}

func (m *MemoryGroupStore) ListMyGroups(_ context.Context, userID string) ([]*model.MyGroup, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*model.MyGroup
	for _, gid := range m.newestFirst() {
		if ms := m.find(gid, userID); ms != nil {
			out = append(out, &model.MyGroup{GroupID: gid, Name: m.groups[gid].Name, Role: ms.Role})
		}
	}
	return out, nil
}

func (m *MemoryGroupStore) ListMyGroupsDetailed(_ context.Context, userID string) ([]*model.GroupCard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []*model.GroupCard{}
	for _, gid := range m.newestFirst() {
		if ms := m.find(gid, userID); ms != nil {
			g := m.groups[gid]
			out = append(out, &model.GroupCard{
				GroupID:     gid,
				Name:        g.Name,
				Description: g.Description,
				Role:        ms.Role,
				MemberCount: len(m.member[gid]),
				JoinedAt:    ms.JoinedAt,
			})
		}
	}
	return out, nil
}

func (m *MemoryGroupStore) JoinWithInviteToken(_ context.Context, groupID, inviteToken, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, banned := m.bans[groupID][userID]; banned {
		return errors.New("user_banned")
	}
	g, ok := m.groups[groupID]
	if !ok || g.InviteToken != inviteToken {
		return errors.New("invalid_invite_token")
	}
	if m.find(groupID, userID) == nil {
		m.addMemberLocked(groupID, userID, model.RoleMember)
	}
	return nil
}

func (m *MemoryGroupStore) RegenerateInviteToken(_ context.Context, groupID, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireAdminLocked(groupID, userID); err != nil {
		return "", err
	}
	g, ok := m.groups[groupID]
	if !ok {
		return "", errors.New("group_not_found")
	}
	g.InviteToken = uuid.NewString()
	return g.InviteToken, nil
}

func (m *MemoryGroupStore) IsMember(_ context.Context, groupID, userID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.find(groupID, userID) != nil, nil
}

func (m *MemoryGroupStore) ListMembers(_ context.Context, groupID string) ([]*model.GroupMembership, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[groupID]; !ok {
		return nil, errors.New("group_not_found")
	}
	out := make([]*model.GroupMembership, 0, len(m.member[groupID]))
	for _, ms := range m.member[groupID] {
		cp := *ms
		out = append(out, &cp)
	}
	return out, nil
}

func (m *MemoryGroupStore) KickMember(_ context.Context, groupID, adminID, targetUserID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireAdminLocked(groupID, adminID); err != nil {
		return err
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}
	if err := m.requireRemovableLocked(groupID, targetUserID); err != nil {
		return err
	}
	m.removeLocked(groupID, targetUserID)
	return nil
}

func (m *MemoryGroupStore) BanMember(_ context.Context, groupID, adminID, targetUserID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireAdminLocked(groupID, adminID); err != nil {
		return err
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}
	if err := m.requireRemovableLocked(groupID, targetUserID); err != nil {
		return err
	}
	m.removeLocked(groupID, targetUserID)
	m.banLocked(groupID, targetUserID, adminID)
	return nil
}

func (m *MemoryGroupStore) ChangeMemberRole(_ context.Context, groupID, adminID, targetUserID, newRole string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireAdminLocked(groupID, adminID); err != nil {
		return err
	}
	if newRole != model.RoleAdmin && newRole != model.RoleMember {
		return errors.New("invalid_role")
	}
	if adminID == targetUserID {
		return errors.New("cannot_modify_self")
	}
	ms := m.find(groupID, targetUserID)
	if ms == nil {
		return errors.New("target_not_found")
	}
	ms.Role = newRole
	return nil
}

func (m *MemoryGroupStore) TransferAdmin(_ context.Context, groupID, currentAdminID, newAdminID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.requireAdminLocked(groupID, currentAdminID); err != nil {
		return err
	}
	if currentAdminID == newAdminID {
		return errors.New("cannot_modify_self")
	}
	target := m.find(groupID, newAdminID)
	if target == nil {
		return errors.New("target_not_found")
	}
	target.Role = model.RoleAdmin
	m.find(groupID, currentAdminID).Role = model.RoleMember
	return nil
}

func (m *MemoryGroupStore) GetMemberRole(_ context.Context, groupID, userID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ms := m.find(groupID, userID)
	if ms == nil {
		return "", errors.New("not_member")
	}
	return ms.Role, nil
}

func (m *MemoryGroupStore) CountAdmins(_ context.Context, groupID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.countAdminsLocked(groupID), nil
}

func (m *MemoryGroupStore) RemoveMember(_ context.Context, groupID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.find(groupID, userID) == nil {
		return errors.New("target_not_found")
	}
	m.removeLocked(groupID, userID)
	return nil
}

func (m *MemoryGroupStore) DeleteGroup(_ context.Context, groupID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.groups[groupID]; !ok {
		return errors.New("group_not_found")
	}
	m.deleteGroupLocked(groupID)
	return nil
}

// HandleAccountDeletion replica la sucesión de repository.removeUserFromGroup.
func (m *MemoryGroupStore) HandleAccountDeletion(_ context.Context, userID string) ([]model.SuccessionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	results := []model.SuccessionResult{}
	for _, gid := range append([]string(nil), m.order...) {
		ms := m.find(gid, userID)
		if ms == nil {
			continue
		}
		res := model.SuccessionResult{GroupID: gid}
		wasAdmin := ms.Role == model.RoleAdmin
		m.removeLocked(gid, userID)

		if wasAdmin && m.countAdminsLocked(gid) == 0 {
			if len(m.member[gid]) == 0 {
				m.deleteGroupLocked(gid)
				res.GroupDeleted = true
				results = append(results, res)
				continue
			}
			oldest := m.oldest(gid, "")
			oldest.Role = model.RoleAdmin
			id := oldest.UserID
			res.PromotedUserID = &id
		}
		if g := m.groups[gid]; g.OwnerUserID == userID {
			if a := m.oldest(gid, model.RoleAdmin); a != nil {
				g.OwnerUserID = a.UserID
			}
		}
		results = append(results, res)
	}
	for _, banned := range m.bans {
		delete(banned, userID)
	}
	return results, nil
}

// --- Helpers de siembra y consulta para tests ---

// AddMember agrega una membresía directamente (sin validar token ni baneo).
func (m *MemoryGroupStore) AddMember(groupID, userID, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addMemberLocked(groupID, userID, role)
}

// BannedBy devuelve quién baneó al usuario en el grupo.
func (m *MemoryGroupStore) BannedBy(groupID, userID string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	by, ok := m.bans[groupID][userID]
	return by, ok
}

// GroupExists indica si el grupo sigue existiendo.
func (m *MemoryGroupStore) GroupExists(groupID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.groups[groupID]
	return ok
}

// --- Internos (se llaman con m.mu tomado) ---

func (m *MemoryGroupStore) addMemberLocked(groupID, userID, role string) {
	m.member[groupID] = append(m.member[groupID], &model.GroupMembership{
		ID: uuid.NewString(), GroupID: groupID, UserID: userID, Role: role, JoinedAt: m.clock(),
	})
}

func (m *MemoryGroupStore) find(groupID, userID string) *model.GroupMembership {
	for _, ms := range m.member[groupID] {
		if ms.UserID == userID {
			return ms
		}
	}
	return nil
}

func (m *MemoryGroupStore) removeLocked(groupID, userID string) {
	list := m.member[groupID]
	for i, ms := range list {
		if ms.UserID == userID {
			m.member[groupID] = append(list[:i:i], list[i+1:]...)
			return
		}
	}
}

func (m *MemoryGroupStore) banLocked(groupID, userID, by string) {
	if m.bans[groupID] == nil {
		m.bans[groupID] = map[string]string{}
	}
	if _, exists := m.bans[groupID][userID]; !exists {
		m.bans[groupID][userID] = by
	}
}

func (m *MemoryGroupStore) deleteGroupLocked(groupID string) {
	delete(m.groups, groupID)
	delete(m.member, groupID)
	delete(m.bans, groupID)
	for i, id := range m.order {
		if id == groupID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

func (m *MemoryGroupStore) requireAdminLocked(groupID, userID string) error {
	ms := m.find(groupID, userID)
	if ms == nil || ms.Role != model.RoleAdmin {
		return errors.New("forbidden")
	}
	return nil
}

func (m *MemoryGroupStore) requireRemovableLocked(groupID, targetUserID string) error {
	ms := m.find(groupID, targetUserID)
	if ms == nil {
		return errors.New("target_not_found")
	}
	if ms.Role == model.RoleAdmin {
		return errors.New("target_is_admin")
	}
	return nil
}

func (m *MemoryGroupStore) countAdminsLocked(groupID string) int {
	n := 0
	for _, ms := range m.member[groupID] {
		if ms.Role == model.RoleAdmin {
			n++
		}
	}
	return n
}

// oldest devuelve el miembro más antiguo (opcionalmente filtrado por rol).
func (m *MemoryGroupStore) oldest(groupID, role string) *model.GroupMembership {
	var best *model.GroupMembership
	for _, ms := range m.member[groupID] {
		if role != "" && ms.Role != role {
			continue
		}
		if best == nil || ms.JoinedAt.Before(best.JoinedAt) {
			best = ms
		}
	}
	return best
}

// newestFirst devuelve los ids de grupo del más nuevo al más antiguo.
func (m *MemoryGroupStore) newestFirst() []string {
	ids := append([]string(nil), m.order...)
	sort.SliceStable(ids, func(i, j int) bool {
		return m.groups[ids[i]].CreatedAt.After(m.groups[ids[j]].CreatedAt)
	})
	return ids
}
