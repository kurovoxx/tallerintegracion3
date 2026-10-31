package http

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
)

type attendanceFixture struct {
	engine   *gin.Engine
	subjectA string
	subjectB string
}

func newAttendanceFixture(t *testing.T) *attendanceFixture {
	t.Helper()
	cur, sub, _ := newServiceFixture()
	abs := newFakeAbsences(sub)
	ctx := t.Context()

	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret:   []byte(testSecret),
		Issuer:   middleware.TokenIssuer,
		Audience: middleware.TokenAudience,
	})
	curSvc := service.NewCurriculumService(cur)
	subSvc := service.NewSubjectService(sub)
	absSvc := service.NewAbsenceService(abs)
	attSvc := service.NewAttendanceService(sub, abs)

	total, min := 32, 75.0
	mallaA, err := curSvc.Create(ctx, testUserA, "Malla A")
	if err != nil {
		t.Fatal(err)
	}
	mallaB, err := curSvc.Create(ctx, testUserB, "Malla B")
	if err != nil {
		t.Fatal(err)
	}
	matA, err := subSvc.Create(ctx, testUserA, mallaA.ID, service.SubjectInput{
		Name: gsptr("Mat A"), TotalClasses: &total, MinAttendancePct: &min,
	})
	if err != nil {
		t.Fatal(err)
	}
	matB, err := subSvc.Create(ctx, testUserB, mallaB.ID, service.SubjectInput{Name: gsptr("Mat B")})
	if err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	g := r.Group("")
	g.Use(mw.RequireAuth())
	absH := NewAbsencesHandler(absSvc)
	attH := NewAttendanceHandler(attSvc)
	g.POST("/subjects/:id/absences", absH.Create)
	g.GET("/subjects/:id/absences", absH.ListBySubject)
	g.GET("/absences/:id", absH.Get)
	g.PATCH("/absences/:id", absH.Update)
	g.DELETE("/absences/:id", absH.Delete)
	g.GET("/subjects/:id/attendance", attH.Get)
	return &attendanceFixture{engine: r, subjectA: matA.ID, subjectB: matB.ID}
}

func (f *attendanceFixture) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func absenceID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	if w.Code != 201 {
		t.Fatalf("esperado 201, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["id"] == "" {
		t.Fatalf("201 sin {id}: %s", w.Body.String())
	}
	return body["id"]
}

func TestAbsences_FlujoCompleto(t *testing.T) {
	f := newAttendanceFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	id := absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-03-10"}`, tok))
	absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-03-11","justified":true}`, tok))

	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/absences", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), id) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/absences/"+id, "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), "2026-03-10") {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "PATCH", "/absences/"+id, `{"justified":true}`, tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"justified":true`) {
		t.Fatalf("toggle: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/attendance", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"unjustified_count":0`) {
		t.Fatalf("attendance tras toggle: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "DELETE", "/absences/"+id, "", tok); w.Code != 204 {
		t.Fatalf("delete 204, got %d", w.Code)
	}
	if w := f.do(t, "GET", "/absences/"+id, "", tok); w.Code != 404 {
		t.Fatalf("tras delete 404, got %d", w.Code)
	}
}

func TestAbsences_CrossUser_404(t *testing.T) {
	f := newAttendanceFixture(t)
	tokA := mintToken(t, testUserA, testSecret, time.Hour)
	tokB := mintToken(t, testUserB, testSecret, time.Hour)
	id := absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-03-10"}`, tokA))

	cases := []struct{ method, path, body string }{
		{"POST", "/subjects/" + f.subjectA + "/absences", `{"date":"2026-03-11"}`},
		{"GET", "/subjects/" + f.subjectA + "/absences", ""},
		{"GET", "/absences/" + id, ""},
		{"PATCH", "/absences/" + id, `{"justified":true}`},
		{"DELETE", "/absences/" + id, ""},
		{"GET", "/subjects/" + f.subjectA + "/attendance", ""},
	}
	for _, tc := range cases {
		if w := f.do(t, tc.method, tc.path, tc.body, tokB); w.Code != 404 {
			t.Fatalf("B %s %s: esperado 404, got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/attendance", "", ""); w.Code != 401 {
		t.Fatalf("sin token 401, got %d", w.Code)
	}
}

func TestAbsences_Validaciones(t *testing.T) {
	f := newAttendanceFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-03-10"}`, tok))

	cases := []struct {
		name, method, path, body, wantCode string
		wantStatus                         int
	}{
		{"duplicado", "POST", "/subjects/" + f.subjectA + "/absences", `{"date":"2026-03-10"}`, "conflict", 409},
		{"fecha imposible", "POST", "/subjects/" + f.subjectA + "/absences", `{"date":"2026-02-30"}`, "invalid_date", 400},
		{"fecha no ISO", "POST", "/subjects/" + f.subjectA + "/absences", `{"date":"ayer"}`, "invalid_date", 400},
		{"sin fecha", "POST", "/subjects/" + f.subjectA + "/absences", `{}`, "bad_request", 400},
		{"materia ajena", "POST", "/subjects/" + f.subjectB + "/absences", `{"date":"2026-03-10"}`, "not_found", 404},
		{"uuid malo", "GET", "/absences/no-uuid", "", "invalid_uuid", 400},
		{"patch sin justified", "PATCH", "/subjects/" + f.subjectA + "/absences", `{}`, "bad_request", 400},
	}
	// El caso patch usa un id real de falta.
	id := absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-05-01"}`, tok))
	for _, tc := range cases {
		path := tc.path
		if strings.HasPrefix(tc.name, "patch ") {
			path = "/absences/" + id
		}
		w := f.do(t, tc.method, path, tc.body, tok)
		if w.Code != tc.wantStatus || !strings.Contains(w.Body.String(), tc.wantCode) {
			t.Fatalf("%s: esperado %d %s, got %d %s", tc.name, tc.wantStatus, tc.wantCode, w.Code, w.Body.String())
		}
	}
}

func TestAttendance_Resumen_HTTP(t *testing.T) {
	f := newAttendanceFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	for i := 1; i <= 5; i++ {
		absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-06-0`+string(rune('0'+i))+`"}`, tok))
	}
	absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-06-10","justified":true}`, tok))
	absenceID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/absences", `{"date":"2026-06-11","justified":true}`, tok))

	w := f.do(t, "GET", "/subjects/"+f.subjectA+"/attendance", "", tok)
	if w.Code != 200 {
		t.Fatalf("attendance 200, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["percentage"] != 0.84375 {
		t.Fatalf("percentage 0.84375, got %v (%s)", body["percentage"], w.Body.String())
	}
	if body["meets_minimum"] != true || body["remaining"] != 3.0 {
		t.Fatalf("meets true + remaining 3: %s", w.Body.String())
	}
	if body["unjustified_count"] != 5.0 || body["justified_count"] != 2.0 || body["total_classes"] != 32.0 {
		t.Fatalf("conteos/total: %s", w.Body.String())
	}
}
