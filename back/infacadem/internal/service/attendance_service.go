package service

import (
	"context"
	"math"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// AttendanceSummary es la respuesta de GET /subjects/{id}/attendance.
// Solo lectura: no persiste nada.
// Percentage es fracción 0-1 (null si total_classes null/0). Divergencia
// documentada con el front: attendance_screen muestra percentage*100 con
// clamp(0,100) para display; aquí va el valor crudo de la fórmula.
// Remaining puede ser negativo si ya se excedió el mínimo (igual que el front).
type AttendanceSummary struct {
	TotalClasses     *int     `json:"total_classes"`
	UnjustifiedCount int      `json:"unjustified_count"`
	JustifiedCount   int      `json:"justified_count"`
	Percentage       *float64 `json:"percentage"`
	MinAttendancePct *float64 `json:"min_attendance_pct"`
	MeetsMinimum     *bool    `json:"meets_minimum"`
	Remaining        *int     `json:"remaining"`
}

// AttendanceService calcula el resumen de asistencia de una materia propia.
type AttendanceService struct {
	subjects SubjectStore
	absences AbsenceStore
}

func NewAttendanceService(subjects SubjectStore, absences AbsenceStore) *AttendanceService {
	return &AttendanceService{subjects: subjects, absences: absences}
}

// Summary aplica la fórmula oficial: % = 1 - (injustificadas/total);
// justificadas no restan. total null/0 → percentage/meets/remaining null;
// min null → meets/remaining null (percentage sí calcula).
func (s *AttendanceService) Summary(ctx context.Context, userID, subjectID string) (*AttendanceSummary, error) {
	if err := requireUser(userID); err != nil {
		return nil, err
	}
	subjectID, err := ValidateUUID(subjectID)
	if err != nil {
		return nil, err
	}
	uid := strings.TrimSpace(userID)
	sub, err := s.subjects.GetByID(ctx, subjectID, uid)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	unjustified, justified, owned, err := s.absences.CountsBySubject(ctx, subjectID, uid)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, NewServiceError(utils.ErrNotFound)
	}

	out := &AttendanceSummary{
		TotalClasses:     sub.TotalClasses,
		UnjustifiedCount: unjustified,
		JustifiedCount:   justified,
		MinAttendancePct: sub.MinAttendancePct,
	}
	totalOK := sub.TotalClasses != nil && *sub.TotalClasses > 0
	if totalOK {
		p := 1 - float64(unjustified)/float64(*sub.TotalClasses)
		out.Percentage = &p
	}
	if totalOK && sub.MinAttendancePct != nil {
		meets := *out.Percentage >= *sub.MinAttendancePct/100
		out.MeetsMinimum = &meets
		remaining := int(math.Floor(float64(*sub.TotalClasses)*(1-*sub.MinAttendancePct/100))) - unjustified
		out.Remaining = &remaining
	}
	return out, nil
}
