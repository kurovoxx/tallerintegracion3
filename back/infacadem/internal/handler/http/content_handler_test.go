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

type contentsFixture struct {
	engine *gin.Engine
	mat    string
}

func newContentsFixture(t *testing.T) *contentsFixture {
	t.Helper()
	cur, sub, _ := newServiceFixture()
	ctx := t.Context()
	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret:   []byte(testSecret),
		Issuer:   middleware.TokenIssuer,
		Audience: middleware.TokenAudience,
	})
	curSvc := service.NewCurriculumService(cur)
	subSvc := service.NewSubjectService(sub)
	cntSvc := service.NewContentService(newFakeContents(sub))
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
	cntH := NewContentsHandler(cntSvc)
	grdH := NewGradesHandler(
		service.NewGradeService(newFakeGrades(sub)),
		service.NewAverageService(newFakeSubjects(cur), newFakeGrades(sub), newFakeRequirements(sub, newFakeGrades(sub))),
	)
	g.POST("/curricula/:id/subjects", NewSubjectsHandler(subSvc).Create)
	g.POST("/subjects/:id/contents", cntH.Create)
	g.GET("/subjects/:id/contents", cntH.ListBySubject)
	g.GET("/contents/:id", cntH.Get)
	g.PATCH("/contents/:id", cntH.Update)
	g.DELETE("/contents/:id", cntH.Delete)
	g.POST("/subjects/:id/grades", grdH.Create)
	g.GET("/subjects/:id/grades", grdH.List)
	g.PATCH("/grades/:id", grdH.Update)
	return &contentsFixture{engine: r, mat: mat.ID}
}

func (f *contentsFixture) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
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

func TestContents_FlujoCRUD(t *testing.T) {
	f := newContentsFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := f.do(t, "POST", "/subjects/"+f.mat+"/contents", `{"title":"U1","body":"Intro","position":1}`, tok)
	if w.Code != 201 {
		t.Fatalf("create 201: %d %s", w.Code, w.Body.String())
	}
	var created map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &created)

	w = f.do(t, "GET", "/contents/"+created["id"], "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"U1"`) {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "PATCH", "/contents/"+created["id"], `{"title":"U1b","body":null}`, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"title":"U1b"`) {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}

	// Toggle done persiste tras releer.
	w = f.do(t, "PATCH", "/contents/"+created["id"], `{"done":true}`, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"done":true`) {
		t.Fatalf("done: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "GET", "/contents/"+created["id"], "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"done":true`) {
		t.Fatalf("done persiste: %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "GET", "/subjects/"+f.mat+"/contents", "", tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), created["id"]) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "DELETE", "/contents/"+created["id"], "", tok)
	if w.Code != 204 {
		t.Fatalf("delete 204: %d", w.Code)
	}

	// Cross-user: todo 404, nunca 403.
	tokB := mintToken(t, testUserB, testSecret, time.Hour)
	w = f.do(t, "GET", "/subjects/"+f.mat+"/contents", "", tokB)
	if w.Code != 404 {
		t.Fatalf("ajena 404: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "PATCH", "/contents/"+created["id"], `{"title":"Hack"}`, tokB)
	if w.Code != 404 {
		t.Fatalf("patch ajeno 404: %d %s", w.Code, w.Body.String())
	}

	// 401 sin token en las 5 rutas.
	for _, tc := range [][2]string{
		{"POST", "/subjects/" + f.mat + "/contents"},
		{"GET", "/subjects/" + f.mat + "/contents"},
		{"GET", "/contents/" + created["id"]},
		{"PATCH", "/contents/" + created["id"]},
		{"DELETE", "/contents/" + created["id"]},
	} {
		body := ""
		if tc[0] == "POST" || tc[0] == "PATCH" {
			body = `{"title":"X"}`
		}
		if w := f.do(t, tc[0], tc[1], body, ""); w.Code != 401 {
			t.Fatalf("%s %s 401: %d", tc[0], tc[1], w.Code)
		}
	}

	// Body inválido → 400; UUID malo → 400; inexistente → 404.
	w = f.do(t, "POST", "/subjects/"+f.mat+"/contents", `{invalid`, tok)
	if w.Code != 400 {
		t.Fatalf("bad body 400: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "POST", "/subjects/"+f.mat+"/contents", `{}`, tok)
	if w.Code != 400 {
		t.Fatalf("sin título 400: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "PATCH", "/contents/no-uuid", `{"title":"X"}`, tok)
	if w.Code != 400 {
		t.Fatalf("uuid 400: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "PATCH", "/contents/"+created["id"], `{}`, tok)
	if w.Code != 400 {
		t.Fatalf("patch vacío 400: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "GET", "/contents/11111111-1111-4111-8111-111111111111", "", tok)
	if w.Code != 404 {
		t.Fatalf("inexistente 404: %d", w.Code)
	}
	w = f.do(t, "PATCH", "/contents/"+created["id"], `{invalid`, tok)
	if w.Code != 400 {
		t.Fatalf("patch bad body 400: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "DELETE", "/contents/no-uuid", "", tok)
	if w.Code != 400 {
		t.Fatalf("delete uuid 400: %d", w.Code)
	}
	w = f.do(t, "DELETE", "/contents/"+created["id"], "", tokB)
	if w.Code != 404 {
		t.Fatalf("delete ajeno 404: %d", w.Code)
	}
}

func TestGrades_Pinned_FlujoHTTP(t *testing.T) {
	f := newContentsFixture(t)
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	mk := func(name string) string {
		w := f.do(t, "POST", "/subjects/"+f.mat+"/grades",
			`{"name":"`+name+`","weight":0.5}`, tok)
		if w.Code != 201 {
			t.Fatalf("grade 201: %d %s", w.Code, w.Body.String())
		}
		var m map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &m)
		return m["id"]
	}
	idA, idB := mk("A"), mk("B")

	w := f.do(t, "PATCH", "/grades/"+idA, `{"pinned":true}`, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"pinned":true`) {
		t.Fatalf("pin A: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "PATCH", "/grades/"+idB, `{"pinned":true}`, tok)
	if w.Code != 200 {
		t.Fatalf("pin B: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "GET", "/subjects/"+f.mat+"/grades", "", tok)
	body := w.Body.String()
	if strings.Count(body, `"pinned":true`) != 1 || !strings.Contains(body, `"pinned":false`) {
		t.Fatalf("un solo pin: %s", body)
	}

	// Renombrar vía PATCH existente.
	w = f.do(t, "PATCH", "/grades/"+idA, `{"name":"A2"}`, tok)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"name":"A2"`) {
		t.Fatalf("rename: %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "PATCH", "/grades/"+idA, `{"name":""}`, tok)
	if w.Code != 400 {
		t.Fatalf("rename vacío 400: %d %s", w.Code, w.Body.String())
	}
}
