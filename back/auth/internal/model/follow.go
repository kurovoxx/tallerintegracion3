package model

import "time"

// UserFollow representa una relación de "seguir" en identity.user_follows.
type UserFollow struct {
	FollowerID   string    `json:"follower_id"`
	FollowingID  string    `json:"following_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// FollowStatus indica si el usuario autenticado sigue a un perfil público.
type FollowStatus struct {
	PublicID string `json:"public_id"`
	Following bool  `json:"following"`
}

// FollowedUser es un usuario seguido (lista de following/followers).
// Incluye datos mínimos públicos para mostrar en la UI.
type FollowedUser struct {
	UserID        string  `json:"user_id"`
	PublicID      *string `json:"public_id,omitempty"`
	DisplayName   string  `json:"display_name"`
	PhotoThumbURL *string `json:"photo_thumb_url,omitempty"`
	FollowedAt    time.Time `json:"followed_at"`
}
