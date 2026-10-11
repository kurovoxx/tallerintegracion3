package model

// Estados válidos de academic.subjects.status.
const (
	SubjectStatusInProgress = "in_progress"
	SubjectStatusApproved   = "approved"
	SubjectStatusFailed     = "failed"
	SubjectStatusPending    = "pending"
)

// IsValidSubjectStatus reporta si s pertenece al enum de status.
func IsValidSubjectStatus(s string) bool {
	switch s {
	case SubjectStatusInProgress, SubjectStatusApproved, SubjectStatusFailed, SubjectStatusPending:
		return true
	default:
		return false
	}
}

// Subject representa academic.subjects (agentSql.md).
// Semester, Credits y TotalClasses son nulables (NULL si no se informan).
// MinAttendancePct es decimal(5,2) nulable.
// AverageDecimalPlaces es smallint NOT NULL DEFAULT 2, válido solo 0..2.
// Status es varchar(20) NOT NULL DEFAULT 'in_progress', válido solo el enum.
type Subject struct {
	ID                   string   `json:"id" db:"id"`
	CurriculumID         string   `json:"curriculum_id" db:"curriculum_id"`
	Name                 string   `json:"name" db:"name"` // varchar(200) NOT NULL
	Semester             *int     `json:"semester" db:"semester"`
	Credits              *int     `json:"credits" db:"credits"`
	TotalClasses         *int     `json:"total_classes" db:"total_classes"`
	MinAttendancePct     *float64 `json:"min_attendance_pct" db:"min_attendance_pct"`
	AverageDecimalPlaces int      `json:"average_decimal_places" db:"average_decimal_places"`
	Status               string   `json:"status" db:"status"`
}
