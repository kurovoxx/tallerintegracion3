package model

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// PublicUser expone datos mínimos no sensibles para resolución
// servicio-a-servicio (p. ej. nombres de miembros de un grupo).
// DisplayName puede ser nil si el perfil no lo define.
type PublicUser struct {
	UserID      string  `json:"user_id"`
	Email       string  `json:"email"`
	DisplayName *string `json:"display_name,omitempty"`
}
