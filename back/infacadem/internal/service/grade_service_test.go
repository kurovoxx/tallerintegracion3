package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// Fakes con semántica de propiedad para grades/requirements.

type fakeGrades struct {
	mu    sync.Mutex
	items map[string]*model.Grade
	subs  map[string]string // subjectID -> userID (materias propias)
}

func newFakeGrades() *fakeGrades {
	return &fakeGrades{items: map[string]*model.Grade{}, subs: map[string]string{}}
}

func (f *fakeGrades) ownSubject(subjectID, userID string) bool {
	u, ok := f.subs[subjectID]
	return ok && u == userID
}

func (f *fakeGrades) Create(_ context.Context, userID, subjectID string, in repository.GradeCreate) (*model.Grade, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ownSubject(subjectID, userID) {
		return nil, nil
	}
	g := &model.Grade{ID: uuid.NewString(), SubjectID: subjectID, ParentGradeID: in.ParentGradeID,
		IsFinalExam: in.IsFinalExam, Status: in.Status, Name: in.Name, Score: in.Score,
		Weight: in.Weight, ScheduledDate: in.ScheduledDate, Topics: in.Topics,
		NotifyDiscord: in.NotifyDiscord, GoogleCalendarEventID: in.GoogleCalendarEventID,
		CreatedAt: time.Now()}
	f.items[g.ID] = g
	cp := *g
	return &cp, nil
}

func (f *fakeGrades) GetByID(_ context.Context, id, userID string) (*model.Grade, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.items[id]; ok && f.ownSubject(g.SubjectID, userID) {
		cp := *g
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeGrades) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Grade, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.ownSubject(subjectID, userID) {
		return nil, false, nil
	}
	out := []model.Grade{}
	for _, g := range f.items {
		if g.SubjectID == subjectID {
			out = append(out, *g)
		}
	}
	return out, true, nil
}

func (f *fakeGrades) Update(_ context.Context, id, userID string, p repository.GradePatch) (*model.Grade, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.items[id]
	if !ok || !f.ownSubject(g.SubjectID, userID) {
		return nil, nil
	}
	if p.Name != nil {
		g.Name = *p.Name
	}
	if p.ClearScore {
		g.Score = nil
	} else if p.Score != nil {
		g.Score = p.Score
	}
	if p.Weight != nil {
		g.Weight = *p.Weight
	}
	if p.Status != nil {
		g.Status = *p.Status
	}
	if p.ClearScheduledAt {
		g.ScheduledDate = nil
	} else if p.ScheduledDate != nil {
		g.ScheduledDate = p.ScheduledDate
	}
	if p.ClearTopics {
		g.Topics = nil
	} else if p.Topics != nil {
		g.Topics = p.Topics
	}
	if p.NotifyDiscord != nil {
		g.NotifyDiscord = *p.NotifyDiscord
	}
	if p.ClearCalendarEvt {
		g.GoogleCalendarEventID = nil
	} else if p.GoogleCalendarEventID != nil {
		g.GoogleCalendarEventID = p.GoogleCalendarEventID
	}
	if p.IsFinalExam != nil {
		g.IsFinalExam = *p.IsFinalExam
	}
	cp := *g
	return &cp, nil
}

func (f *fakeGrades) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.items[id]; ok && f.ownSubject(g.SubjectID, userID) {
		delete(f.items, id)
		for cid, ch := range f.items {
			if ch.ParentGradeID != nil && *ch.ParentGradeID == id {
				delete(f.items, cid)
			}
		}
		return true, nil
	}
	return false, nil
}

func (f *fakeGrades) CountFinalExams(_ context.Context, subjectID, userID, excludeID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, g := range f.items {
		if g.SubjectID == subjectID && g.IsFinalExam && g.ID != excludeID && f.ownSubject(subjectID, userID) {
			n++
		}
	}
	return n, nil
}

type fakeRequirements struct {
	mu    sync.Mutex
	items map[string]*model.Requirement
	subs  map[string]string
}

func newFakeRequirements(subs map[string]string) *fakeRequirements {
	return &fakeRequirements{items: map[string]*model.Requirement{}, subs: subs}
}

func (f *fakeRequirements) Create(_ context.Context, userID, subjectID string, source *string, checks bool, comp string, thr float64, action string) (*model.Requirement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.subs[subjectID]; !ok || u != userID {
		return nil, nil
	}
	r := &model.Requirement{ID: uuid.NewString(), SubjectID: subjectID, SourceGradeID: source,
		ChecksFinalAverage: checks, Comparator: comp, ThresholdValue: thr, Action: action, CreatedAt: time.Now()}
	f.items[r.ID] = r
	cp := *r
	return &cp, nil
}

func (f *fakeRequirements) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Requirement, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.subs[subjectID]; !ok || u != userID {
		return nil, false, nil
	}
	out := []model.Requirement{}
	for _, r := range f.items {
		if r.SubjectID == subjectID {
			out = append(out, *r)
		}
	}
	return out, true, nil
}

func (f *fakeRequirements) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r, ok := f.items[id]; ok {
		if u, ok2 := f.subs[r.SubjectID]; ok2 && u == userID {
			delete(f.items, id)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRequirements) GradeSubjectOf(_ context.Context, gradeID, subjectID, userID string) (bool, error) {
	return true, nil // los tests de service validan pertenencia vía fakeGrades aparte
}

// subjectStore real (fakes de academic_service_test.go) para AverageService,
// con average_decimal_places configurable por materia.
func avgSubjectStore(subs *fakeSubjects, decimals map[string]int) *avgSubjectAdapter {
	return &avgSubjectAdapter{subs: subs, decimals: decimals}
}

type avgSubjectAdapter struct {
	subs     *fakeSubjects
	decimals map[string]int
}

func (a *avgSubjectAdapter) Create(ctx context.Context, userID, curriculumID string, in repository.SubjectCreate) (*model.Subject, error) {
	return a.subs.Create(ctx, userID, curriculumID, in)
}
func (a *avgSubjectAdapter) GetByID(_ context.Context, id, userID string) (*model.Subject, error) {
	a.subs.mu.Lock()
	defer a.subs.mu.Unlock()
	a.subs.cur.mu.Lock()
	defer a.subs.cur.mu.Unlock()
	s, ok := a.subs.items[id]
	if !ok {
		return nil, nil
	}
	c, ok := a.subs.cur.items[s.CurriculumID]
	if !ok || c.UserID != userID {
		return nil, nil
	}
	cp := *s
	if d, ok := a.decimals[id]; ok {
		cp.AverageDecimalPlaces = d
	}
	return &cp, nil
}
func (a *avgSubjectAdapter) ListByCurriculum(ctx context.Context, curriculumID, userID string) ([]model.Subject, bool, error) {
	return a.subs.ListByCurriculum(ctx, curriculumID, userID)
}
func (a *avgSubjectAdapter) Update(ctx context.Context, id, userID string, p repository.SubjectPatch) (*model.Subject, error) {
	return a.subs.Update(ctx, id, userID, p)
}
func (a *avgSubjectAdapter) Delete(ctx context.Context, id, userID string) (bool, error) {
	return a.subs.Delete(ctx, id, userID)
}

func gradeFixture() (*GradeService, *RequirementService, *fakeGrades, *fakeRequirements, string) {
	g := newFakeGrades()
	subID := uuid.NewString()
	g.subs[subID] = userA
	otherSub := uuid.NewString()
	g.subs[otherSub] = userA
	foreignSub := uuid.NewString()
	g.subs[foreignSub] = userB
	r := newFakeRequirements(g.subs)
	return NewGradeService(g), NewRequirementService(r), g, r, subID
}

func floatPtr(f float64) *float64 { return &f }
func boolPtr(b bool) *bool        { return &b }

func TestGrade_Create_Validaciones(t *testing.T) {
	svc, _, _, _, subID := gradeFixture()
	ctx := context.Background()

	g, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("Prueba 1"), Weight: floatPtr(0.3)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if g.Status != "pending" || g.Score != nil {
		t.Fatalf("defaults: %+v", g)
	}
	if _, err := svc.Create(ctx, userA, subID, GradeInput{Weight: floatPtr(0.3)}); err == nil {
		t.Fatal("sin name 400")
	} else {
		mustCode(t, err, "invalid_name")
	}
	if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("X")}); err == nil {
		t.Fatal("sin weight 400")
	} else {
		mustCode(t, err, "invalid_weight")
	}
	for _, w := range []float64{-0.1, 1.5} {
		if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("X"), Weight: floatPtr(w)}); err == nil {
			t.Fatalf("weight %v 400", w)
		} else {
			mustCode(t, err, "invalid_weight")
		}
	}
	for _, s := range []int{9, 71} {
		if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("X"), Weight: floatPtr(0.5), Score: &s}); err == nil {
			t.Fatalf("score %d 400", s)
		} else {
			mustCode(t, err, "invalid_score")
		}
	}
	if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("X"), Weight: floatPtr(0.5), Status: strPtr("zzz")}); err == nil {
		t.Fatal("status malo 400")
	} else {
		mustCode(t, err, "invalid_status")
	}
	if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("X"), Weight: floatPtr(0.5), ScheduledDate: strPtr("ayer")}); err == nil {
		t.Fatal("fecha mala 400")
	} else {
		mustCode(t, err, "invalid_date")
	}
	if _, err := svc.Create(ctx, userB, subID, GradeInput{Name: strPtr("X"), Weight: floatPtr(0.5)}); err == nil {
		t.Fatal("materia ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Create(ctx, userA, "no-uuid", GradeInput{Name: strPtr("X"), Weight: floatPtr(0.5)}); err == nil {
		t.Fatal("uuid malo 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}

func TestGrade_FinalUnico_409(t *testing.T) {
	svc, _, _, _, subID := gradeFixture()
	ctx := context.Background()

	if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("Final"), Weight: floatPtr(0.4), IsFinalExam: boolPtr(true)}); err != nil {
		t.Fatalf("primer final: %v", err)
	}
	if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("Final 2"), Weight: floatPtr(0.4), IsFinalExam: boolPtr(true)}); err == nil {
		t.Fatal("segundo final 409")
	} else {
		mustCode(t, err, "conflict")
	}
	// Vía PATCH a otra nota también 409.
	other, _ := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("Otra"), Weight: floatPtr(0.2)})
	if _, err := svc.Update(ctx, userA, other.ID, GradeInput{IsFinalExam: boolPtr(true)}); err == nil {
		t.Fatal("patch a final duplicado 409")
	} else {
		mustCode(t, err, "conflict")
	}
}

func TestGrade_Hijos_Efectivo(t *testing.T) {
	svc, _, _, _, subID := gradeFixture()
	ctx := context.Background()

	// Réplica del front: hijos 70%/30% (fracciones .7/.3), scores 55 y 60.
	padre, _ := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("Padre"), Weight: floatPtr(1), Score: intPtr(10)})
	h1, _ := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H1"), Weight: floatPtr(0.7), Score: intPtr(55)})
	_, _ = svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H2"), Weight: floatPtr(0.3), Score: intPtr(60)})
	// Hijo pendiente (sin nota) no pondera.
	_, _ = svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H3"), Weight: floatPtr(0.9)})

	nodes, err := svc.List(ctx, userA, subID)
	if err != nil || len(nodes) != 1 || len(nodes[0].Children) != 3 {
		t.Fatalf("anidadas: %v %+v", err, nodes)
	}
	_ = h1

	// Promedio del padre: (55*.7+60*.3)/1.0 = 56.5 → ignora score propio.
	byParent := map[string][]model.Grade{
		padre.ID: {
			{ID: "a", Weight: 0.7, Score: intPtr(55)},
			{ID: "b", Weight: 0.3, Score: intPtr(60)},
			{ID: "c", Weight: 0.9},
		},
	}
	v, ok := effectiveScore(model.Grade{ID: padre.ID, Score: intPtr(10)}, byParent, map[string]bool{})
	if !ok || v != 56.5 {
		t.Fatalf("efectivo padre (ignora score propio): %v %v", v, ok)
	}
	// Padre sin hijas con nota → rama ignorada.
	v2, ok2 := effectiveScore(model.Grade{ID: "x"}, map[string][]model.Grade{"x": {{ID: "y", Weight: 0.5}}}, map[string]bool{})
	if ok2 {
		t.Fatalf("rama sin notas debe ignorarse, got %v", v2)
	}
}

func TestAverage_Calculo_Requisitos(t *testing.T) {
	// Armar vía fakes 3_1_2: malla + materia propias.
	cStore := newFakeCurricula()
	sStore := newFakeSubjects(cStore)
	malla, _ := NewCurriculumService(cStore).Create(context.Background(), userA, "M")
	mat, _ := NewSubjectService(sStore).Create(context.Background(), userA, malla.ID, SubjectInput{Name: strPtr("Mat")})

	g := newFakeGrades()
	g.subs[mat.ID] = userA
	gradeSvc := NewGradeService(g)
	reqSvc := NewRequirementService(newFakeRequirements(g.subs))
	avgSvc := NewAverageService(avgSubjectStore(sStore, map[string]int{mat.ID: 1}), g, reqSvc.repo)

	ctx := context.Background()
	s70 := 70
	s40 := 40
	if _, err := gradeSvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("A"), Weight: floatPtr(0.5), Score: &s70}); err != nil {
		t.Fatal(err)
	}
	if _, err := gradeSvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("B"), Weight: floatPtr(0.5), Score: &s40}); err != nil {
		t.Fatal(err)
	}
	// (70*.5+40*.5)/10 = 5.5, 1 decimal.
	res, err := avgSvc.Average(ctx, userA, mat.ID)
	if err != nil || res.Average == nil || *res.Average != 5.5 || res.Status != "approved" {
		t.Fatalf("promedio 5.5 approved: %+v %v", res, err)
	}
	// Redondeo: 5.55 → 5.6 con 1 decimal (half-up como el front).
	if got := RoundHalfUp(5.55, 1); got != 5.6 {
		t.Fatalf("round half-up: %v", got)
	}
	// force_exam con promedio >= 4.0 → exam_required.
	thr := 4.0
	tf := true
	if _, err := reqSvc.Create(ctx, userA, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: strPtr(">="), ThresholdValue: &thr, Action: strPtr("force_exam")}); err != nil {
		t.Fatal(err)
	}
	if res, _ := avgSvc.Average(ctx, userA, mat.ID); res.Status != "exam_required" {
		t.Fatalf("force_exam: %+v", res)
	}
	// force_fail domina a force_exam.
	thr2 := 6.0
	if _, err := reqSvc.Create(ctx, userA, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: strPtr("<"), ThresholdValue: &thr2, Action: strPtr("force_fail")}); err != nil {
		t.Fatal(err)
	}
	if res, _ := avgSvc.Average(ctx, userA, mat.ID); res.Status != "failed" {
		t.Fatalf("force_fail domina: %+v", res)
	}
	// Promedio bajo sin requisitos → failed.
	g2 := newFakeGrades()
	g2.subs[mat.ID] = userA
	gs2 := NewGradeService(g2)
	rq2 := newFakeRequirements(g2.subs)
	rs2 := NewRequirementService(rq2)
	av2 := NewAverageService(avgSubjectStore(sStore, map[string]int{mat.ID: 1}), g2, rq2)
	_ = rs2
	s30 := 30
	if _, err := gs2.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("C"), Weight: floatPtr(1), Score: &s30}); err != nil {
		t.Fatal(err)
	}
	if res, _ := av2.Average(ctx, userA, mat.ID); res.Average == nil || *res.Average != 3.0 || res.Status != "failed" {
		t.Fatalf("3.0 failed: %+v", res)
	}
	// Materia sin notas → null/pending.
	g3 := newFakeGrades()
	g3.subs[mat.ID] = userA
	av3 := NewAverageService(avgSubjectStore(sStore, map[string]int{mat.ID: 1}), g3, newFakeRequirements(g3.subs))
	if res, _ := av3.Average(ctx, userA, mat.ID); res.Average != nil || res.Status != "pending" {
		t.Fatalf("vacía null/pending: %+v", res)
	}
	// Cross-user y uuid malo.
	if _, err := av2.Average(ctx, userB, mat.ID); err == nil {
		t.Fatal("average ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := av2.Average(ctx, userA, "no-uuid"); err == nil {
		t.Fatal("uuid malo 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}

func TestRequirement_Validaciones(t *testing.T) {
	_, reqSvc, _, _, _ := gradeFixture()
	_ = reqSvc
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	ctx := context.Background()
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M")
	mat, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Mat")})
	otra, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Otra")})

	g := newFakeGrades()
	g.subs[mat.ID] = userA
	g.subs[otra.ID] = userA
	gsvc := NewGradeService(g)
	rsvc := NewRequirementService(newFakeRequirements(g.subs))
	nota, _ := gsvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("N"), Weight: floatPtr(1), Score: intPtr(50)})

	tf, ff := true, false
	ge, badComp := ">=", "=>"
	thr := 4.0
	fe := "force_exam"
	mkSrc := func(id string) *string { return &id }

	// XOR: ambas fuentes y ninguna → 400.
	if _, err := rsvc.Create(ctx, userA, mat.ID, RequirementInput{SourceGradeID: mkSrc(nota.ID), ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("ambas fuentes 400")
	} else {
		mustCode(t, err, "invalid_requirement")
	}
	if _, err := rsvc.Create(ctx, userA, mat.ID, RequirementInput{ChecksFinalAverage: &ff, Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("ninguna fuente 400")
	} else {
		mustCode(t, err, "invalid_requirement")
	}
	// Comparador/acción malos → 400.
	if _, err := rsvc.Create(ctx, userA, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &badComp, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("comparador malo 400")
	} else {
		mustCode(t, err, "invalid_comparator")
	}
	badAction := "explotar"
	if _, err := rsvc.Create(ctx, userA, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &badAction}); err == nil {
		t.Fatal("acción mala 400")
	} else {
		mustCode(t, err, "invalid_action")
	}
	// Fuente de otra materia → 400 (verificar con GradeSubjectOf realista).
	rsvc2 := NewRequirementService(&rejectingReqStore{subs: g.subs})
	if _, err := rsvc2.Create(ctx, userA, mat.ID, RequirementInput{SourceGradeID: mkSrc(nota.ID), Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("fuente ajena 400")
	} else {
		mustCode(t, err, "invalid_requirement")
	}
	// Materia ajena → 404.
	if _, err := rsvc.Create(ctx, userB, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("materia ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	// List/Delete + cross-user.
	r1, err := rsvc.Create(ctx, userA, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe})
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := rsvc.List(ctx, userA, mat.ID); len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
	if err := rsvc.Delete(ctx, userB, r1.ID); err == nil {
		t.Fatal("delete ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := rsvc.Delete(ctx, userA, r1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_ = otra
}

// rejectingReqStore niega pertenencia de fuente (simula nota de otra materia).
type rejectingReqStore struct {
	subs map[string]string
}

func (f *rejectingReqStore) Create(_ context.Context, userID, subjectID string, source *string, checks bool, comp string, thr float64, action string) (*model.Requirement, error) {
	return &model.Requirement{ID: uuid.NewString()}, nil
}
func (f *rejectingReqStore) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Requirement, bool, error) {
	return []model.Requirement{}, true, nil
}
func (f *rejectingReqStore) Delete(_ context.Context, id, userID string) (bool, error) {
	return true, nil
}
func (f *rejectingReqStore) GradeSubjectOf(_ context.Context, gradeID, subjectID, userID string) (bool, error) {
	return false, nil
}

// Requisito sobre fuente pendiente no dispara.
func TestRequirement_FuentePendiente_NoDispara(t *testing.T) {
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	ctx := context.Background()
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M")
	mat, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Mat")})

	g := newFakeGrades()
	g.subs[mat.ID] = userA
	gsvc := NewGradeService(g)
	rq := newFakeRequirements(g.subs)
	rsvc := NewRequirementService(rq)
	avgSvc := NewAverageService(avgSubjectStore(sub, map[string]int{mat.ID: 1}), g, rq)

	pend, _ := gsvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("P"), Weight: floatPtr(1)})
	s60 := 60
	if _, err := gsvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("G"), Weight: floatPtr(1), Score: &s60}); err != nil {
		t.Fatal(err)
	}
	thr := 1.0
	ff := "force_fail"
	lt := "<"
	// Fuente pendiente con umbral imposible-igual: no debe disparar aunque
	// el promedio dispare otras cosas; aquí solo hay este requisito.
	if _, err := rsvc.Create(ctx, userA, mat.ID, RequirementInput{SourceGradeID: &pend.ID, Comparator: &lt, ThresholdValue: &thr, Action: &ff}); err != nil {
		t.Fatal(err)
	}
	res, err := avgSvc.Average(ctx, userA, mat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "approved" { // 6.0 >= 4.0, el force_fail pendiente no dispara
		t.Fatalf("fuente pendiente no dispara: %+v", res)
	}
}
