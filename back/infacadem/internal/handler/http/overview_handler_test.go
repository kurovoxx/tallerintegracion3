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

type overviewFixture struct {
	engine   *gin.Engine
	subjectA string // aprobada, con nota 60/1.0 y 1 falta
	subjectB string // vacía (sin notas/horarios/faltas)
	otherSub string // de otro usuario
}

func newOverviewFixture(t *testing.T) *overviewFixture {
	t.Helper()
	cur, sub, sch := newServiceFixture()
	grd := newFakeGrades(sub)
	req := newFakeRequirements(sub, grd)
	abs := newFakeAbsences(sub)
	ctx := t.Context()

	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret:   []byte(testSecret),
		Issuer:   middleware.TokenIssuer,
		Audience: middleware.TokenAudience,
	})
	curSvc := service.NewCurriculumService(cur)
	subSvc := service.NewSubjectService(sub)
	schSvc := service.NewScheduleService(sch)
	gradeSvc := service.NewGradeService(grd)
	absSvc := service.NewAbsenceService(abs)
	avgSvc := service.NewAverageService(sub, grd, req)
	attSvc := service.NewAttendanceService(sub, abs)

	total, min := 10, 50.0
	mallaA, err := curSvc.Create(ctx, testUserA, "Malla A")
	if err != nil {
		t.Fatal(err)
	}
	mallaB, err := curSvc.Create(ctx, testUserB, "Malla B")
	if err != nil {
		t.Fatal(err)
	}
	matA, err := subSvc.Create(ctx, testUserA, mallaA.ID, service.SubjectInput{
		Name: gsptr("Aprobada"), Status: gsptr("approved"),
		TotalClasses: &total, MinAttendancePct: &min,
	})
	if err != nil {
		t.Fatal(err)
	}
	matEmpty, err := subSvc.Create(ctx, testUserA, mallaA.ID, service.SubjectInput{Name: gsptr("Vacía")})
	if err != nil {
		t.Fatal(err)
	}
	matB, err := subSvc.Create(ctx, testUserB, mallaB.ID, service.SubjectInput{Name: gsptr("Ajena")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gradeSvc.Create(ctx, testUserA, matA.ID, service.GradeInput{Name: gsptr("P1"), Weight: fptr(1), Score: iptr(60)}); err != nil {
		t.Fatal(err)
	}
	if _, err := absSvc.Create(ctx, testUserA, matA.ID, "2026-03-01", false); err != nil {
		t.Fatal(err)
	}

	r := gin.New()
	g := r.Group("")
	g.Use(mw.RequireAuth())
	ovH := NewOverviewHandler(
		service.NewOverviewService(curSvc, sub),
		service.NewDetailService(subSvc, schSvc, gradeSvc, avgSvc, attSvc),
	)
	g.GET("/academic/overview", ovH.Overview)
	g.GET("/subjects/:id/detail", ovH.SubjectDetail)
	// Individuales para comparar consistencia.
	subH := NewSubjectsHandler(subSvc)
	grdH := NewGradesHandler(gradeSvc, avgSvc)
	attH := NewAttendanceHandler(attSvc)
	g.GET("/subjects/:id", subH.Get)
	g.GET("/subjects/:id/grades", grdH.List)
	g.GET("/subjects/:id/schedules", NewSchedulesHandler(schSvc).ListBySubject)
	g.GET("/subjects/:id/average", grdH.Average)
	g.GET("/subjects/:id/attendance", attH.Get)
	return &overviewFixture{engine: r, subjectA: matA.ID, subjectB: matEmpty.ID, otherSub: matB.ID}
}

func fptr(f float64) *float64 { return &f }
func iptr(i int) *int         { return &i }

func (f *overviewFixture) do(t *testing.T, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(nil))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func TestOverview_HTTP(t *testing.T) {
	f := newOverviewFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := f.do(t, "GET", "/academic/overview", tok)
	if w.Code != 200 {
		t.Fatalf("overview 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Curricula []struct {
			ID              string `json:"id"`
			Name            string `json:"name"`
			SubjectsCount   int    `json:"subjects_count"`
			ApprovedCount   int    `json:"approved_count"`
			InProgressCount int    `json:"in_progress_count"`
		} `json:"curricula"`
		Stats struct {
			CurriculaCount int `json:"curricula_count"`
			SubjectsCount  int `json:"subjects_count"`
		} `json:"stats"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Curricula) != 1 || body.Curricula[0].Name != "Malla A" {
		t.Fatalf("solo propia: %s", w.Body.String())
	}
	c := body.Curricula[0]
	if c.ID == "" || c.SubjectsCount != 2 || c.ApprovedCount != 1 || c.InProgressCount != 1 {
		t.Fatalf("conteos: %+v", c)
	}
	if body.Stats.CurriculaCount != 1 || body.Stats.SubjectsCount != 2 {
		t.Fatalf("stats: %+v", body.Stats)
	}
	// B ve solo su malla con su materia (aislamiento: sin rastro de A).
	tokB := mintToken(t, testUserB, testSecret, time.Hour)
	wB := f.do(t, "GET", "/academic/overview", tokB)
	if !strings.Contains(wB.Body.String(), `"name":"Malla B"`) ||
		!strings.Contains(wB.Body.String(), `"subjects_count":1`) ||
		strings.Contains(wB.Body.String(), "Malla A") {
		t.Fatalf("B solo lo propio: %s", wB.Body.String())
	}
	if w := f.do(t, "GET", "/academic/overview", ""); w.Code != 401 {
		t.Fatalf("sin token 401, got %d", w.Code)
	}
}

func TestSubjectDetail_HTTP(t *testing.T) {
	f := newOverviewFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := f.do(t, "GET", "/subjects/"+f.subjectA+"/detail", tok)
	if w.Code != 200 {
		t.Fatalf("detail 200, got %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Subject struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"subject"`
		Schedules []any `json:"schedules"`
		Grades    []struct {
			Grade struct {
				Name string `json:"name"`
			} `json:"grade"`
			Children []any `json:"children"`
		} `json:"grades"`
		Average struct {
			Average *float64 `json:"average"`
			Status  string   `json:"status"`
		} `json:"average"`
		Attendance struct {
			UnjustifiedCount int      `json:"unjustified_count"`
			Percentage       *float64 `json:"percentage"`
		} `json:"attendance"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Subject.Name != "Aprobada" || len(body.Grades) != 1 || body.Grades[0].Grade.Name != "P1" {
		t.Fatalf("forma: %s", w.Body.String())
	}
	if body.Schedules == nil || body.Grades[0].Children == nil {
		t.Fatal("listas deben ser [] nunca null")
	}
	if body.Average.Average == nil || *body.Average.Average != 6.0 || body.Average.Status != "approved" {
		t.Fatalf("average: %s", w.Body.String())
	}
	if body.Attendance.Percentage == nil || *body.Attendance.Percentage != 0.9 || body.Attendance.UnjustifiedCount != 1 {
		t.Fatalf("attendance: %s", w.Body.String())
	}

	// Materia vacía: listas [], average null/pending, attendance con nulls.
	w = f.do(t, "GET", "/subjects/"+f.subjectB+"/detail", tok)
	if w.Code != 200 {
		t.Fatalf("detail vacía 200, got %d", w.Code)
	}
	for _, frag := range []string{`"schedules":[]`, `"grades":[]`, `"average":null`, `"status":"pending"`, `"percentage":null`} {
		if frag == `"average":null` {
			continue // average es objeto con average:null; ver abajo
		}
		if !strings.Contains(w.Body.String(), frag) {
			t.Fatalf("falta %s en %s", frag, w.Body.String())
		}
	}
	if !strings.Contains(w.Body.String(), `"average":{"average":null,"status":"pending"}`) {
		t.Fatalf("average null/pending: %s", w.Body.String())
	}

	// Ajena/inexistente → 404; uuid malo → 400; sin token → 401.
	if w := f.do(t, "GET", "/subjects/"+f.otherSub+"/detail", tok); w.Code != 404 {
		t.Fatalf("ajena 404, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/subjects/11111111-1111-4111-8111-111111111111/detail", tok); w.Code != 404 {
		t.Fatalf("inexistente 404, got %d", w.Code)
	}
	if w := f.do(t, "GET", "/subjects/no-uuid/detail", tok); w.Code != 400 {
		t.Fatalf("uuid malo 400, got %d", w.Code)
	}
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/detail", ""); w.Code != 401 {
		t.Fatalf("sin token 401, got %d", w.Code)
	}
}

func TestSubjectDetail_Consistencia(t *testing.T) {
	f := newOverviewFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	detail := f.do(t, "GET", "/subjects/"+f.subjectA+"/detail", tok)
	avg := f.do(t, "GET", "/subjects/"+f.subjectA+"/average", tok)
	att := f.do(t, "GET", "/subjects/"+f.subjectA+"/attendance", tok)
	grades := f.do(t, "GET", "/subjects/"+f.subjectA+"/grades", tok)
	sched := f.do(t, "GET", "/subjects/"+f.subjectA+"/schedules", tok)
	subj := f.do(t, "GET", "/subjects/"+f.subjectA, tok)
	for _, w := range []*httptest.ResponseRecorder{detail, avg, att, grades, sched, subj} {
		if w.Code != 200 {
			t.Fatalf("todas 200, got %d %s", w.Code, w.Body.String())
		}
	}
	var d map[string]json.RawMessage
	if err := json.Unmarshal(detail.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	// Mismos valores que las rutas individuales.
	for key, other := range map[string]*httptest.ResponseRecorder{
		"average": avg, "attendance": att,
	} {
		var a, b any
		_ = json.Unmarshal(d[key], &a)
		_ = json.Unmarshal(other.Body.Bytes(), &b)
		if string(mustJSON(t, a)) != string(mustJSON(t, b)) {
			t.Fatalf("%s difiere: %s vs %s", key, d[key], other.Body.String())
		}
	}
	var ggItems []any
	_ = json.Unmarshal(d["grades"], &ggItems)
	var giItems struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(grades.Body.Bytes(), &giItems)
	if string(mustJSON(t, ggItems)) != string(mustJSON(t, giItems.Items)) {
		t.Fatal("grades difiere del agregado")
	}
	var ds struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(d["schedules"], &ds.Items)
	var si struct {
		Items []any `json:"items"`
	}
	_ = json.Unmarshal(sched.Body.Bytes(), &si)
	if string(mustJSON(t, ds.Items)) != string(mustJSON(t, si.Items)) {
		t.Fatal("schedules difiere del agregado")
	}
	var dsub, ssub map[string]any
	_ = json.Unmarshal(d["subject"], &dsub)
	_ = json.Unmarshal(subj.Body.Bytes(), &ssub)
	if string(mustJSON(t, dsub)) != string(mustJSON(t, ssub)) {
		t.Fatal("subject difiere del agregado")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
