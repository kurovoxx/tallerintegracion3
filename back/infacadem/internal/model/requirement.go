package model

import "time"

// Comparadores válidos de academic.grade_requirements.comparator.
const (
	ComparatorGT = ">"
	ComparatorGE = ">="
	ComparatorLT = "<"
	ComparatorLE = "<="
	ComparatorEQ = "="
)

// IsValidComparator reporta si c pertenece al enum de comparadores.
func IsValidComparator(c string) bool {
	switch c {
	case ComparatorGT, ComparatorGE, ComparatorLT, ComparatorLE, ComparatorEQ:
		return true
	default:
		return false
	}
}

// Acciones válidas de academic.grade_requirements.action.
const (
	ActionForceExam = "force_exam"
	ActionForceFail = "force_fail"
	ActionForcePass = "force_pass"
)

// IsValidRequirementAction reporta si a pertenece al enum de acciones.
func IsValidRequirementAction(a string) bool {
	switch a {
	case ActionForceExam, ActionForceFail, ActionForcePass:
		return true
	default:
		return false
	}
}

// Estados derivados de GET /subjects/{id}/average (no se persisten).
const (
	AverageApproved     = "approved"
	AverageFailed       = "failed"
	AverageExamRequired = "exam_required"
	AveragePending      = "pending"
)

// Requirement representa academic.grade_requirements (agentSql.md).
// Fuente del requisito: exactamente una de las dos (XOR, validado en service):
// ChecksFinalAverage=true (compara el promedio de la materia) o
// SourceGradeID (compara el valor efectivo de esa nota).
// ThresholdValue es decimal(5,2) y por contrato vive en escala 1.0-7.0
// (igual que la aprobación 4.0 del front); los valores de notas (10-70)
// se convierten (/10) antes de comparar.
type Requirement struct {
	ID                 string    `json:"id" db:"id"`
	SubjectID          string    `json:"subject_id" db:"subject_id"`
	SourceGradeID      *string   `json:"source_grade_id" db:"source_grade_id"`
	ChecksFinalAverage bool      `json:"checks_final_average" db:"checks_final_average"`
	Comparator         string    `json:"comparator" db:"comparator"`
	ThresholdValue     float64   `json:"threshold_value" db:"threshold_value"`
	Action             string    `json:"action" db:"action"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
}
