package model

import "time"

// GroupInvitation representa una invitación pendiente de un admin de grupo a
// un usuario seguido. Estados: pending (espera respuesta), accepted (se unió
// al grupo), rejected (rechazada).
type GroupInvitation struct {
	ID              string     `json:"id"`
	GroupID         string     `json:"group_id"`
	InvitedUserID   string     `json:"invited_user_id"`
	InvitedByUserID string     `json:"invited_by_user_id"`
	Status          string     `json:"status"` // pending, accepted, rejected
	CreatedAt       time.Time  `json:"created_at"`
	RespondedAt     *time.Time `json:"responded_at,omitempty"`
}

// GroupInvitationView es lo que el front necesita para mostrar una invitación
// pendiente: datos del grupo y del invitador.
type GroupInvitationView struct {
	ID              string    `json:"id"`
	GroupID         string    `json:"group_id"`
	GroupName       string    `json:"group_name"`
	InvitedByUserID string    `json:"invited_by_user_id"`
	InvitedByName   *string   `json:"invited_by_name,omitempty"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// InviteToGroupRequest es el body de POST /groups/:id/invite.
type InviteToGroupRequest struct {
	InvitedUserID string `json:"invited_user_id" binding:"required"`
}

// AcceptRejectInvitationRequest es el body de POST /group-invitations/:id/accept|reject.
// Body vacío está permitido (solo se valida el id en la ruta).
type AcceptRejectInvitationRequest struct{}
