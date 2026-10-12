package service

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// Fake de ausencias con propiedad por materia (subjectID -> userID).

type fakeAbsences struct {
	mu    sync.Mutex
	items map[string]*model.Absence
	subs  map[string]string
}

func newFakeAbsences(subs map[string]string) *fakeAbsences {
	return &fakeAbsences{items: map[string]*model.Absence{}, subs: subs}
}

func (f *fakeAbsences) owned(subjectID, userID string) bool {
	u, ok := f.subs[subjectID]
	return ok && u == userID
}

func (f *fakeAbsences) Create(_ context.Context, userID, subjectID string, date time.Time, justified bool, source, typ *string) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.owned(subjectID, userID) {
		return nil, nil
	}
	a := &model.Absence{ID: uuid.NewString(), SubjectID: subjectID, Date: date, Justified: justified, Source: source, Type: typ, CreatedAt: time.Now()}
	f.items[a.ID] = a
	cp := *a
	return &cp, nil
}

func (f *fakeAbsences) ExistsOnDate(_ context.Context, userID, subjectID string, date time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.owned(subjectID, userID) {
		return false, nil
	}
	y1, m1, d1 := date.Date()
	for _, a := range f.items {
		if a.SubjectID != subjectID {
			continue
		}
		if y2, m2, d2 := a.Date.Date(); y1 == y2 && m1 == m2 && d1 == d2 {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAbsences) GetByID(_ context.Context, id, userID string) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.items[id]; ok && f.owned(a.SubjectID, userID) {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeAbsences) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Absence, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.owned(subjectID, userID) {
		return nil, false, nil
	}
	out := []model.Absence{}
	for _, a := range f.items {
		if a.SubjectID == subjectID {
			out = append(out, *a)
		}
	}
	// Orden determinista como el SQL (ORDER BY date ASC, id ASC).
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.Before(out[j].Date)
		}
		return out[i].ID < out[j].ID
	})
	return out, true, nil
}

func (f *fakeAbsences) UpdateJustified(_ context.Context, id, userID string, justified bool) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.items[id]; ok && f.owned(a.SubjectID, userID) {
		a.Justified = justified
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeAbsences) Update(_ context.Context, id, userID string, p repository.AbsencePatch) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.items[id]
	if !ok || !f.owned(a.SubjectID, userID) {
		return nil, nil
	}
	if p.Justified != nil {
		a.Justified = *p.Justified
	}
	if p.ClearSource {
		a.Source = nil
	} else if p.Source != nil {
		a.Source = p.Source
	}
	if p.ClearType {
		a.Type = nil
	} else if p.Type != nil {
		a.Type = p.Type
	}
	cp := *a
	return &cp, nil
}

func (f *fakeAbsences) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.items[id]; ok && f.owned(a.SubjectID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

func (f *fakeAbsences) CountsBySubject(_ context.Context, subjectID, userID string) (int, int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.owned(subjectID, userID) {
		return 0, 0, false, nil
	}
	un, just := 0, 0
	for _, a := range f.items {
		if a.SubjectID != subjectID {
			continue
		}
		if a.Justified {
			just++
		} else {
			un++
		}
	}
	return un, just, true, nil
}

// Seed: materia propia con total y mínimo configurados.
func attendanceFixture(t *testing.T, total *int, min *float64) (*AbsenceService, *AttendanceService, string) {
	t.Helper()
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	ctx := context.Background()
	malla, err := NewCurriculumService(cur).Create(ctx, userA, "M")
	if err != nil {
		t.Fatal(err)
	}
	mat, err := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{
		Name: strPtr("Mat"), TotalClasses: total, MinAttendancePct: min,
	})
	if err != nil {
		t.Fatal(err)
	}
	abs := newFakeAbsences(map[string]string{mat.ID: userA})
	return NewAbsenceService(abs), NewAttendanceService(sub, abs), mat.ID
}

func TestAbsence_CRUD_Duplicado(t *testing.T) {
	svc, _, subID := attendanceFixture(t, intPtr(32), floatPtr(75))
	ctx := context.Background()

	a, err := svc.Create(ctx, userA, subID, "2026-03-10", false, nil, nil)
	if err != nil || a.ID == "" || a.Justified {
		t.Fatalf("create: %v %+v", a, err)
	}
	if _, err := svc.Create(ctx, userA, subID, "2026-03-10", true, nil, nil); err == nil {
		t.Fatal("misma fecha debe ser 409")
	} else {
		mustCode(t, err, "conflict")
	}
	// Otra fecha sí pasa.
	if _, err := svc.Create(ctx, userA, subID, "2026-03-11", true, nil, nil); err != nil {
		t.Fatalf("otra fecha: %v", err)
	}
	for _, bad := range []string{"2026-02-30", "ayer", "10/03/2026", ""} {
		if _, err := svc.Create(ctx, userA, subID, bad, false, nil, nil); err == nil {
			t.Fatalf("fecha %q debe ser 400", bad)
		} else {
			mustCode(t, err, "invalid_date")
		}
	}
	if _, err := svc.Create(ctx, userB, subID, "2026-03-12", false, nil, nil); err == nil {
		t.Fatal("materia ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Create(ctx, userA, "no-uuid", "2026-03-12", false, nil, nil); err == nil {
		t.Fatal("uuid malo 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}

	got, err := svc.Get(ctx, userA, a.ID)
	if err != nil || got.Date.Format("2006-01-02") != "2026-03-10" {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Get(ctx, userB, a.ID); err == nil {
		t.Fatal("get ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	list, err := svc.List(ctx, userA, subID)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: %v %d", err, len(list))
	}

	// Toggle true→false mueve el conteo (se verifica en attendance).
	if _, err := svc.SetJustified(ctx, userA, a.ID, boolPtr(true)); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if _, err := svc.SetJustified(ctx, userA, a.ID, nil); err == nil {
		t.Fatal("justified ausente debe ser 400")
	} else {
		mustCode(t, err, "bad_request")
	}
	if _, err := svc.SetJustified(ctx, userB, a.ID, boolPtr(false)); err == nil {
		t.Fatal("patch ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, userB, a.ID); err == nil {
		t.Fatal("delete ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, userA, a.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestAttendance_FormulaFrente(t *testing.T) {
	// Réplica exacta del ejemplo front: total 32, 5 injust + 2 just
	// → 1-5/32 = 0.84375; restantes min 75 = floor(32*0.25)-5 = 3.
	svc, att, subID := attendanceFixture(t, intPtr(32), floatPtr(75))
	ctx := context.Background()
	for i := 1; i <= 5; i++ {
		if _, err := svc.Create(ctx, userA, subID, sprintfDate(2026, 3, i), false, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := 6; i <= 7; i++ {
		if _, err := svc.Create(ctx, userA, subID, sprintfDate(2026, 3, i), true, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	res, err := att.Summary(ctx, userA, subID)
	if err != nil {
		t.Fatal(err)
	}
	if res.UnjustifiedCount != 5 || res.JustifiedCount != 2 {
		t.Fatalf("conteos: %+v", res)
	}
	if res.Percentage == nil || *res.Percentage != 0.84375 {
		t.Fatalf("percentage 0.84375: %+v", res)
	}
	if res.MeetsMinimum == nil || !*res.MeetsMinimum {
		t.Fatalf("meets true: %+v", res)
	}
	if res.Remaining == nil || *res.Remaining != 3 {
		t.Fatalf("remaining 3: %+v", res)
	}
	// Toggle una injustificada a justificada → 1-4/32 = 0.875, restantes 4.
	// Se elige explícitamente (no list[0]: el orden no debe importar).
	list, _ := svc.List(ctx, userA, subID)
	var target string
	for _, a := range list {
		if !a.Justified {
			target = a.ID
			break
		}
	}
	if target == "" {
		t.Fatal("sin injustificadas para togglear")
	}
	if _, err := svc.SetJustified(ctx, userA, target, boolPtr(true)); err != nil {
		t.Fatal(err)
	}
	res, _ = att.Summary(ctx, userA, subID)
	if *res.Percentage != 0.875 || *res.Remaining != 4 {
		t.Fatalf("tras toggle: %+v", res)
	}
	// Delete restaura.
	if err := svc.Delete(ctx, userA, target); err != nil {
		t.Fatal(err)
	}
	res, _ = att.Summary(ctx, userA, subID)
	if res.UnjustifiedCount != 4 || res.JustifiedCount != 2 {
		t.Fatalf("tras delete: %+v", res)
	}
}

func sprintfDate(y, m, d int) string {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func TestAttendance_Nulos(t *testing.T) {
	ctx := context.Background()

	// total null → percentage/meets/remaining null (conteos sí).
	svc, att, subID := attendanceFixture(t, nil, floatPtr(75))
	if _, err := svc.Create(ctx, userA, subID, "2026-04-01", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	res, err := att.Summary(ctx, userA, subID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Percentage != nil || res.MeetsMinimum != nil || res.Remaining != nil {
		t.Fatalf("total null → nulos: %+v", res)
	}
	if res.UnjustifiedCount != 1 || res.TotalClasses != nil {
		t.Fatalf("conteos presentes: %+v", res)
	}

	// total 0 → igual que null.
	svc0, att0, sub0 := attendanceFixture(t, intPtr(0), floatPtr(75))
	if _, err := svc0.Create(ctx, userA, sub0, "2026-04-01", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if res, _ := att0.Summary(ctx, userA, sub0); res.Percentage != nil || res.Remaining != nil {
		t.Fatalf("total 0 → nulos: %+v", res)
	}

	// min null → meets/remaining null, percentage sí calcula.
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M2")
	mat, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("M2"), TotalClasses: intPtr(20)})
	abs := newFakeAbsences(map[string]string{mat.ID: userA})
	svc2 := NewAbsenceService(abs)
	att2 := NewAttendanceService(sub, abs)
	if _, err := svc2.Create(ctx, userA, mat.ID, "2026-04-01", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	res2, err := att2.Summary(ctx, userA, mat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Percentage == nil || *res2.Percentage != 0.95 {
		t.Fatalf("percentage 0.95: %+v", res2)
	}
	if res2.MeetsMinimum != nil || res2.Remaining != nil {
		t.Fatalf("min null → meets/remaining null: %+v", res2)
	}

	// Todas justificadas → 100% (1.0).
	curJ := newFakeCurricula()
	subJ := newFakeSubjects(curJ)
	mallaJ, _ := NewCurriculumService(curJ).Create(ctx, userA, "MJ")
	matJ, _ := NewSubjectService(subJ).Create(ctx, userA, mallaJ.ID, SubjectInput{Name: strPtr("MJ"), TotalClasses: intPtr(10), MinAttendancePct: floatPtr(75)})
	absJ := newFakeAbsences(map[string]string{matJ.ID: userA})
	svcJ := NewAbsenceService(absJ)
	if _, err := svcJ.Create(ctx, userA, matJ.ID, "2026-04-01", true, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svcJ.Create(ctx, userA, matJ.ID, "2026-04-02", true, nil, nil); err != nil {
		t.Fatal(err)
	}
	resJ, _ := NewAttendanceService(subJ, absJ).Summary(ctx, userA, matJ.ID)
	if resJ.Percentage == nil || *resJ.Percentage != 1.0 || resJ.MeetsMinimum == nil || !*resJ.MeetsMinimum {
		t.Fatalf("todas justificadas 1.0/meets: %+v", resJ)
	}

	// Cross-user + uuid malo en summary.
	if _, err := att2.Summary(ctx, userB, mat.ID); err == nil {
		t.Fatal("summary ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := att2.Summary(ctx, userA, "no-uuid"); err == nil {
		t.Fatal("uuid malo 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}
