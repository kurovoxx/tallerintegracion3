package http

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// Fakes en memoria con semántica de propiedad (duplicados del service test:
// distinto paquete, misma semántica). Hilo-seguros.

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

func (f *fakeSchedules) Create(_ context.Context, userID, subjectID string, day int, start, end time.Time) (*model.Schedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return nil, nil
	}
	s := &model.Schedule{ID: uuid.NewString(), SubjectID: subjectID, DayOfWeek: day, StartTime: start, EndTime: end}
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

func newServiceFixture() (*fakeCurricula, *fakeSubjects, *fakeSchedules) {
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	sch := newFakeSchedules(sub)
	return cur, sub, sch
}
