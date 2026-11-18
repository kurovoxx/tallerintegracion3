package model

import "time"

// Group representa una fila de social.groups.
// Nota: sin campo "type" — se eliminó junto con el rol de profesor (ver agentMain.md).
type Group struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Description            *string   `json:"description,omitempty"`
	PhotoURL               *string   `json:"photo_url,omitempty"`
	PhotoThumbURL          *string   `json:"photo_thumb_url,omitempty"`
	OwnerUserID            string    `json:"owner_user_id"`
	NotesRestrictedToStaff bool      `json:"notes_restricted_to_staff"`
	InviteToken            string    `json:"invite_token"`
	CreatedAt              time.Time `json:"created_at"`
}

// GroupUpdate es el PATCH de un grupo. Los flags distinguen "no enviado" de
// "enviado vacío" para poder borrar la foto sin tocar los demás campos.
type GroupUpdate struct {
	Name             string
	Description      *string
	PhotoURL         *string
	PhotoThumbURL    *string
	UpdatePhotoURL   bool
	UpdatePhotoThumb bool
}

// GroupMembership representa una fila de social.group_memberships.
// Role: "admin" | "member" (ver agentSql.md — ya no existe student/teacher a este nivel).
type GroupMembership struct {
	ID       string    `json:"id"`
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	MemberPermissions
}

// MemberPermissions son los permisos extra que un admin delega a un miembro
// común. Los admins los tienen siempre (ver Effective). Todos parten en false:
// por defecto solo los admins comparten apuntes, agendan reuniones y editan el
// kanban del sprint.
type MemberPermissions struct {
	CanShareNotes     bool `json:"can_share_notes"`
	CanCreateMeetings bool `json:"can_create_meetings"`
	CanEditKanban     bool `json:"can_edit_kanban"`
}

// MemberPermissionsUpdate es el PATCH de permisos; nil = no tocar ese permiso.
type MemberPermissionsUpdate struct {
	CanShareNotes     *bool
	CanCreateMeetings *bool
	CanEditKanban     *bool
}

// Effective devuelve los permisos reales según el rol: un admin puede todo.
func (p MemberPermissions) Effective(role string) MemberPermissions {
	if role == RoleAdmin {
		return MemberPermissions{CanShareNotes: true, CanCreateMeetings: true, CanEditKanban: true}
	}
	return p
}

// MemberView enriquece GroupMembership con datos públicos de Identity.
// DisplayName/Email se omiten cuando no se pudieron resolver
// (backward-compatible con clientes que solo esperan IDs+rol).
type MemberView struct {
	ID       string    `json:"id"`
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
	MemberPermissions
	DisplayName   *string `json:"display_name,omitempty"`
	Email         *string `json:"email,omitempty"`
	PhotoThumbURL *string `json:"photo_thumb_url,omitempty"`
}

type MyGroup struct {
	GroupID string `json:"group_id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
}

const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// GroupView es la representación pública de un grupo para un miembro.
// El invite_token solo se incluye cuando quien consulta es admin: si no, un
// miembro (o cualquiera con el id) podría reinvitar a usuarios baneados.
type GroupView struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Description            *string   `json:"description,omitempty"`
	PhotoURL               *string   `json:"photo_url,omitempty"`
	PhotoThumbURL          *string   `json:"photo_thumb_url,omitempty"`
	OwnerUserID            string    `json:"owner_user_id"`
	NotesRestrictedToStaff bool      `json:"notes_restricted_to_staff"`
	InviteToken            string    `json:"invite_token,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	Role                   string    `json:"role"` // rol de quien consulta en el grupo
	MemberCount            int       `json:"member_count"`
}

// SuccessionResult describe qué ocurrió con un grupo al eliminar la cuenta de
// uno de sus miembros (sucesión automática de admin).
type SuccessionResult struct {
	GroupID        string  `json:"group_id"`
	PromotedUserID *string `json:"promoted_user_id,omitempty"` // miembro más antiguo promovido a admin
	GroupDeleted   bool    `json:"group_deleted"`              // el usuario era el último miembro
}
