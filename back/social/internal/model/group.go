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
}

// MemberView enriquece GroupMembership con datos públicos de Identity.
// DisplayName/Email/PublicID se omiten cuando no se pudieron resolver
// (backward-compatible con clientes que solo esperan IDs+rol). PublicID es el
// código público del perfil: la interfaz lo muestra en lugar de UserID.
type MemberView struct {
	ID            string    `json:"id"`
	GroupID       string    `json:"group_id"`
	UserID        string    `json:"user_id"`
	Role          string    `json:"role"`
	JoinedAt      time.Time `json:"joined_at"`
	DisplayName   *string   `json:"display_name,omitempty"`
	Email         *string   `json:"email,omitempty"`
	PhotoThumbURL *string   `json:"photo_thumb_url,omitempty"`
	PublicID      *string   `json:"public_id,omitempty"`
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
