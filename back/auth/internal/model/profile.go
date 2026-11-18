package model

import "time"

// Profile representa identity.profiles según agentSql.md:40.
// Campos nulables usan *string para distinguir NULL vs valor.
// Email (de identity.users) lo completa el Service de forma best-effort;
// omitempty mantiene compatibilidad con clientes que no lo esperan.
type Profile struct {
	UserID        string    `json:"user_id"`
	PublicID      string    `json:"public_id"`
	DisplayName   string    `json:"display_name"`
	PhotoURL      *string   `json:"photo_url"`
	PhotoThumbURL *string   `json:"photo_thumb_url"`
	Phone         *string   `json:"phone"`
	Institution   *string   `json:"institution"`
	Description   *string   `json:"description"`
	Visibility    string    `json:"visibility"` // public | private
	Email         *string   `json:"email,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PublicProfile es la vista de un perfil consultado por su código público
// (GET /profile/public/:public_id). El repositorio carga todos los campos que
// el dueño puede elegir publicar y ProfileService.GetPublicProfileByPublicID
// los recorta según la visibilidad antes de que lleguen al handler:
//   - público (o consultado por su dueño): nombre, foto, descripción, correo,
//     teléfono e institución;
//   - privado consultado por un tercero: solo nombre y foto.
//
// La foto solo se entrega si es una imagen subida desde la app (data URL).
// UserID se usa únicamente para comparar con el solicitante y no se serializa.
type PublicProfile struct {
	UserID      string  `json:"-"`
	PublicID    string  `json:"public_id"`
	DisplayName string  `json:"display_name"`
	PhotoURL    *string `json:"photo_url"`
	Institution *string `json:"institution,omitempty"`
	Description *string `json:"description,omitempty"`
	Email       *string `json:"email,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	Visibility  string  `json:"visibility"` // public | private
}

const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

func IsValidVisibility(v string) bool {
	return v == VisibilityPublic || v == VisibilityPrivate
}
