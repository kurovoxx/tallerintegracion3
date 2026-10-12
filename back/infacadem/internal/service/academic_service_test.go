package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// Fakes en memoria con semántica de propiedad (incluye cascada en borrado
// de malla, como la FK ON DELETE CASCADE).

type fakeCurricula struct {
	mu    sync.Mutex
	items map[string]*model.Curriculum
}

func newFakeCurricula() *fakeCurricula { return &fakeCurricula{items: map[string]*model.Curriculum{}} }

func (f *fakeCurricula) Create(_ context.Context, userID, name string) (*model.Curriculum, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &model.Curriculum{ID: uuid.NewString(), UserID: userID, Name: name, CreatedAt: time.Now()}
	f.items[c.ID] = c
	cp := *c
	return &cp, nil
}

func (f *fakeCurricula) GetByID(_ context.Context, id, userID string) (*model.Curriculum, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.items[id]; ok && c.UserID == userID {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeCurricula) ListByUser(_ context.Context, userID string) ([]model.Curriculum, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.Curriculum{}
	for _, c := range f.items {
		if c.UserID == userID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeCurricula) UpdateName(_ context.Context, id, userID, name string) (*model.Curriculum, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.items[id]; ok && c.UserID == userID {
		c.Name = name
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeCurricula) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.items[id]; ok && c.UserID == userID {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

type fakeSubjects struct {
	mu    sync.Mutex
	items map[string]*model.Subject
	cur   *fakeCurricula
}

func newFakeSubjects(cur *fakeCurricula) *fakeSubjects {
	return &fakeSubjects{items: map[string]*model.Subject{}, cur: cur}
}

func (f *fakeSubjects) ownedLocked(curriculumID, userID string) bool {
	c, ok := f.cur.items[curriculumID]
	return ok && c.UserID == userID
}

func (f *fakeSubjects) Create(_ context.Context, userID, curriculumID string, in repository.SubjectCreate) (*model.Subject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.mu.Lock()
	defer f.cur.mu.Unlock()
	if !f.ownedLocked(curriculumID, userID) {
		return nil, nil
	}
	s := &model.Subject{ID: uuid.NewString(), CurriculumID: curriculumID, Name: in.Name,
		Code: in.Code, Professor: in.Professor, Room: in.Room,
		Semester: in.Semester, Credits: in.Credits, TotalClasses: in.TotalClasses,
		MinAttendancePct: in.MinAttendancePct, AverageDecimalPlaces: in.AverageDecimalPlaces, Status: in.Status}
	f.items[s.ID] = s
	cp := *s
	return &cp, nil
}

func (f *fakeSubjects) GetByID(_ context.Context, id, userID string) (*model.Subject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.mu.Lock()
	defer f.cur.mu.Unlock()
	if s, ok := f.items[id]; ok && f.ownedLocked(s.CurriculumID, userID) {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeSubjects) ListByCurriculum(_ context.Context, curriculumID, userID string) ([]model.Subject, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.mu.Lock()
	defer f.cur.mu.Unlock()
	if !f.ownedLocked(curriculumID, userID) {
		return nil, false, nil
	}
	out := []model.Subject{}
	for _, s := range f.items {
		if s.CurriculumID == curriculumID {
			out = append(out, *s)
		}
	}
	return out, true, nil
}

func (f *fakeSubjects) Update(_ context.Context, id, userID string, p repository.SubjectPatch) (*model.Subject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.mu.Lock()
	defer f.cur.mu.Unlock()
	s, ok := f.items[id]
	if !ok || !f.ownedLocked(s.CurriculumID, userID) {
		return nil, nil
	}
	if p.Name != nil {
		s.Name = *p.Name
	}
	if p.ClearCode {
		s.Code = nil
	} else if p.Code != nil {
		s.Code = p.Code
	}
	if p.ClearProfessor {
		s.Professor = nil
	} else if p.Professor != nil {
		s.Professor = p.Professor
	}
	if p.ClearRoom {
		s.Room = nil
	} else if p.Room != nil {
		s.Room = p.Room
	}
	if p.Semester != nil {
		s.Semester = p.Semester
	}
	if p.Credits != nil {
		s.Credits = p.Credits
	}
	if p.TotalClasses != nil {
		s.TotalClasses = p.TotalClasses
	}
	if p.MinAttendancePct != nil {
		s.MinAttendancePct = p.MinAttendancePct
	}
	if p.AverageDecimalPlaces != nil {
		s.AverageDecimalPlaces = *p.AverageDecimalPlaces
	}
	if p.Status != nil {
		s.Status = *p.Status
	}
	cp := *s
	return &cp, nil
}

func (f *fakeSubjects) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.mu.Lock()
	defer f.cur.mu.Unlock()
	if s, ok := f.items[id]; ok && f.ownedLocked(s.CurriculumID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

type fakeSchedules struct {
	mu    sync.Mutex
	items map[string]*model.Schedule
	sub   *fakeSubjects
}

func newFakeSchedules(sub *fakeSubjects) *fakeSchedules {
	return &fakeSchedules{items: map[string]*model.Schedule{}, sub: sub}
}

func (f *fakeSchedules) ownedSubjectLocked(subjectID, userID string) bool {
	f.sub.cur.mu.Lock()
	defer f.sub.cur.mu.Unlock()
	s, ok := f.sub.items[subjectID]
	if !ok {
		return false
	}
	c, ok := f.sub.cur.items[s.CurriculumID]
	return ok && c.UserID == userID
}

func (f *fakeSchedules) Create(_ context.Context, userID, subjectID string, day int, start, end time.Time, modality *string) (*model.Schedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return nil, nil
	}
	s := &model.Schedule{ID: uuid.NewString(), SubjectID: subjectID, DayOfWeek: day, StartTime: start, EndTime: end, Modality: modality}
	f.items[s.ID] = s
	cp := *s
	return &cp, nil
}

func (f *fakeSchedules) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Schedule, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return nil, false, nil
	}
	out := []model.Schedule{}
	for _, s := range f.items {
		if s.SubjectID == subjectID {
			out = append(out, *s)
		}
	}
	return out, true, nil
}

func (f *fakeSchedules) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if s, ok := f.items[id]; ok && f.ownedSubjectLocked(s.SubjectID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

func newFixture() (*CurriculumService, *SubjectService, *ScheduleService, *fakeCurricula, *fakeSubjects, *fakeSchedules) {
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	sch := newFakeSchedules(sub)
	return NewCurriculumService(cur), NewSubjectService(sub), NewScheduleService(sch), cur, sub, sch
}

const (
	userA = "11111111-1111-4111-8111-111111111111"
	userB = "22222222-2222-4222-8222-222222222222"
)

func mustCode(t *testing.T, err error, want string) {
	t.Helper()
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("esperado *ServiceError, got %T (%v)", err, err)
	}
	if se.Code != want {
		t.Fatalf("code esperado %q, got %q", want, se.Code)
	}
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestCurriculum_CRUD(t *testing.T) {
	svc, _, _, _, _, _ := newFixture()
	ctx := context.Background()

	c, err := svc.Create(ctx, userA, "Malla 2026")
	if err != nil || c.ID == "" {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Create(ctx, userA, "  "); err == nil {
		t.Fatal("nombre vacío debe ser 400")
	} else {
		mustCode(t, err, "invalid_name")
	}
	if _, err := svc.Create(ctx, userA, string(make([]byte, 201))); err == nil {
		t.Fatal("nombre >200 debe ser 400")
	} else {
		mustCode(t, err, "invalid_name")
	}

	got, err := svc.Get(ctx, userA, c.ID)
	if err != nil || got.Name != "Malla 2026" {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Get(ctx, userB, c.ID); err == nil {
		t.Fatal("recurso ajeno debe ser 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Get(ctx, userA, "no-uuid"); err == nil {
		t.Fatal("uuid malformado debe ser 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}

	list, err := svc.List(ctx, userA)
	if err != nil || len(list) != 1 {
		t.Fatalf("list propias: %v %d", err, len(list))
	}
	if listB, _ := svc.List(ctx, userB); len(listB) != 0 {
		t.Fatal("list ajena debe ser vacía")
	}

	upd, err := svc.Update(ctx, userA, c.ID, "Malla 2027")
	if err != nil || upd.Name != "Malla 2027" {
		t.Fatalf("update: %v", err)
	}
	if _, err := svc.Update(ctx, userB, c.ID, "Hack"); err == nil {
		t.Fatal("update ajeno debe ser 404")
	} else {
		mustCode(t, err, "not_found")
	}

	if err := svc.Delete(ctx, userB, c.ID); err == nil {
		t.Fatal("delete ajeno debe ser 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, userA, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, userA, c.ID); err == nil {
		t.Fatal("tras delete debe ser 404")
	} else {
		mustCode(t, err, "not_found")
	}
}

func TestSubject_CRUD_Validaciones(t *testing.T) {
	_, svc, _, cur, _, _ := newFixture()
	ctx := context.Background()
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "Malla")

	sub, err := svc.Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Cálculo")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sub.Status != "in_progress" || sub.AverageDecimalPlaces != 2 {
		t.Fatalf("defaults DDL esperados, got %+v", sub)
	}
	if _, err := svc.Create(ctx, userB, malla.ID, SubjectInput{Name: strPtr("X")}); err == nil {
		t.Fatal("malla ajena debe ser 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Create(ctx, userA, malla.ID, SubjectInput{}); err == nil {
		t.Fatal("sin name debe ser 400")
	} else {
		mustCode(t, err, "invalid_name")
	}
	if _, err := svc.Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("X"), AverageDecimalPlaces: intPtr(3)}); err == nil {
		t.Fatal("decimal 3 debe ser 400")
	} else {
		mustCode(t, err, "invalid_decimal_places")
	}
	if _, err := svc.Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("X"), Status: strPtr("inventado")}); err == nil {
		t.Fatal("status inventado debe ser 400")
	} else {
		mustCode(t, err, "invalid_status")
	}

	if _, err := svc.Get(ctx, userB, sub.ID); err == nil {
		t.Fatal("get ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if list, _ := svc.List(ctx, userA, malla.ID); len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
	if _, err := svc.List(ctx, userB, malla.ID); err == nil {
		t.Fatal("list malla ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}

	upd, err := svc.Update(ctx, userA, sub.ID, SubjectInput{Status: strPtr("approved")})
	if err != nil || upd.Status != "approved" {
		t.Fatalf("patch status: %v", err)
	}
	if _, err := svc.Update(ctx, userA, sub.ID, SubjectInput{}); err == nil {
		t.Fatal("patch vacío debe ser 400")
	} else {
		mustCode(t, err, "bad_request")
	}
	if _, err := svc.Update(ctx, userB, sub.ID, SubjectInput{Status: strPtr("failed")}); err == nil {
		t.Fatal("patch ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, userB, sub.ID); err == nil {
		t.Fatal("delete ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, userA, sub.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestSchedule_CRUD_Validaciones(t *testing.T) {
	_, subSvc, schSvc, cur, _, _ := newFixture()
	ctx := context.Background()
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "Malla")
	mat, _ := subSvc.Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("Física")})

	s, err := schSvc.Create(ctx, userA, mat.ID, 1, "08:00", "09:30", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s.StartTime.Format("15:04") != "08:00" || s.EndTime.Format("15:04") != "09:30" {
		t.Fatalf("horas: %+v", s)
	}
	for _, day := range []int{-1, 7} {
		if _, err := schSvc.Create(ctx, userA, mat.ID, day, "08:00", "09:00", nil); err == nil {
			t.Fatalf("day %d debe ser 400", day)
		} else {
			mustCode(t, err, "invalid_day_of_week")
		}
	}
	if _, err := schSvc.Create(ctx, userA, mat.ID, 1, "09:00", "09:00", nil); err == nil {
		t.Fatal("end==start debe ser 400")
	} else {
		mustCode(t, err, "invalid_time_range")
	}
	if _, err := schSvc.Create(ctx, userA, mat.ID, 1, "10:00", "09:00", nil); err == nil {
		t.Fatal("end<start debe ser 400")
	} else {
		mustCode(t, err, "invalid_time_range")
	}
	if _, err := schSvc.Create(ctx, userA, mat.ID, 1, "mañana", "09:00", nil); err == nil {
		t.Fatal("formato malo debe ser 400")
	} else {
		mustCode(t, err, "invalid_time_format")
	}
	if _, err := schSvc.Create(ctx, userB, mat.ID, 1, "08:00", "09:00", nil); err == nil {
		t.Fatal("materia ajena debe ser 404")
	} else {
		mustCode(t, err, "not_found")
	}

	if list, _ := schSvc.List(ctx, userA, mat.ID); len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
	if _, err := schSvc.List(ctx, userB, mat.ID); err == nil {
		t.Fatal("list ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := schSvc.Delete(ctx, userB, s.ID); err == nil {
		t.Fatal("delete ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := schSvc.Delete(ctx, userA, s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestCurriculum_ConcurrentCreate(t *testing.T) {
	svc, _, _, _, _, _ := newFixture()
	ctx := context.Background()
	const n = 2
	out := make([]*model.Curriculum, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out[i], errs[i] = svc.Create(ctx, userA, fmt.Sprintf("Malla %d", i))
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("create concurrente: %v", err)
		}
	}
	if out[0].ID == out[1].ID {
		t.Fatal("doble POST debe crear 2 recursos distintos")
	}
}
