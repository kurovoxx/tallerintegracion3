package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

// Cobertura de ramas de error y funciones sin cubrir: fakes que propagan
// errores genéricos (no ServiceError) + llamadas directas por función.

var errBoom = errors.New("boom")

// --- Fakes con error inyectado (embeben los fakes felices) ---

type errCurricula struct {
	*fakeCurricula
	listErr   error
	getErr    error
	updateErr error
	deleteErr error
	createErr error
}

func (f *errCurricula) ListByUser(ctx context.Context, userID string) ([]model.Curriculum, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.fakeCurricula.ListByUser(ctx, userID)
}

func (f *errCurricula) GetByID(ctx context.Context, id, userID string) (*model.Curriculum, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.fakeCurricula.GetByID(ctx, id, userID)
}

func (f *errCurricula) UpdateName(ctx context.Context, id, userID, name string) (*model.Curriculum, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.fakeCurricula.UpdateName(ctx, id, userID, name)
}

func (f *errCurricula) Delete(ctx context.Context, id, userID string) (bool, error) {
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	return f.fakeCurricula.Delete(ctx, id, userID)
}

func (f *errCurricula) Create(ctx context.Context, userID, name string) (*model.Curriculum, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.fakeCurricula.Create(ctx, userID, name)
}

type errSubjects struct {
	*fakeSubjects
	err error
}

func (f *errSubjects) Create(ctx context.Context, u, c string, in repository.SubjectCreate) (*model.Subject, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeSubjects.Create(ctx, u, c, in)
}

func (f *errSubjects) GetByID(ctx context.Context, id, u string) (*model.Subject, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeSubjects.GetByID(ctx, id, u)
}

func (f *errSubjects) ListByCurriculum(ctx context.Context, c, u string) ([]model.Subject, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	return f.fakeSubjects.ListByCurriculum(ctx, c, u)
}

func (f *errSubjects) Update(ctx context.Context, id, u string, p repository.SubjectPatch) (*model.Subject, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeSubjects.Update(ctx, id, u, p)
}

func (f *errSubjects) Delete(ctx context.Context, id, u string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.fakeSubjects.Delete(ctx, id, u)
}

type errSchedules struct {
	*fakeSchedules
	err error
}

func (f *errSchedules) ListBySubject(ctx context.Context, s, u string) ([]model.Schedule, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	return f.fakeSchedules.ListBySubject(ctx, s, u)
}

func (f *errSchedules) Delete(ctx context.Context, id, u string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.fakeSchedules.Delete(ctx, id, u)
}

func (f *errSchedules) Create(ctx context.Context, u, s string, d int, st, en time.Time, mod *string) (*model.Schedule, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeSchedules.Create(ctx, u, s, d, st, en, mod)
}

type errGrades struct {
	*fakeGrades
	getErr    error
	updateErr error
	deleteErr error
	listErr   error
	createErr error
	countErr  error
}

func (f *errGrades) GetByID(ctx context.Context, id, u string) (*model.Grade, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.fakeGrades.GetByID(ctx, id, u)
}

func (f *errGrades) ListBySubject(ctx context.Context, s, u string) ([]model.Grade, bool, error) {
	if f.listErr != nil {
		return nil, false, f.listErr
	}
	return f.fakeGrades.ListBySubject(ctx, s, u)
}

func (f *errGrades) Update(ctx context.Context, id, u string, p repository.GradePatch) (*model.Grade, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.fakeGrades.Update(ctx, id, u, p)
}

func (f *errGrades) Delete(ctx context.Context, id, u string) (bool, error) {
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	return f.fakeGrades.Delete(ctx, id, u)
}

func (f *errGrades) Create(ctx context.Context, u, s string, in repository.GradeCreate) (*model.Grade, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.fakeGrades.Create(ctx, u, s, in)
}

func (f *errGrades) CountFinalExams(ctx context.Context, s, u, e string) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.fakeGrades.CountFinalExams(ctx, s, u, e)
}

type errRequirements struct {
	*fakeRequirements
	err error
}

func (f *errRequirements) ListBySubject(ctx context.Context, s, u string) ([]model.Requirement, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	return f.fakeRequirements.ListBySubject(ctx, s, u)
}

func (f *errRequirements) Delete(ctx context.Context, id, u string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.fakeRequirements.Delete(ctx, id, u)
}

func (f *errRequirements) Create(ctx context.Context, u, s string, src *string, c bool, cp string, th float64, a string) (*model.Requirement, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeRequirements.Create(ctx, u, s, src, c, cp, th, a)
}

type errAbsences struct {
	*fakeAbsences
	err error
}

func (f *errAbsences) GetByID(ctx context.Context, id, u string) (*model.Absence, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.fakeAbsences.GetByID(ctx, id, u)
}

func (f *errAbsences) ListBySubject(ctx context.Context, s, u string) ([]model.Absence, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	return f.fakeAbsences.ListBySubject(ctx, s, u)
}

func (f *errAbsences) Delete(ctx context.Context, id, u string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.fakeAbsences.Delete(ctx, id, u)
}

func (f *errAbsences) CountsBySubject(ctx context.Context, s, u string) (int, int, bool, error) {
	if f.err != nil {
		return 0, 0, false, f.err
	}
	return f.fakeAbsences.CountsBySubject(ctx, s, u)
}

func TestCoverage_ServiceError_Error(t *testing.T) {
	se := NewServiceError("not_found")
	if se.Error() == "" || se.Code != "not_found" {
		t.Fatalf("Error(): %+v", se)
	}
}

func TestCoverage_Curriculum_Errores(t *testing.T) {
	ctx := context.Background()
	base := newFakeCurricula()
	svc := NewCurriculumService(&errCurricula{fakeCurricula: base, listErr: errBoom})
	if _, err := svc.List(ctx, userA); err != errBoom {
		t.Fatalf("list err: %v", err)
	}
	svc2 := NewCurriculumService(&errCurricula{fakeCurricula: base, getErr: errBoom})
	c, _ := NewCurriculumService(base).Create(ctx, userA, "M")
	if _, err := svc2.Get(ctx, userA, c.ID); err != errBoom {
		t.Fatalf("get err: %v", err)
	}
	svc3 := NewCurriculumService(&errCurricula{fakeCurricula: base, updateErr: errBoom})
	if _, err := svc3.Update(ctx, userA, c.ID, "X"); err != errBoom {
		t.Fatalf("update err: %v", err)
	}
	svc4 := NewCurriculumService(&errCurricula{fakeCurricula: base, deleteErr: errBoom})
	if err := svc4.Delete(ctx, userA, c.ID); err != errBoom {
		t.Fatalf("delete err: %v", err)
	}
	svc5 := NewCurriculumService(&errCurricula{fakeCurricula: base, createErr: errBoom})
	if _, err := svc5.Create(ctx, userA, "X"); err != errBoom {
		t.Fatalf("create err: %v", err)
	}
	// Sin user.
	if _, err := NewCurriculumService(base).Create(ctx, "  ", "X"); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if err := NewCurriculumService(base).Delete(ctx, "", c.ID); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := NewCurriculumService(base).Get(ctx, "", c.ID); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := NewCurriculumService(base).List(ctx, ""); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := NewCurriculumService(base).Update(ctx, "", c.ID, "X"); err == nil {
		t.Fatal("sin user 401")
	} else {
		mustCode(t, err, "unauthorized")
	}
	// UUID malo en update/delete.
	if _, err := NewCurriculumService(base).Update(ctx, userA, "x", "X"); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if err := NewCurriculumService(base).Delete(ctx, userA, "x"); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}

func TestCoverage_Subject_Errores(t *testing.T) {
	ctx := context.Background()
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M")
	mat, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("S")})
	svc := NewSubjectService(&errSubjects{fakeSubjects: sub, err: errBoom})
	if _, err := svc.Get(ctx, userA, mat.ID); err != errBoom {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.List(ctx, userA, malla.ID); err != errBoom {
		t.Fatalf("list: %v", err)
	}
	if _, err := svc.Update(ctx, userA, mat.ID, SubjectInput{Status: strPtr("approved")}); err != errBoom {
		t.Fatalf("update: %v", err)
	}
	if err := svc.Delete(ctx, userA, mat.ID); err != errBoom {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("X")}); err != errBoom {
		t.Fatalf("create: %v", err)
	}
	// Sin user / uuid malo en cada método.
	svcOK := NewSubjectService(sub)
	for _, fn := range []func() error{
		func() error { _, e := svcOK.Get(ctx, "", mat.ID); return e },
		func() error { _, e := svcOK.List(ctx, "", malla.ID); return e },
		func() error {
			_, e := svcOK.Update(ctx, "", mat.ID, SubjectInput{Status: strPtr("approved")})
			return e
		},
		func() error { return svcOK.Delete(ctx, "", mat.ID) },
		func() error {
			_, e := svcOK.Create(ctx, "", malla.ID, SubjectInput{Name: strPtr("X")})
			return e
		},
	} {
		if err := fn(); err == nil {
			t.Fatal("sin user 401")
		} else {
			mustCode(t, err, "unauthorized")
		}
	}
	if _, err := svcOK.Get(ctx, userA, "x"); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svcOK.List(ctx, userA, "x"); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if err := svcOK.Delete(ctx, userA, "x"); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	// Update por campo inválido.
	if _, err := svcOK.Update(ctx, userA, mat.ID, SubjectInput{Status: strPtr("zzz")}); err == nil {
		t.Fatal("status 400")
	} else {
		mustCode(t, err, "invalid_status")
	}
	if _, err := svcOK.Update(ctx, userA, mat.ID, SubjectInput{AverageDecimalPlaces: intPtr(9)}); err == nil {
		t.Fatal("decimal 400")
	} else {
		mustCode(t, err, "invalid_decimal_places")
	}
	if _, err := svcOK.Update(ctx, userA, mat.ID, SubjectInput{Name: strPtr("")}); err == nil {
		t.Fatal("name 400")
	} else {
		mustCode(t, err, "invalid_name")
	}
	if _, err := svcOK.Update(ctx, userA, "x", SubjectInput{Status: strPtr("approved")}); err == nil {
		t.Fatal("uuid 400")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}

func TestCoverage_Schedule_Errores(t *testing.T) {
	ctx := context.Background()
	cur := newFakeCurricula()
	sub := newFakeSubjects(cur)
	sch := newFakeSchedules(sub)
	malla, _ := NewCurriculumService(cur).Create(ctx, userA, "M")
	mat, _ := NewSubjectService(sub).Create(ctx, userA, malla.ID, SubjectInput{Name: strPtr("S")})
	svc := NewScheduleService(&errSchedules{fakeSchedules: sch, err: errBoom})
	if _, err := svc.List(ctx, userA, mat.ID); err != errBoom {
		t.Fatalf("list: %v", err)
	}
	if err := svc.Delete(ctx, userA, mat.ID); err != errBoom {
		t.Fatalf("delete: %v", err)
	}
	svcOK := NewScheduleService(sch)
	if _, err := svcOK.List(ctx, "", mat.ID); err == nil {
		t.Fatal("sin user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if err := svcOK.Delete(ctx, "", mat.ID); err == nil {
		t.Fatal("sin user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svcOK.List(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if err := svcOK.Delete(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svcOK.Create(ctx, "", mat.ID, 1, "08:00", "09:00", nil); err == nil {
		t.Fatal("sin user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svcOK.Create(ctx, userA, "x", 1, "08:00", "09:00", nil); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
}

func TestCoverage_Grade_GetDelete_Update(t *testing.T) {
	ctx := context.Background()
	g := newFakeGrades()
	subID := newUUID()
	g.subs[subID] = userA
	svc := NewGradeService(g)
	s70 := 70
	created, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("G"), Weight: floatPtr(0.5), Score: &s70})
	if err != nil {
		t.Fatal(err)
	}
	// Get: ok, ajeno 404, uuid 400, sin user, repo error.
	if _, err := svc.Get(ctx, userA, created.ID); err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.Get(ctx, userB, created.ID); err == nil {
		t.Fatal("ajeno")
	} else {
		mustCode(t, err, "not_found")
	}
	if _, err := svc.Get(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svc.Get(ctx, "", created.ID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	svcErr := NewGradeService(&errGrades{fakeGrades: g, getErr: errBoom})
	if _, err := svcErr.Get(ctx, userA, created.ID); err != errBoom {
		t.Fatalf("repo err: %v", err)
	}
	// Delete: repo error, ajeno, uuid, sin user, ok.
	svcDel := NewGradeService(&errGrades{fakeGrades: g, deleteErr: errBoom})
	if err := svcDel.Delete(ctx, userA, created.ID); err != errBoom {
		t.Fatalf("repo err: %v", err)
	}
	if err := svc.Delete(ctx, "", created.ID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if err := svc.Delete(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	// Update: cada validación + repo error + nil + count error + sin user.
	svcUpd := NewGradeService(&errGrades{fakeGrades: g, updateErr: errBoom})
	if _, err := svcUpd.Update(ctx, userA, created.ID, GradeInput{Name: strPtr("N")}); err != errBoom {
		t.Fatalf("repo err: %v", err)
	}
	svcCnt := NewGradeService(&errGrades{fakeGrades: g, countErr: errBoom})
	if _, err := svcCnt.Update(ctx, userA, created.ID, GradeInput{IsFinalExam: boolPtr(true)}); err != errBoom {
		t.Fatalf("count err: %v", err)
	}
	if _, err := svc.Update(ctx, "", created.ID, GradeInput{Name: strPtr("N")}); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.Update(ctx, userA, "x", GradeInput{Name: strPtr("N")}); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{Name: strPtr("")}); err == nil {
		t.Fatal("name")
	} else {
		mustCode(t, err, "invalid_name")
	}
	badScore := 5
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{Score: &badScore}); err == nil {
		t.Fatal("score")
	} else {
		mustCode(t, err, "invalid_score")
	}
	badW := 2.0
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{Weight: &badW}); err == nil {
		t.Fatal("weight")
	} else {
		mustCode(t, err, "invalid_weight")
	}
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{Status: strPtr("zzz")}); err == nil {
		t.Fatal("status")
	} else {
		mustCode(t, err, "invalid_status")
	}
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{ScheduledDate: strPtr("ayer")}); err == nil {
		t.Fatal("date")
	} else {
		mustCode(t, err, "invalid_date")
	}
	// Update con campos válidos (topics, notify, evento, clear).
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{Topics: strPtr("T"), NotifyDiscord: boolPtr(true)}); err != nil {
		t.Fatalf("patch ok: %v", err)
	}
	if _, err := svc.Update(ctx, userA, created.ID, GradeInput{ClearScore: true, ClearTopics: true}); err != nil {
		t.Fatalf("clear ok: %v", err)
	}
	// List: repo error + sin user + uuid.
	svcList := NewGradeService(&errGrades{fakeGrades: g, listErr: errBoom})
	if _, err := svcList.List(ctx, userA, subID); err != errBoom {
		t.Fatalf("list err: %v", err)
	}
	if _, err := svc.List(ctx, "", subID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.List(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	// CreateChild: sin user, uuid, repo error, create error.
	if _, err := svc.CreateChild(ctx, "", created.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5)}); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.CreateChild(ctx, userA, "x", GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5)}); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	svcChild := NewGradeService(&errGrades{fakeGrades: g, getErr: errBoom})
	if _, err := svcChild.CreateChild(ctx, userA, created.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5)}); err != errBoom {
		t.Fatalf("get err: %v", err)
	}
	svcChild2 := NewGradeService(&errGrades{fakeGrades: g, createErr: errBoom})
	if _, err := svcChild2.CreateChild(ctx, userA, created.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5)}); err != errBoom {
		t.Fatalf("create err: %v", err)
	}
	// Create: count error con final.
	svcCreate := NewGradeService(&errGrades{fakeGrades: g, countErr: errBoom})
	if _, err := svcCreate.Create(ctx, userA, subID, GradeInput{Name: strPtr("F"), Weight: floatPtr(0.5), IsFinalExam: boolPtr(true)}); err != errBoom {
		t.Fatalf("count err: %v", err)
	}
	if _, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("F"), Weight: floatPtr(0.5), ScheduledDate: strPtr("ayer")}); err == nil {
		t.Fatal("date")
	} else {
		mustCode(t, err, "invalid_date")
	}
	// Delete final ok.
	if err := svc.Delete(ctx, userA, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, userA, created.ID); err == nil {
		t.Fatal("tras delete 404")
	} else {
		mustCode(t, err, "not_found")
	}
}

func TestCoverage_Requirement_ListDelete_Errores(t *testing.T) {
	ctx := context.Background()
	g := newFakeGrades()
	subID := newUUID()
	g.subs[subID] = userA
	r := newFakeRequirements(g.subs)
	svc := NewRequirementService(r)
	tf, ge, fe, thr := true, ">=", "force_exam", 4.0
	created, err := svc.Create(ctx, userA, subID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe})
	if err != nil {
		t.Fatal(err)
	}
	svcErr := NewRequirementService(&errRequirements{fakeRequirements: r, err: errBoom})
	if _, err := svcErr.List(ctx, userA, subID); err != errBoom {
		t.Fatalf("list: %v", err)
	}
	if err := svcErr.Delete(ctx, userA, created.ID); err != errBoom {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svcErr.Create(ctx, userA, subID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err != errBoom {
		// GradeSubjectOf no falla en este fake; Create propaga err del store.
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.List(ctx, "", subID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if err := svc.Delete(ctx, "", created.ID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.List(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if err := svc.Delete(ctx, userA, "x"); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	if _, err := svc.Create(ctx, "", subID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svc.Create(ctx, userA, "x", RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe}); err == nil {
		t.Fatal("uuid")
	} else {
		mustCode(t, err, "invalid_uuid")
	}
	// Threshold ausente → 400.
	if _, err := svc.Create(ctx, userA, subID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, Action: &fe}); err == nil {
		t.Fatal("sin threshold 400")
	} else {
		mustCode(t, err, "invalid_requirement")
	}
}

func TestCoverage_Absence_Errores(t *testing.T) {
	ctx := context.Background()
	abs := newFakeAbsences(map[string]string{})
	svc := NewAbsenceService(&errAbsences{fakeAbsences: abs, err: errBoom})
	subID := newUUID()
	if _, err := svc.Get(ctx, userA, subID); err != errBoom {
		t.Fatalf("get: %v", err)
	}
	if _, err := svc.List(ctx, userA, subID); err != errBoom {
		t.Fatalf("list: %v", err)
	}
	if err := svc.Delete(ctx, userA, subID); err != errBoom {
		t.Fatalf("delete: %v", err)
	}
	svcOK := NewAbsenceService(abs)
	if _, err := svcOK.Get(ctx, "", subID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svcOK.List(ctx, "", subID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if err := svcOK.Delete(ctx, "", subID); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svcOK.Create(ctx, "", subID, "2026-01-01", false); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
	if _, err := svcOK.SetJustified(ctx, "", subID, boolPtr(true)); err == nil {
		t.Fatal("user")
	} else {
		mustCode(t, err, "unauthorized")
	}
}

func TestCoverage_Comparadores(t *testing.T) {
	cases := []struct {
		v, th float64
		op    string
		want  bool
	}{
		{5, 4, ">", true}, {4, 4, ">", false},
		{4, 4, ">=", true}, {3, 4, ">=", false},
		{3, 4, "<", true}, {4, 4, "<", false},
		{4, 4, "<=", true}, {5, 4, "<=", false},
		{4, 4, "=", true}, {4, 5, "=", false},
		{1, 2, "??", false},
	}
	for _, tc := range cases {
		if got := applyComparator(tc.v, tc.th, tc.op); got != tc.want {
			t.Fatalf("compare %v %s %v: %v", tc.v, tc.op, tc.th, got)
		}
	}
	if got := RoundHalfUp(2.5, 0); got != 3 {
		t.Fatalf("round: %v", got)
	}
}

func newUUID() string {
	return "aaaaaaaa-0000-4000-8000-000000000001"
}

// fakeAbsencesNil devuelve lista nil (rama out==nil → []).
type fakeAbsencesNil struct {
	*fakeAbsences
}

func (f *fakeAbsencesNil) ListBySubject(ctx context.Context, s, u string) ([]model.Absence, bool, error) {
	return nil, true, nil
}

func TestCoverage_Absence_ListNil(t *testing.T) {
	ctx := context.Background()
	svc := NewAbsenceService(&fakeAbsencesNil{fakeAbsences: newFakeAbsences(map[string]string{})})
	out, err := svc.List(ctx, userA, newUUID())
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("lista [] nunca null: %+v", out)
	}
}

// fakeGradesNil devuelve lista nil para ListBySubject.
type fakeGradesNil struct {
	*fakeGrades
}

func (f *fakeGradesNil) ListBySubject(ctx context.Context, s, u string) ([]model.Grade, bool, error) {
	return nil, true, nil
}

func TestCoverage_Grade_CreateChild_Validaciones(t *testing.T) {
	ctx := context.Background()
	g := newFakeGrades()
	subID := newUUID()
	g.subs[subID] = userA
	svc := NewGradeService(g)
	padre, err := svc.Create(ctx, userA, subID, GradeInput{Name: strPtr("P"), Weight: floatPtr(1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Weight: floatPtr(0.5)}); err == nil {
		t.Fatal("sin name 400")
	} else {
		mustCode(t, err, "invalid_name")
	}
	if _, err := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(2)}); err == nil {
		t.Fatal("weight 400")
	} else {
		mustCode(t, err, "invalid_weight")
	}
	badScore := 99
	if _, err := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5), Score: &badScore}); err == nil {
		t.Fatal("score 400")
	} else {
		mustCode(t, err, "invalid_score")
	}
	if _, err := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5), Status: strPtr("zzz")}); err == nil {
		t.Fatal("status 400")
	} else {
		mustCode(t, err, "invalid_status")
	}
	if _, err := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5), ScheduledDate: strPtr("x")}); err == nil {
		t.Fatal("date 400")
	} else {
		mustCode(t, err, "invalid_date")
	}
	if _, err := svc.CreateChild(ctx, userA, padre.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5), IsFinalExam: boolPtr(true)}); err != nil {
		t.Fatalf("hija final ok (límite por materia): %v", err)
	}
	// Padre ajeno → 404.
	if _, err := svc.CreateChild(ctx, userB, padre.ID, GradeInput{Name: strPtr("H"), Weight: floatPtr(0.5)}); err == nil {
		t.Fatal("padre ajeno 404")
	} else {
		mustCode(t, err, "not_found")
	}
	// List con repo nil → [] nunca null.
	svcNil := NewGradeService(&fakeGradesNil{fakeGrades: g})
	nodes, err := svcNil.List(ctx, userA, subID)
	if err != nil {
		t.Fatal(err)
	}
	if nodes == nil || len(nodes) != 0 {
		t.Fatalf("nodos []: %+v", nodes)
	}
}

// Fakes mixtos para las ramas post-Get de Detail.Get.

type okSubjectGetter struct{ sub *model.Subject }

func (f *okSubjectGetter) Get(ctx context.Context, u, id string) (*model.Subject, error) {
	return f.sub, nil
}

type boomScheduleLister struct{ e error }

func (f *boomScheduleLister) List(ctx context.Context, u, s string) ([]model.Schedule, error) {
	return nil, f.e
}

type boomGradeLister struct{ e error }

func (f *boomGradeLister) List(ctx context.Context, u, s string) ([]GradeNode, error) {
	return nil, f.e
}

type okGradeLister struct{}

func (f *okGradeLister) List(ctx context.Context, u, s string) ([]GradeNode, error) {
	return []GradeNode{}, nil
}

type boomAverager struct{ e error }

func (f *boomAverager) Average(ctx context.Context, u, s string) (*AverageResult, error) {
	return nil, f.e
}

type okAverager struct{}

func (f *okAverager) Average(ctx context.Context, u, s string) (*AverageResult, error) {
	return &AverageResult{Status: "pending"}, nil
}

type boomAttender struct{ e error }

func (f *boomAttender) Summary(ctx context.Context, u, s string) (*AttendanceSummary, error) {
	return nil, f.e
}

type okAttender struct{}

func (f *okAttender) Summary(ctx context.Context, u, s string) (*AttendanceSummary, error) {
	return &AttendanceSummary{}, nil
}

func TestCoverage_Detail_RamasError(t *testing.T) {
	ctx := context.Background()
	sub := &model.Subject{ID: newUUID()}
	cases := []struct {
		name string
		d    *DetailService
	}{
		{"schedules", NewDetailService(&okSubjectGetter{sub: sub}, &boomScheduleLister{e: errBoom}, &okGradeLister{}, &okAverager{}, &okAttender{})},
		{"grades", NewDetailService(&okSubjectGetter{sub: sub}, &okScheduleLister{}, &boomGradeLister{e: errBoom}, &okAverager{}, &okAttender{})},
		{"average", NewDetailService(&okSubjectGetter{sub: sub}, &okScheduleLister{}, &okGradeLister{}, &boomAverager{e: errBoom}, &okAttender{})},
		{"attendance", NewDetailService(&okSubjectGetter{sub: sub}, &okScheduleLister{}, &okGradeLister{}, &okAverager{}, &boomAttender{e: errBoom})},
	}
	for _, tc := range cases {
		if _, err := tc.d.Get(ctx, userA, sub.ID); err != errBoom {
			t.Fatalf("%s: esperado boom, got %v", tc.name, err)
		}
	}
}

type okScheduleLister struct{}

func (f *okScheduleLister) List(ctx context.Context, u, s string) ([]model.Schedule, error) {
	return []model.Schedule{}, nil
}
