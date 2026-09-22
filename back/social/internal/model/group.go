package model

import "time"

// Group representa una fila de social.groups.
// Nota: sin campo "type" — se eliminó junto con el rol de profesor (ver agentMain.md).
type Group struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Description            *string   `json:"description,omitempty"`
	OwnerUserID            string    `json:"owner_user_id"`
	NotesRestrictedToStaff bool      `json:"notes_restricted_to_staff"`
	InviteToken            string    `json:"invite_token"`
	CreatedAt              time.Time `json:"created_at"`
}

// GroupMembership representa una fila de social.group_memberships.
// Role: "admin" | "member" (ver agentSql.md — ya no existe student/teacher a este nivel).
type GroupMembership struct {
	ID       string    `json:"id"`
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
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
	OwnerUserID            string    `json:"owner_user_id"`
	NotesRestrictedToStaff bool      `json:"notes_restricted_to_staff"`
	InviteToken            string    `json:"invite_token,omitempty"`
	CreatedAt              time.Time `json:"created_at"`
	Role                   string    `json:"role"` // rol de quien consulta en el grupo
}

// SuccessionResult describe qué ocurrió con un grupo al eliminar la cuenta de
// uno de sus miembros (sucesión automática de admin).
type SuccessionResult struct {
	GroupID        string  `json:"group_id"`
	PromotedUserID *string `json:"promoted_user_id,omitempty"` // miembro más antiguo promovido a admin
	GroupDeleted   bool    `json:"group_deleted"`              // el usuario era el último miembro
}
