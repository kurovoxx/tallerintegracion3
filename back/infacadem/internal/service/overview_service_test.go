package service

import (
	"context"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// List adapta el fake 3_1_2 a CurriculumLister (mismo orden del store).
func (f *fakeCurricula) List(ctx context.Context, userID string) ([]model.Curriculum, error) {
	return f.ListByUser(ctx, userID)
}

// Fake de conteo agregado (una sola respuesta, como el GROUP BY real).

type fakeCounter struct {
	counts map[string]repository.SubjectCounts
	err    error
}

func (f *fakeCounter) CountsByCurricula(_ context.Context, _ string) (map[string]repository.SubjectCounts, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]repository.SubjectCounts, len(f.counts))
	for k, v := range f.counts {
		out[k] = v
	}
	return out, nil
}

func TestOverview_Vacia(t *testing.T) {
	svc := NewOverviewService(newFakeCurricula(), &fakeCounter{counts: map[string]repository.SubjectCounts{}})
	out, err := svc.Get(context.Background(), userA)
	if err != nil {
		t.Fatal(err)
	}
	if out.Curricula == nil || len(out.Curricula) != 0 {
		t.Fatalf("curricula []: %+v", out)
	}
	if out.Stats.CurriculaCount != 0 || out.Stats.SubjectsCount != 0 {
		t.Fatalf("stats cero: %+v", out)
	}
	if _, err := svc.Get(context.Background(), "  "); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
}

func TestOverview_Conteos(t *testing.T) {
	cur := newFakeCurricula()
	ctx := context.Background()
	curSvc := NewCurriculumService(cur)
	m1, _ := curSvc.Create(ctx, userA, "M1")
	m2, _ := curSvc.Create(ctx, userA, "M2")
	m3, _ := curSvc.Create(ctx, userA, "M3") // sin materias
	counter := &fakeCounter{counts: map[string]repository.SubjectCounts{
		m1.ID: {Total: 3, Approved: 1, InProgress: 2},
		m2.ID: {Total: 1, Approved: 0, InProgress: 0},
	}}
	svc := NewOverviewService(cur, counter)
	out, err := svc.Get(ctx, userA)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Curricula) != 3 {
		t.Fatalf("3 mallas: %+v", out)
	}
	byID := map[string]CurriculumOverview{}
	for _, c := range out.Curricula {
		byID[c.ID] = c
	}
	if byID[m1.ID].ApprovedCount != 1 || byID[m1.ID].InProgressCount != 2 || byID[m1.ID].SubjectsCount != 3 {
		t.Fatalf("m1: %+v", byID[m1.ID])
	}
	if byID[m3.ID].SubjectsCount != 0 {
		t.Fatalf("m3 ceros: %+v", byID[m3.ID])
	}
	if out.Stats.CurriculaCount != 3 || out.Stats.SubjectsCount != 4 {
		t.Fatalf("stats: %+v", out.Stats)
	}
	// Solo propias: userB no ve nada.
	outB, err := svc.Get(ctx, userB)
	if err != nil {
		t.Fatal(err)
	}
	if len(outB.Curricula) != 0 || outB.Stats.SubjectsCount != 0 {
		t.Fatalf("ajenas invisibles: %+v", outB)
	}
}

func TestDetail_Vacia(t *testing.T) {
	cStore := newFakeCurricula()
	sStore := newFakeSubjects(cStore)
	ctx := context.Background()
	malla, _ := NewCurriculumService(cStore).Create(ctx, userA, "M")
	mat, _ := NewSubjectService(sStore).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Mat")})

	grd := newFakeGrades()
	grd.subs[mat.ID] = userA
	rq := newFakeRequirements(grd.subs)
	attAbs := newFakeAbsences(map[string]string{mat.ID: userA})

	d := NewDetailService(
		&detailSubject{sStore: sStore},
		&detailSchedules{},
		&detailGrades{},
		NewAverageService(avgSubjectStore(sStore, map[string]int{mat.ID: 2}), grd, rq),
		NewAttendanceService(sStore, attAbs),
	)
	res, err := d.Get(ctx, userA, mat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Schedules == nil || len(res.Schedules) != 0 {
		t.Fatalf("schedules []: %+v", res)
	}
	if res.Grades == nil || len(res.Grades) != 0 {
		t.Fatalf("grades []: %+v", res)
	}
	if res.Average == nil || res.Average.Average != nil || res.Average.Status != "pending" {
		t.Fatalf("average null/pending: %+v", res.Average)
	}
	if res.Attendance == nil || res.Attendance.Percentage != nil || res.Attendance.MeetsMinimum != nil || res.Attendance.Remaining != nil {
		t.Fatalf("attendance nulls: %+v", res.Attendance)
	}
	if res.Subject.ID != mat.ID {
		t.Fatalf("subject: %+v", res.Subject)
	}
	// Ajena → 404, uuid malo → 400.
	if _, err := d.Get(ctx, userB, mat.ID); err == nil {
		t.Fatal("ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := d.Get(ctx, userA, "no-uuid"); err == nil {
		t.Fatal("uuid malo 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}

// Adapters mínimos sobre mapas para el detail vacío.

type detailSubject struct{ sStore *fakeSubjects }

func (a *detailSubject) Get(ctx context.Context, userID, id string) (*model.Subject, error) {
	return NewSubjectService(a.sStore).Get(ctx, userID, id)
}

type detailSchedules struct{}

func (a *detailSchedules) List(_ context.Context, _, _ string) ([]model.Schedule, error) {
	return []model.Schedule{}, nil
}

type detailGrades struct{}

func (a *detailGrades) List(_ context.Context, _, _ string) ([]GradeNode, error) {
	return []GradeNode{}, nil
}

func TestDetail_Completa_Decimales(t *testing.T) {
	cStore := newFakeCurricula()
	sStore := newFakeSubjects(cStore)
	ctx := context.Background()
	malla, _ := NewCurriculumService(cStore).Create(ctx, userA, "M")
	// decimals=1 para verificar redondeo en el agregado.
	mat, _ := NewSubjectService(sStore).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Mat")})

	grd := newFakeGrades()
	grd.subs[mat.ID] = userA
	gsvc := NewGradeService(grd)
	s70, s40 := 70, 40
	if _, err := gsvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("A"), Weight: floatPtr(0.5), Score: &s70}); err != nil {
		t.Fatal(err)
	}
	if _, err := gsvc.Create(ctx, userA, mat.ID, GradeInput{Name: strPtr("B"), Weight: floatPtr(0.5), Score: &s40}); err != nil {
		t.Fatal(err)
	}
	rq := newFakeRequirements(grd.subs)
	abs := newFakeAbsences(map[string]string{mat.ID: userA})
	asvc := NewAbsenceService(abs)
	if _, err := asvc.Create(ctx, userA, mat.ID, "2026-03-01", false); err != nil {
		t.Fatal(err)
	}

	// total 2 para que el attendance calcule: PATCH vía repo fake directo.
	sStore.mu.Lock()
	if s, ok := sStore.items[mat.ID]; ok {
		s.TotalClasses = intPtr(2)
		m := 50.0
		s.MinAttendancePct = &m
	}
	sStore.mu.Unlock()

	d := NewDetailService(
		&detailSubject{sStore: sStore},
		&detailSchedulesReal{subs: sStore},
		&gsvcList{gsvc: gsvc},
		NewAverageService(avgSubjectStore(sStore, map[string]int{mat.ID: 1}), grd, rq),
		NewAttendanceService(sStore, abs),
	)
	res, err := d.Get(ctx, userA, mat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Grades) != 2 {
		t.Fatalf("2 notas: %+v", res.Grades)
	}
	if res.Average == nil || res.Average.Average == nil || *res.Average.Average != 5.5 {
		t.Fatalf("average 5.5 (1 decimal): %+v", res.Average)
	}
	if res.Attendance == nil || res.Attendance.Percentage == nil || *res.Attendance.Percentage != 0.5 {
		t.Fatalf("attendance 0.5: %+v", res.Attendance)
	}
	if res.Attendance.MeetsMinimum == nil || !*res.Attendance.MeetsMinimum {
		t.Fatalf("meets (0.5>=0.5): %+v", res.Attendance)
	}
	if len(res.Schedules) != 0 {
		t.Fatalf("sin horarios: %+v", res.Schedules)
	}
}

type detailSchedulesReal struct{ subs *fakeSubjects }

func (a *detailSchedulesReal) List(_ context.Context, _, _ string) ([]model.Schedule, error) {
	return []model.Schedule{}, nil
}

type gsvcList struct{ gsvc *GradeService }

func (a *gsvcList) List(ctx context.Context, userID, subjectID string) ([]GradeNode, error) {
	return a.gsvc.List(ctx, userID, subjectID)
}
