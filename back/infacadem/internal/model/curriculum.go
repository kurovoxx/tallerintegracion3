package model

import "time"

// Curriculum representa academic.curricula (agentSql.md).
// user_id es referencia LÓGICA a identity.users.id (sin FK física).
type Curriculum struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Name      string    `json:"name" db:"name"` // varchar(200) NOT NULL
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
