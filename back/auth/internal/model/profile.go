package model

import "time"

// Profile representa identity.profiles según agentSql.md:40.
// Campos nulables usan *string para distinguir NULL vs valor.
// Email (de identity.users) lo completa el Service de forma best-effort;
// omitempty mantiene compatibilidad con clientes que no lo esperan.
type Profile struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	PhotoURL    *string   `json:"photo_url"`
	Phone       *string   `json:"phone"`
	Institution *string   `json:"institution"`
	Description *string   `json:"description"`
	Visibility  string    `json:"visibility"` // public | private
	Email       *string   `json:"email,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

func IsValidVisibility(v string) bool {
	return v == VisibilityPublic || v == VisibilityPrivate
}
