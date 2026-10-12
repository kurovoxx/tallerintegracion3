package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/utils"
)

// Fakes que devuelven error genérico (no ServiceError) para ejercitar la
// rama 500 de respondServiceError en cada handler.

type boomCurricula struct{ e error }

func (f *boomCurricula) Create(ctx context.Context, u, n string) (*model.Curriculum, error) {
	return nil, f.e
}
func (f *boomCurricula) GetByID(ctx context.Context, id, u string) (*model.Curriculum, error) {
	return nil, f.e
}
func (f *boomCurricula) ListByUser(ctx context.Context, u string) ([]model.Curriculum, error) {
	return nil, f.e
}
func (f *boomCurricula) UpdateName(ctx context.Context, id, u, n string) (*model.Curriculum, error) {
	return nil, f.e
}
func (f *boomCurricula) Delete(ctx context.Context, id, u string) (bool, error) {
	return false, f.e
}

type boomSubjects struct{ e error }

func (f *boomSubjects) Create(ctx context.Context, u, c string, in repository.SubjectCreate) (*model.Subject, error) {
	return nil, f.e
}
func (f *boomSubjects) GetByID(ctx context.Context, id, u string) (*model.Subject, error) {
	return nil, f.e
}
func (f *boomSubjects) ListByCurriculum(ctx context.Context, c, u string) ([]model.Subject, bool, error) {
	return nil, false, f.e
}
func (f *boomSubjects) Update(ctx context.Context, id, u string, p repository.SubjectPatch) (*model.Subject, error) {
	return nil, f.e
}
func (f *boomSubjects) Delete(ctx context.Context, id, u string) (bool, error) {
	return false, f.e
}
func (f *boomSubjects) CountsByCurricula(ctx context.Context, u string) (map[string]repository.SubjectCounts, error) {
	return nil, f.e
}

type boomSchedules struct{ e error }

func (f *boomSchedules) Create(ctx context.Context, u, s string, d int, st, en time.Time, mod *string) (*model.Schedule, error) {
	return nil, f.e
}

func (f *boomSchedules) ListBySubject(ctx context.Context, s, u string) ([]model.Schedule, bool, error) {
	return nil, false, f.e
}

func (f *boomSchedules) Delete(ctx context.Context, id, u string) (bool, error) {
	return false, f.e
}

type boomGrades struct{ e error }

func (f *boomGrades) Create(ctx context.Context, u, s string, in repository.GradeCreate) (*model.Grade, error) {
	return nil, f.e
}
func (f *boomGrades) GetByID(ctx context.Context, id, u string) (*model.Grade, error) {
	return nil, f.e
}
func (f *boomGrades) ListBySubject(ctx context.Context, s, u string) ([]model.Grade, bool, error) {
	return nil, false, f.e
}
func (f *boomGrades) Update(ctx context.Context, id, u string, p repository.GradePatch) (*model.Grade, error) {
	return nil, f.e
}
func (f *boomGrades) Delete(ctx context.Context, id, u string) (bool, error) {
	return false, f.e
}
func (f *boomGrades) CountFinalExams(ctx context.Context, s, u, e string) (int, error) {
	return 0, f.e
}

type boomRequirements struct{ e error }

func (f *boomRequirements) Create(ctx context.Context, u, s string, src *string, c bool, cp string, th float64, a string) (*model.Requirement, error) {
	return nil, f.e
}
func (f *boomRequirements) ListBySubject(ctx context.Context, s, u string) ([]model.Requirement, bool, error) {
	return nil, false, f.e
}
func (f *boomRequirements) Delete(ctx context.Context, id, u string) (bool, error) {
	return false, f.e
}
func (f *boomRequirements) GradeSubjectOf(ctx context.Context, g, s, u string) (bool, error) {
	return false, f.e
}

type boomAbsences struct{ e error }

func (f *boomAbsences) Create(ctx context.Context, u, s string, d time.Time, j bool) (*model.Absence, error) {
	return nil, f.e
}
func (f *boomAbsences) ExistsOnDate(ctx context.Context, u, s string, d time.Time) (bool, error) {
	return false, f.e
}
func (f *boomAbsences) GetByID(ctx context.Context, id, u string) (*model.Absence, error) {
	return nil, f.e
}
func (f *boomAbsences) ListBySubject(ctx context.Context, s, u string) ([]model.Absence, bool, error) {
	return nil, false, f.e
}
func (f *boomAbsences) UpdateJustified(ctx context.Context, id, u string, j bool) (*model.Absence, error) {
	return nil, f.e
}
func (f *boomAbsences) Delete(ctx context.Context, id, u string) (bool, error) {
	return false, f.e
}
func (f *boomAbsences) CountsBySubject(ctx context.Context, s, u string) (int, int, bool, error) {
	return 0, 0, false, f.e
}

var errHTTPBoom = errors.New("boom")

func boomEngine(t *testing.T) (*gin.Engine, string) {
	t.Helper()
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret: []byte(testSecret), Issuer: middleware.TokenIssuer, Audience: middleware.TokenAudience,
	})
	r := gin.New()
	g := r.Group("")
	g.Use(mw.RequireAuth())
	curH := NewCurriculaHandler(service.NewCurriculumService(&boomCurricula{e: errHTTPBoom}))
	subH := NewSubjectsHandler(service.NewSubjectService(&boomSubjects{e: errHTTPBoom}))
	schH := NewSchedulesHandler(service.NewScheduleService(&boomSchedules{e: errHTTPBoom}))
	grdSvc := service.NewGradeService(&boomGrades{e: errHTTPBoom})
	avgBoom := service.NewAverageService(&boomSubjects{e: errHTTPBoom}, &boomGrades{e: errHTTPBoom}, &boomRequirements{e: errHTTPBoom})
	attBoom := service.NewAttendanceService(&boomSubjects{e: errHTTPBoom}, &boomAbsences{e: errHTTPBoom})
	grdH := NewGradesHandler(grdSvc, avgBoom)
	reqH := NewRequirementsHandler(service.NewRequirementService(&boomRequirements{e: errHTTPBoom}))
	absH := NewAbsencesHandler(service.NewAbsenceService(&boomAbsences{e: errHTTPBoom}))
	attH := NewAttendanceHandler(attBoom)
	ovH := NewOverviewHandler(
		service.NewOverviewService(&boomCurriculaLister{e: errHTTPBoom}, &boomSubjects{e: errHTTPBoom}),
		service.NewDetailService(service.NewSubjectService(&boomSubjects{e: errHTTPBoom}),
			service.NewScheduleService(&boomSchedules{e: errHTTPBoom}), grdSvc, avgBoom, attBoom),
	)
	g.POST("/curricula", curH.Create)
	g.GET("/curricula", curH.List)
	g.GET("/curricula/:id", curH.Get)
	g.PATCH("/curricula/:id", curH.Update)
	g.DELETE("/curricula/:id", curH.Delete)
	g.POST("/curricula/:id/subjects", subH.Create)
	g.GET("/curricula/:id/subjects", subH.ListByCurriculum)
	g.GET("/subjects/:id", subH.Get)
	g.PATCH("/subjects/:id", subH.Update)
	g.DELETE("/subjects/:id", subH.Delete)
	g.POST("/subjects/:id/schedules", schH.Create)
	g.GET("/subjects/:id/schedules", schH.ListBySubject)
	g.DELETE("/schedules/:id", schH.Delete)
	g.POST("/subjects/:id/grades", grdH.Create)
	g.GET("/subjects/:id/grades", grdH.List)
	g.GET("/grades/:id", grdH.Get)
	g.PATCH("/grades/:id", grdH.Update)
	g.DELETE("/grades/:id", grdH.Delete)
	g.POST("/grades/:id/children", grdH.CreateChild)
	g.GET("/subjects/:id/average", grdH.Average)
	g.POST("/subjects/:id/requirements", reqH.Create)
	g.GET("/subjects/:id/requirements", reqH.List)
	g.DELETE("/requirements/:id", reqH.Delete)
	g.POST("/subjects/:id/absences", absH.Create)
	g.GET("/subjects/:id/absences", absH.ListBySubject)
	g.GET("/absences/:id", absH.Get)
	g.PATCH("/absences/:id", absH.Update)
	g.DELETE("/absences/:id", absH.Delete)
	g.GET("/subjects/:id/attendance", attH.Get)
	g.GET("/academic/overview", ovH.Overview)
	g.GET("/subjects/:id/detail", ovH.SubjectDetail)
	return r, tok
}

type boomCurriculaLister struct{ e error }

func (f *boomCurriculaLister) List(ctx context.Context, u string) ([]model.Curriculum, error) {
	return nil, f.e
}

func boomDo(t *testing.T, r *gin.Engine, method, path, body, tok string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCoverage_InternalError_500(t *testing.T) {
	r, tok := boomEngine(t)
	id := "11111111-1111-4111-8111-111111111111"
	cases := []struct{ method, path, body string }{
		{"POST", "/curricula", `{"name":"X"}`},
		{"GET", "/curricula", ""},
		{"GET", "/curricula/" + id, ""},
		{"PATCH", "/curricula/" + id, `{"name":"X"}`},
		{"DELETE", "/curricula/" + id, ""},
		{"POST", "/curricula/" + id + "/subjects", `{"name":"X"}`},
		{"GET", "/curricula/" + id + "/subjects", ""},
		{"GET", "/subjects/" + id, ""},
		{"PATCH", "/subjects/" + id, `{"name":"X"}`},
		{"DELETE", "/subjects/" + id, ""},
		{"POST", "/subjects/" + id + "/schedules", `{"day_of_week":1,"start_time":"08:00","end_time":"09:00"}`},
		{"GET", "/subjects/" + id + "/schedules", ""},
		{"DELETE", "/schedules/" + id, ""},
		{"POST", "/subjects/" + id + "/grades", `{"name":"X","weight":0.5}`},
		{"GET", "/subjects/" + id + "/grades", ""},
		{"GET", "/grades/" + id, ""},
		{"PATCH", "/grades/" + id, `{"name":"X"}`},
		{"DELETE", "/grades/" + id, ""},
		{"POST", "/grades/" + id + "/children", `{"name":"X","weight":0.5}`},
		{"GET", "/subjects/" + id + "/average", ""},
		{"POST", "/subjects/" + id + "/requirements", `{"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam"}`},
		{"GET", "/subjects/" + id + "/requirements", ""},
		{"DELETE", "/requirements/" + id, ""},
		{"POST", "/subjects/" + id + "/absences", `{"date":"2026-01-01"}`},
		{"GET", "/subjects/" + id + "/absences", ""},
		{"GET", "/absences/" + id, ""},
		{"PATCH", "/absences/" + id, `{"justified":true}`},
		{"DELETE", "/absences/" + id, ""},
		{"GET", "/subjects/" + id + "/attendance", ""},
		{"GET", "/academic/overview", ""},
		{"GET", "/subjects/" + id + "/detail", ""},
	}
	for _, tc := range cases {
		w := boomDo(t, r, tc.method, tc.path, tc.body, tok)
		if w.Code != 500 || !strings.Contains(w.Body.String(), "internal_error") {
			t.Fatalf("%s %s: esperado 500 internal_error, got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	_ = utils.ErrInternal
}

func TestCoverage_BadBody_400(t *testing.T) {
	r, tok := boomEngine(t)
	id := "11111111-1111-4111-8111-111111111111"
	cases := []struct{ method, path string }{
		{"POST", "/curricula"},
		{"PATCH", "/curricula/" + id},
		{"POST", "/curricula/" + id + "/subjects"},
		{"PATCH", "/subjects/" + id},
		{"POST", "/subjects/" + id + "/schedules"},
		{"POST", "/subjects/" + id + "/grades"},
		{"PATCH", "/grades/" + id},
		{"POST", "/grades/" + id + "/children"},
		{"POST", "/subjects/" + id + "/requirements"},
		{"POST", "/subjects/" + id + "/absences"},
		{"PATCH", "/absences/" + id},
	}
	for _, tc := range cases {
		w := boomDo(t, r, tc.method, tc.path, `{invalid`, tok)
		if w.Code != 400 || !strings.Contains(w.Body.String(), "bad_request") {
			t.Fatalf("%s %s: esperado 400 bad_request, got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestCoverage_RequireUserID_Vacio_LoggingFallback(t *testing.T) {
	// Contexto directo sin engine (FullPath "") y user_id "" explícito:
	// rama userID=="" + fallback de ruta + userID "-" en el log.
	cur, _, _ := newServiceFixture()
	h := NewCurriculaHandler(service.NewCurriculumService(cur))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/curricula", nil)
	c.Set(middleware.ContextUserIDKey, "")
	h.List(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("esperado {error}: %s", w.Body.String())
	}
}

func TestCoverage_Requirements_Delete_HTTP(t *testing.T) {
	cur, sub, _ := newServiceFixture()
	grd := newFakeGrades(sub)
	req := newFakeRequirements(sub, grd)
	ctx := context.Background()
	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret: []byte(testSecret), Issuer: middleware.TokenIssuer, Audience: middleware.TokenAudience,
	})
	curSvc := service.NewCurriculumService(cur)
	subSvc := service.NewSubjectService(sub)
	malla, err := curSvc.Create(ctx, testUserA, "M")
	if err != nil {
		t.Fatal(err)
	}
	mat, err := subSvc.Create(ctx, testUserA, malla.ID, service.SubjectInput{Name: gsptr("S")})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	g := r.Group("")
	g.Use(mw.RequireAuth())
	reqH := NewRequirementsHandler(service.NewRequirementService(req))
	g.POST("/subjects/:id/requirements", reqH.Create)
	g.GET("/subjects/:id/requirements", reqH.List)
	g.DELETE("/requirements/:id", reqH.Delete)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := boomDo(t, r, "POST", "/subjects/"+mat.ID+"/requirements", `{"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam"}`, tok)
	if w.Code != 201 {
		t.Fatalf("create 201, got %d %s", w.Code, w.Body.String())
	}
	var created map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	w = boomDo(t, r, "GET", "/subjects/"+mat.ID+"/requirements", "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), created["id"]) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w = boomDo(t, r, "DELETE", "/requirements/"+created["id"], "", tok)
	if w.Code != 204 {
		t.Fatalf("delete 204, got %d", w.Code)
	}
	w = boomDo(t, r, "DELETE", "/requirements/"+created["id"], "", tok)
	if w.Code != 404 {
		t.Fatalf("delete repetido 404, got %d", w.Code)
	}
}
