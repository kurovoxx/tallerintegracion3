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

type gradesFixture struct {
	engine     *gin.Engine
	subjectA   string
	subjectB   string
	foreignSub string
}

func newGradesFixture(t *testing.T) *gradesFixture {
	t.Helper()
	cur, sub, _ := newServiceFixture()
	grd := newFakeGrades(sub)
	req := newFakeRequirements(sub, grd)
	ctx := t.Context()
	_ = ctx

	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret:   []byte(testSecret),
		Issuer:   middleware.TokenIssuer,
		Audience: middleware.TokenAudience,
	})
	curSvc := service.NewCurriculumService(cur)
	subSvc := service.NewSubjectService(sub)
	gradeSvc := service.NewGradeService(grd)
	avgSvc := service.NewAverageService(sub, grd, req)
	reqSvc := service.NewRequirementService(req)

	// Seed: malla+materia por usuario (vía services, como las crearía el CRUD).
	mallaA, err := curSvc.Create(ctx, testUserA, "Malla A")
	if err != nil {
		t.Fatal(err)
	}
	mallaB, err := curSvc.Create(ctx, testUserB, "Malla B")
	if err != nil {
		t.Fatal(err)
	}
	matA, err := subSvc.Create(ctx, testUserA, mallaA.ID, service.SubjectInput{Name: gsptr("Mat A")})
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
	grdH := NewGradesHandler(gradeSvc, avgSvc)
	reqH := NewRequirementsHandler(reqSvc)
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
	return &gradesFixture{engine: r, subjectA: matA.ID, subjectB: matB.ID, foreignSub: matB.ID}
}

func (f *gradesFixture) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
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

func gradeID(t *testing.T, w *httptest.ResponseRecorder) string {
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

func gsptr(s string) *string { return &s }

func TestGrades_FlujoCompleto(t *testing.T) {
	f := newGradesFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	id := gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/grades", `{"name":"Parcial 1","score":55,"weight":0.4,"status":"graded"}`, tok))

	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/grades", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), id) {
		t.Fatalf("list anidada: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/grades/"+id, "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), "Parcial 1") {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
	// scheduled_date con formato fecha.
	if w := f.do(t, "PATCH", "/grades/"+id, `{"scheduled_date":"2026-11-01"}`, tok); w.Code != 200 || !strings.Contains(w.Body.String(), "2026-11-01") {
		t.Fatalf("patch fecha: %d %s", w.Code, w.Body.String())
	}
	// null explícito limpia el score (vuelve a programada).
	if w := f.do(t, "PATCH", "/grades/"+id, `{"score":null}`, tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"score":null`) {
		t.Fatalf("patch null: %d %s", w.Code, w.Body.String())
	}

	// Subnota + promedio del padre.
	padre := gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/grades", `{"name":"Proyecto","weight":0.6}`, tok))
	hijo := gradeID(t, f.do(t, "POST", "/grades/"+padre+"/children", `{"name":"Avance","score":60,"weight":0.5}`, tok))
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/grades", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), hijo) {
		t.Fatalf("hija anidada: %d %s", w.Code, w.Body.String())
	}
	_ = hijo
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/average", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"average":6`) {
		t.Fatalf("average 6.0 (solo hija califica): %d %s", w.Code, w.Body.String())
	}

	if w := f.do(t, "DELETE", "/grades/"+id, "", tok); w.Code != 204 {
		t.Fatalf("delete 204, got %d", w.Code)
	}
	if w := f.do(t, "GET", "/grades/"+id, "", tok); w.Code != 404 {
		t.Fatalf("tras delete 404, got %d", w.Code)
	}
}

func TestGrades_CrossUser_404(t *testing.T) {
	f := newGradesFixture(t)
	tokA := mintToken(t, testUserA, testSecret, time.Hour)
	tokB := mintToken(t, testUserB, testSecret, time.Hour)

	id := gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/grades", `{"name":"Privada","weight":0.5}`, tokA))
	child := gradeID(t, f.do(t, "POST", "/grades/"+id+"/children", `{"name":"H","weight":0.5}`, tokA))
	reqID := gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/requirements", `{"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam"}`, tokA))

	cases := []struct{ method, path, body string }{
		{"POST", "/subjects/" + f.subjectA + "/grades", `{"name":"Hack","weight":0.5}`},
		{"GET", "/subjects/" + f.subjectA + "/grades", ""},
		{"GET", "/grades/" + id, ""},
		{"PATCH", "/grades/" + id, `{"name":"Hack"}`},
		{"DELETE", "/grades/" + id, ""},
		{"POST", "/grades/" + id + "/children", `{"name":"Hack","weight":0.5}`},
		{"GET", "/subjects/" + f.subjectA + "/average", ""},
		{"POST", "/subjects/" + f.subjectA + "/requirements", `{"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam"}`},
		{"GET", "/subjects/" + f.subjectA + "/requirements", ""},
		{"DELETE", "/requirements/" + reqID, ""},
		{"DELETE", "/grades/" + child, ""},
	}
	for _, tc := range cases {
		w := f.do(t, tc.method, tc.path, tc.body, tokB)
		if w.Code != 404 {
			t.Fatalf("B %s %s: esperado 404, got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// Rutas sin token → 401.
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/average", "", ""); w.Code != 401 {
		t.Fatalf("average sin token 401, got %d", w.Code)
	}
}

func TestGrades_Validaciones_400(t *testing.T) {
	f := newGradesFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	final := gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/grades", `{"name":"Final","weight":0.3,"is_final_exam":true}`, tok))

	cases := []struct {
		name, method, path, body, wantCode string
	}{
		{"sin name", "POST", "/subjects/" + f.subjectA + "/grades", `{"weight":0.5}`, "invalid_name"},
		{"weight >1", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X","weight":1.5}`, "invalid_weight"},
		{"weight <0", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X","weight":-0.2}`, "invalid_weight"},
		{"weight ausente", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X"}`, "invalid_weight"},
		{"score 5", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X","weight":0.5,"score":5}`, "invalid_score"},
		{"score 71", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X","weight":0.5,"score":71}`, "invalid_score"},
		{"status malo", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X","weight":0.5,"status":"zzz"}`, "invalid_status"},
		{"fecha mala", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"X","weight":0.5,"scheduled_date":"ayer"}`, "invalid_date"},
		{"materia ajena", "POST", "/subjects/" + f.foreignSub + "/grades", `{"name":"X","weight":0.5}`, "not_found_404"},
		{"uuid malo", "GET", "/grades/no-uuid", "", "invalid_uuid"},
		{"final duplicado", "POST", "/subjects/" + f.subjectA + "/grades", `{"name":"Final 2","weight":0.3,"is_final_exam":true}`, "conflict"},
		{"req ambas fuentes", "POST", "/subjects/" + f.subjectA + "/requirements", `{"source_grade_id":"` + final + `","checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"force_exam"}`, "invalid_requirement"},
		{"req sin fuente", "POST", "/subjects/" + f.subjectA + "/requirements", `{"comparator":">=","threshold_value":4.0,"action":"force_exam"}`, "invalid_requirement"},
		{"req comparador malo", "POST", "/subjects/" + f.subjectA + "/requirements", `{"checks_final_average":true,"comparator":"=>","threshold_value":4.0,"action":"force_exam"}`, "invalid_comparator"},
		{"req acción mala", "POST", "/subjects/" + f.subjectA + "/requirements", `{"checks_final_average":true,"comparator":">=","threshold_value":4.0,"action":"volar"}`, "invalid_action"},
		{"req fuente ajena", "POST", "/subjects/" + f.subjectA + "/requirements", `{"source_grade_id":"11111111-1111-4111-8111-111111111111","comparator":">=","threshold_value":4.0,"action":"force_exam"}`, "invalid_requirement"},
	}
	for _, tc := range cases {
		w := f.do(t, tc.method, tc.path, tc.body, tok)
		wantStatus := 400
		wantCode := tc.wantCode
		if wantCode == "not_found_404" {
			wantStatus, wantCode = 404, "not_found"
		}
		if tc.name == "final duplicado" {
			wantStatus, wantCode = 409, "conflict"
		}
		if w.Code != wantStatus || !strings.Contains(w.Body.String(), wantCode) {
			t.Fatalf("%s: esperado %d %s, got %d %s", tc.name, wantStatus, wantCode, w.Code, w.Body.String())
		}
	}
	_ = final
}

func TestGrades_Average_Requisitos_HTTP(t *testing.T) {
	f := newGradesFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/grades", `{"name":"A","score":70,"weight":0.5}`, tok))
	gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/grades", `{"name":"B","score":40,"weight":0.5}`, tok))
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/average", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"average":5.5`) || !strings.Contains(w.Body.String(), `"approved"`) {
		t.Fatalf("average 5.5 approved: %d %s", w.Code, w.Body.String())
	}
	gradeID(t, f.do(t, "POST", "/subjects/"+f.subjectA+"/requirements", `{"checks_final_average":true,"comparator":"<","threshold_value":6.0,"action":"force_fail"}`, tok))
	if w := f.do(t, "GET", "/subjects/"+f.subjectA+"/average", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), `"failed"`) {
		t.Fatalf("force_fail: %d %s", w.Code, w.Body.String())
	}
}
