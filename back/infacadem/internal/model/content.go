package model

// Content representa academic.contents (agentSql.md, UX-05).
// Unidades de una materia: Title NOT NULL, Body text nulable y Position
// para orden (duplicados permitidos, reorden simple por position).
type Content struct {
	ID        string  `json:"id" db:"id"`
	SubjectID string  `json:"subject_id" db:"subject_id"`
	Title     string  `json:"title" db:"title"`
	Body      *string `json:"body" db:"body"`
	Position  int     `json:"position" db:"position"`
	Done      bool    `json:"done" db:"done"`
}
