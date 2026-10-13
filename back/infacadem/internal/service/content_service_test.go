package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

type fakeContents struct {
	mu    sync.Mutex
	items map[string]*model.Content
	sub   *fakeSubjects
}

func newFakeContents(sub *fakeSubjects) *fakeContents {
	return &fakeContents{items: map[string]*model.Content{}, sub: sub}
}

func (f *fakeContents) ownedSubject(subjectID, userID string) bool {
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
	if !f.ownedSubject(subjectID, userID) {
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
	if c, ok := f.items[id]; ok && f.ownedSubject(c.SubjectID, userID) {
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
	if !f.ownedSubject(subjectID, userID) {
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
	if !ok || !f.ownedSubject(c.SubjectID, userID) {
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
	cp := *c
	return &cp, nil
}

func (f *fakeContents) Delete(_ context.Context, id, userID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sub.mu.Lock()
	defer f.sub.mu.Unlock()
	if c, ok := f.items[id]; ok && f.ownedSubject(c.SubjectID, userID) {
		delete(f.items, id)
		return true, nil
	}
	return false, nil
}

func TestContent_CRUD_Validaciones(t *testing.T) {
	ctx := context.Background()
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	svc := NewContentService(newFakeContents(sub))
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M")
	mat, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("S")})

	created, err := svc.Create(ctx, userA, mat.ID, ContentInput{
		Title: strPtr("Unidad 1"), Body: strPtr("  Cuerpo "), Position: intPtr(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Body == nil || *created.Body != "Cuerpo" || created.Position != 2 {
		t.Fatalf("create: %+v", created)
	}
	if _, err := svc.Create(ctx, userA, mat.ID, ContentInput{}); err == nil {
		t.Fatal("sin título 400")
	} else {
		mustCode(t, err, "invalid_name")
	}
	if _, err := svc.Create(ctx, userB, mat.ID, ContentInput{Title: strPtr("X")}); err == nil {
		t.Fatal("ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Create(ctx, "", mat.ID, ContentInput{Title: strPtr("X")}); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.Create(ctx, userA, "x", ContentInput{Title: strPtr("X")}); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}

	upd, err := svc.Update(ctx, userA, created.ID, ContentInput{Title: strPtr("U1b"), Body: strPtr("")})
	if err != nil || upd.Title != "U1b" || upd.Body != nil {
		t.Fatalf("patch set/limpia: %+v %v", upd, err)
	}
	if _, err := svc.Update(ctx, userA, created.ID, ContentInput{}); err == nil {
		t.Fatal("vacío 400")
	} else {
		mustCode(t, err, "bad_request")
	}
	if _, err := svc.Update(ctx, "", created.ID, ContentInput{Title: strPtr("X")}); err == nil {
		t.Fatal("update sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.Update(ctx, userA, "x", ContentInput{Title: strPtr("X")}); err == nil {
		t.Fatal("update uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svc.Update(ctx, userB, created.ID, ContentInput{Title: strPtr("X")}); err == nil {
		t.Fatal("update ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, "", created.ID); err == nil {
		t.Fatal("delete sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if err := svc.Delete(ctx, userA, "x"); err == nil {
		t.Fatal("delete uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if err := svc.Delete(ctx, userB, created.ID); err == nil {
		t.Fatal("delete ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Get(ctx, "", created.ID); err == nil {
		t.Fatal("get sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.Get(ctx, userA, "x"); err == nil {
		t.Fatal("get uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svc.Get(ctx, userB, created.ID); err == nil {
		t.Fatal("get ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	list, err := svc.List(ctx, userA, mat.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if _, err := svc.List(ctx, "", mat.ID); err == nil {
		t.Fatal("list sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.List(ctx, userA, "x"); err == nil {
		t.Fatal("list uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svc.List(ctx, userB, mat.ID); err == nil {
		t.Fatal("list ajena 404")
	} else {
		mustCode(t, err, "not_found")
	}
	if err := svc.Delete(ctx, userA, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, userA, created.ID); err == nil {
		t.Fatal("tras delete 404")
	} else {
		mustCode(t, err, "not_found")
	}
}

func TestGrade_Pinned_Update(t *testing.T) {
	ctx := context.Background()
	g := newFakeGrades()
	subID := newUUID()
	g.subs[subID] = userA
	svc := NewGradeService(g)
	pin := true

	a, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("A"), Weight: floatPtr(1)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("B"), Weight: floatPtr(1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, userA, a.ID, GradeInput{Pinned: &pin}); err != nil {
		t.Fatal(err)
	}
	updB, err := svc.Update(ctx, userA, b.ID, GradeInput{Pinned: &pin})
	if err != nil || !updB.Pinned {
		t.Fatalf("pin B: %+v %v", updB, err)
	}
	gotA, _ := svc.Get(ctx, userA, a.ID)
	if gotA.Pinned {
		t.Fatal("A despinneada por B")
	}
	// Solo-pin no es "sin campos".
	if _, err := svc.Update(ctx, userA, a.ID, GradeInput{}); err == nil {
		t.Fatal("vacío 400")
	} else {
		mustCode(t, err, "bad_request")
	}
}
