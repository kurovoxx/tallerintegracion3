package model

import "time"

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	PublicID     string    `json:"public_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// PublicUser expone datos mínimos no sensibles para resolución
// servicio-a-servicio (p. ej. nombres de miembros de un grupo).
// DisplayName/PhotoThumbURL pueden ser nil si el perfil no los define.
// PublicID es el código público (identity.users.public_id): lo único que los
// consumidores pueden mostrar en la interfaz en lugar del UUID interno.
type PublicUser struct {
	UserID        string  `json:"user_id"`
	Email         string  `json:"email"`
	DisplayName   *string `json:"display_name,omitempty"`
	PhotoThumbURL *string `json:"photo_thumb_url,omitempty"`
	PublicID      *string `json:"public_id,omitempty"`
}
