package model

import "time"

// Absence representa academic.absences (agentSql.md).
// Date es date NOT NULL (día de la falta); Justified indica si está
// justificada (las justificadas no restan en el porcentaje).
type Absence struct {
	ID        string    `json:"id" db:"id"`
	SubjectID string    `json:"subject_id" db:"subject_id"`
	Date      time.Time `json:"date" db:"date"`
	Justified bool      `json:"justified" db:"justified"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
