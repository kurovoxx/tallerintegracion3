package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// PassingGrade es la nota de aprobación en escala 1.0-7.0 (igual que el front).
const PassingGrade = 4.0

// AverageResult es la respuesta de GET /subjects/{id}/average.
// Average nil = nada califica (status pending). Solo lectura: no persiste nada.
type AverageResult struct {
	Average *float64 `json:"average"`
	Status  string   `json:"status"`
}

// AverageService calcula el promedio ponderado y el estado derivado.
type AverageService struct {
	subjects SubjectStore
	grades   GradeStore
	reqs     RequirementStore
}

func NewAverageService(subjects SubjectStore, grades GradeStore, reqs RequirementStore) *AverageService {
	return &AverageService{subjects: subjects, grades: grades, reqs: reqs}
}

// effectiveScore calcula el valor efectivo (escala 10-70) de una nota:
// con hijas → promedio ponderado normalizado de hijas con nota (misma
// fórmula del front: Σ(score×peso)/Σ(peso)); sin hijas → su score propio.
// Una rama sin hijas con nota se ignora (ok=false). Los pesos 0 no ponderan;
// si Σ(pesos)==0 no hay valor. Sin redistribución automática: la suma de
// pesos no necesita ser 1. Recursivo con guarda de ciclos.
func effectiveScore(g model.Grade, byParent map[string][]model.Grade, visited map[string]bool) (float64, bool) {
	children := byParent[g.ID]
	if len(children) == 0 {
		if g.Score == nil {
			return 0, false
		}
		return float64(*g.Score), true
	}
	if visited[g.ID] {
		return 0, false
	}
	visited[g.ID] = true
	var sum, sumW float64
	for _, ch := range children {
		v, ok := effectiveScore(ch, byParent, visited)
		if !ok {
			continue
		}
		sum += v * ch.Weight
		sumW += ch.Weight
	}
	if sumW == 0 {
		return 0, false
	}
	return sum / sumW, true
}

func applyComparator(value, threshold float64, op string) bool {
	switch op {
	case model.ComparatorGT:
		return value > threshold
	case model.ComparatorGE:
		return value >= threshold
	case model.ComparatorLT:
		return value < threshold
	case model.ComparatorLE:
		return value <= threshold
	case model.ComparatorEQ:
		return value == threshold
	default:
		return false
	}
}

// Average calcula el promedio de una materia propia y su estado derivado.
// Orden de requisitos: force_fail > force_exam > force_pass; si ninguno
// dispara, aprobado si promedio >= 4.0. Umbrales en escala 1.0-7.0.
func (s *AverageService) Average(ctx context.Context, userID, subjectID string) (*AverageResult, error) {
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
	all, owned, err := s.grades.ListBySubject(ctx, subjectID, uid)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, NewServiceError(utils.ErrNotFound)
	}
	byParent := make(map[string][]model.Grade)
	var tops []model.Grade
	for _, g := range all {
		if g.ParentGradeID != nil {
			byParent[*g.ParentGradeID] = append(byParent[*g.ParentGradeID], g)
		} else {
			tops = append(tops, g)
		}
	}
	eff := make(map[string]float64) // gradeID → valor efectivo (10-70)
	var sum, sumW float64
	for _, g := range tops {
		v, ok := effectiveScore(g, byParent, map[string]bool{})
		if !ok {
			continue
		}
		eff[g.ID] = v
		sum += v * g.Weight
		sumW += g.Weight
	}
	if len(eff) == 0 || sumW == 0 {
		// Nada califica: promedio null, ningún requisito puede disparar.
		return &AverageResult{Status: model.AveragePending}, nil
	}
	avg := RoundHalfUp(sum/sumW/10, sub.AverageDecimalPlaces)

	reqs, _, err := s.reqs.ListBySubject(ctx, subjectID, uid)
	if err != nil {
		return nil, err
	}
	fires := func(r model.Requirement) bool {
		var value float64
		if r.ChecksFinalAverage {
			value = avg
		} else {
			v, ok := eff[*r.SourceGradeID]
			if !ok {
				return false // fuente pendiente o rama ignorada: no dispara
			}
			value = RoundHalfUp(v/10, sub.AverageDecimalPlaces)
		}
		return applyComparator(value, r.ThresholdValue, r.Comparator)
	}
	for _, action := range []string{model.ActionForceFail, model.ActionForceExam, model.ActionForcePass} {
		for _, r := range reqs {
			if r.Action == action && fires(r) {
				switch action {
				case model.ActionForceFail:
					return &AverageResult{Average: &avg, Status: model.AverageFailed}, nil
				case model.ActionForceExam:
					return &AverageResult{Average: &avg, Status: model.AverageExamRequired}, nil
				default:
					return &AverageResult{Average: &avg, Status: model.AverageApproved}, nil
				}
			}
		}
	}
	status := model.AverageFailed
	if avg >= PassingGrade {
		status = model.AverageApproved
	}
	return &AverageResult{Average: &avg, Status: status}, nil
}
