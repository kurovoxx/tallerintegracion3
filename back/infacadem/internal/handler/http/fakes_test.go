package http

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// CountsByCurricula agrega conteos de materias propias (una pasada, como el
// GROUP BY real) para satisfacer service.SubjectCounter en tests.
func (f *fakeSubjects) CountsByCurricula(_ context.Context, userID string) (map[string]repository.SubjectCounts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cur.mu.Lock()
	defer f.cur.mu.Unlock()
	out := make(map[string]repository.SubjectCounts)
	for _, s := range f.items {
		c, ok := f.cur.items[s.CurriculumID]
		if !ok || c.UserID != userID {
			continue
		}
		agg := out[s.CurriculumID]
		agg.Total++
		if s.Status == model.SubjectStatusApproved {
			agg.Approved++
		}
		if s.Status == model.SubjectStatusInProgress {
			agg.InProgress++
		}
		out[s.CurriculumID] = agg
	}
	return out, nil
}

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

func newServiceFixture() (*fakeCurricula, *fakeSubjects, *fakeSchedules) {
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	sch := newFakeSchedules(sub)
	return cur, sub, sch
}

type fakeGrades struct {
	mu    sync.Mutex
	items map[string]*model.Grade
	sub   *fakeSubjects
}

func newFakeGrades(sub *fakeSubjects) *fakeGrades {
	return &fakeGrades{items: map[string]*model.Grade{}, sub: sub}
}

func (f *fakeGrades) ownedSubjectLocked(subjectID, userID string) bool {
	f.sub.cur.mu.Lock()
	defer f.sub.cur.mu.Unlock()
	for _, s := range f.sub.items {
		if s.ID == subjectID {
			c, ok := f.sub.cur.items[s.CurriculumID]
			return ok && c.UserID == userID
		}
	}
	// La materia puede existir sin subjects creados: buscar en curricula.
	for _, c := range f.sub.cur.items {
		if c.ID == subjectID {
			return c.UserID == userID
		}
	}
	return false
}

func (f *fakeGrades) Create(_ context.Context, userID, subjectID string, in repository.GradeCreate) (*model.Grade, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if g, ok := f.items[id]; ok && f.ownedSubjectLocked(g.SubjectID, userID) {
		cp := *g
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeGrades) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Grade, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	g, ok := f.items[id]
	if !ok || !f.ownedSubjectLocked(g.SubjectID, userID) {
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
	if p.Pinned != nil {
		g.Pinned = *p.Pinned
		if *p.Pinned {
			for _, o := range f.items {
				if o.ID != g.ID && o.SubjectID == g.SubjectID {
					o.Pinned = false
				}
			}
		}
	}
	cp := *g
	return &cp, nil
}

func (f *fakeGrades) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if g, ok := f.items[id]; ok && f.ownedSubjectLocked(g.SubjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return 0, nil
	}
	n := 0
	for _, g := range f.items {
		if g.SubjectID == subjectID && g.IsFinalExam && g.ID != excludeID {
			n++
		}
	}
	return n, nil
}

type fakeRequirements struct {
	mu    sync.Mutex
	items map[string]*model.Requirement
	sub   *fakeSubjects
	grd   *fakeGrades
}

func newFakeRequirements(sub *fakeSubjects, grd *fakeGrades) *fakeRequirements {
	return &fakeRequirements{items: map[string]*model.Requirement{}, sub: sub, grd: grd}
}

func (f *fakeRequirements) ownedSubjectLocked(subjectID, userID string) bool {
	f.sub.cur.mu.Lock()
	defer f.sub.cur.mu.Unlock()
	for _, s := range f.sub.items {
		if s.ID == subjectID {
			c, ok := f.sub.cur.items[s.CurriculumID]
			return ok && c.UserID == userID
		}
	}
	for _, c := range f.sub.cur.items {
		if c.ID == subjectID {
			return c.UserID == userID
		}
	}
	return false
}

func (f *fakeRequirements) Create(_ context.Context, userID, subjectID string, source *string, checks bool, comp string, thr float64, action string) (*model.Requirement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if r, ok := f.items[id]; ok && f.ownedSubjectLocked(r.SubjectID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

func (f *fakeRequirements) GradeSubjectOf(_ context.Context, gradeID, subjectID, userID string) (bool, error) {
	f.grd.mu.Lock()
	defer f.grd.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	g, ok := f.grd.items[gradeID]
	if !ok || g.SubjectID != subjectID {
		return false, nil
	}
	return f.ownedSubjectLocked(subjectID, userID), nil
}

type fakeAbsences struct {
	mu    sync.Mutex
	items map[string]*model.Absence
	sub   *fakeSubjects
}

func newFakeAbsences(sub *fakeSubjects) *fakeAbsences {
	return &fakeAbsences{items: map[string]*model.Absence{}, sub: sub}
}

func (f *fakeAbsences) ownedSubjectLocked(subjectID, userID string) bool {
	f.sub.cur.mu.Lock()
	defer f.sub.cur.mu.Unlock()
	for _, s := range f.sub.items {
		if s.ID == subjectID {
			c, ok := f.sub.cur.items[s.CurriculumID]
			return ok && c.UserID == userID
		}
	}
	return false
}

func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func (f *fakeAbsences) Create(_ context.Context, userID, subjectID string, date time.Time, justified bool, source, typ *string) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return false, nil
	}
	for _, a := range f.items {
		if a.SubjectID == subjectID && sameDay(a.Date, date) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeAbsences) GetByID(_ context.Context, id, userID string) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if a, ok := f.items[id]; ok && f.ownedSubjectLocked(a.SubjectID, userID) {
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeAbsences) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Absence, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if a, ok := f.items[id]; ok && f.ownedSubjectLocked(a.SubjectID, userID) {
		a.Justified = justified
		cp := *a
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeAbsences) Update(_ context.Context, id, userID string, p repository.AbsencePatch) (*model.Absence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	a, ok := f.items[id]
	if !ok || !f.ownedSubjectLocked(a.SubjectID, userID) {
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
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if a, ok := f.items[id]; ok && f.ownedSubjectLocked(a.SubjectID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

func (f *fakeAbsences) CountsBySubject(_ context.Context, subjectID, userID string) (int, int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
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

type fakeContents struct {
	mu    sync.Mutex
	items map[string]*model.Content
	sub   *fakeSubjects
}

func newFakeContents(sub *fakeSubjects) *fakeContents {
	return &fakeContents{items: map[string]*model.Content{}, sub: sub}
}

func (f *fakeContents) ownedSubjectLocked(subjectID, userID string) bool {
	f.sub.cur.mu.Lock()
	defer f.sub.cur.mu.Unlock()
	for _, s := range f.sub.items {
		if s.ID == subjectID {
			c, ok := f.sub.cur.items[s.CurriculumID]
			return ok && c.UserID == userID
		}
	}
	return false
}

func (f *fakeContents) Create(_ context.Context, userID, subjectID, title string, body *string, position int) (*model.Content, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return nil, nil
	}
	c := &model.Content{ID: uuid.NewString(), SubjectID: subjectID, Title: title, Body: body, Position: position}
	f.items[c.ID] = c
	cp := *c
	return &cp, nil
}

func (f *fakeContents) GetByID(_ context.Context, id, userID string) (*model.Content, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if c, ok := f.items[id]; ok && f.ownedSubjectLocked(c.SubjectID, userID) {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeContents) ListBySubject(_ context.Context, subjectID, userID string) ([]model.Content, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if !f.ownedSubjectLocked(subjectID, userID) {
		return nil, false, nil
	}
	out := []model.Content{}
	for _, c := range f.items {
		if c.SubjectID == subjectID {
			out = append(out, *c)
		}
	}
	return out, true, nil
}

func (f *fakeContents) Update(_ context.Context, id, userID string, p repository.ContentPatch) (*model.Content, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	c, ok := f.items[id]
	if !ok || !f.ownedSubjectLocked(c.SubjectID, userID) {
		return nil, nil
	}
	if p.Title != nil {
		c.Title = *p.Title
	}
	if p.ClearBody {
		c.Body = nil
	} else if p.Body != nil {
		c.Body = p.Body
	}
	if p.Position != nil {
		c.Position = *p.Position
	}
	if p.Done != nil {
		c.Done = *p.Done
	}
	cp := *c
	return &cp, nil
}

func (f *fakeContents) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if c, ok := f.items[id]; ok && f.ownedSubjectLocked(c.SubjectID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}
