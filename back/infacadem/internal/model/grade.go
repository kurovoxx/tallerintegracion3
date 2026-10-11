package model

import "time"

// Estados válidos de academic.grades.status.
const (
	GradeStatusGraded  = "graded"
	GradeStatusPending = "pending"
	GradeStatusExcused = "excused"
	GradeStatusMissing = "missing"
)

// IsValidGradeStatus reporta si s pertenece al enum de status.
func IsValidGradeStatus(s string) bool {
	switch s {
	case GradeStatusGraded, GradeStatusPending, GradeStatusExcused, GradeStatusMissing:
		return true
	default:
		return false
	}
}

// Grade representa academic.grades (agentSql.md).
// Score es smallint 10-70 nulable: nil = evaluación programada a futuro.
// Weight es decimal(4,3) NOT NULL y por contrato se almacena como
// fracción 0-1 (el front divide su % por 100 al enviar); sin
// redistribución automática en ningún caso.
// ScheduledDate es date nulable. Una nota con hijos ignora su Score
// propio: su valor efectivo es el promedio ponderado de sus hijos con nota.
type Grade struct {
	ID                    string     `json:"id" db:"id"`
	SubjectID             string     `json:"subject_id" db:"subject_id"`
	ParentGradeID         *string    `json:"parent_grade_id" db:"parent_grade_id"`
	IsFinalExam           bool       `json:"is_final_exam" db:"is_final_exam"`
	Status                string     `json:"status" db:"status"`
	Name                  string     `json:"name" db:"name"` // varchar(200) NOT NULL
	Score                 *int       `json:"score" db:"score"`
	Weight                float64    `json:"weight" db:"weight"`
	ScheduledDate         *time.Time `json:"scheduled_date" db:"scheduled_date"`
	Topics                *string    `json:"topics" db:"topics"`
	NotifyDiscord         bool       `json:"notify_discord" db:"notify_discord"`
	GoogleCalendarEventID *string    `json:"google_calendar_event_id" db:"google_calendar_event_id"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
}
