package model

import "time"

// Schedule representa academic.schedules (agentSql.md).
// DayOfWeek es int NOT NULL válido solo 0..6 (fuera de rango: inválido).
// StartTime y EndTime son time NOT NULL (hora del día); EndTime debe ser
// estrictamente mayor que StartTime (end_time <= start_time: inválido).
// Modality es varchar(50) nulable (NULL si no se informa).
type Schedule struct {
	ID        string    `json:"id" db:"id"`
	SubjectID string    `json:"subject_id" db:"subject_id"`
	DayOfWeek int       `json:"day_of_week" db:"day_of_week"`
	StartTime time.Time `json:"start_time" db:"start_time"`
	EndTime   time.Time `json:"end_time" db:"end_time"`
	Modality  *string   `json:"modality" db:"modality"`
}

// IsValidDayOfWeek reporta si d está en el rango 0..6.
func IsValidDayOfWeek(d int) bool {
	return d >= 0 && d <= 6
}
