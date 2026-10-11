package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
)

func init() { gin.SetMode(gin.TestMode) }

const (
	testSecret = "handler-test-secret-32-chars-long"
	testUserA  = "11111111-1111-4111-8111-111111111111"
	testUserB  = "22222222-2222-4222-8222-222222222222"
)

func mintToken(t *testing.T, userID, secret string, dur time.Duration) string {
	t.Helper()
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, middleware.Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    middleware.TokenIssuer,
			Audience:  jwt.ClaimStrings{middleware.TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(dur)),
		},
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type httpFixture struct {
	engine *gin.Engine
}

func newHTTPFixture() *httpFixture {
	cur, sub, sch := newServiceFixture()
	mw := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret:   []byte(testSecret),
		Issuer:   middleware.TokenIssuer,
		Audience: middleware.TokenAudience,
	})
	r := gin.New()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "academic-service"})
	})
	g := r.Group("")
	g.Use(mw.RequireAuth())
	curH := NewCurriculaHandler(service.NewCurriculumService(cur))
	subH := NewSubjectsHandler(service.NewSubjectService(sub))
	schH := NewSchedulesHandler(service.NewScheduleService(sch))
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
	return &httpFixture{engine: r}
}

func (f *httpFixture) do(t *testing.T, method, path, body, token string) *httptest.ResponseRecorder {
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

func decodeID(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["id"] == "" {
		t.Fatalf("201 sin {id}: %d %s", w.Code, w.Body.String())
	}
	return body["id"]
}

func TestHandler_SinToken_401(t *testing.T) {
	f := newHTTPFixture()
	if w := f.do(t, "GET", "/health", "", ""); w.Code != 200 {
		t.Fatalf("health pública 200, got %d", w.Code)
	}
	paths := []struct{ method, path, body string }{
		{"POST", "/curricula", `{"name":"X"}`},
		{"GET", "/curricula", ""},
		{"GET", "/curricula/11111111-1111-4111-8111-111111111111", ""},
		{"PATCH", "/curricula/11111111-1111-4111-8111-111111111111", `{"name":"X"}`},
		{"DELETE", "/curricula/11111111-1111-4111-8111-111111111111", ""},
		{"POST", "/curricula/11111111-1111-4111-8111-111111111111/subjects", `{"name":"X"}`},
		{"GET", "/curricula/11111111-1111-4111-8111-111111111111/subjects", ""},
		{"GET", "/subjects/11111111-1111-4111-8111-111111111111", ""},
		{"PATCH", "/subjects/11111111-1111-4111-8111-111111111111", `{"name":"X"}`},
		{"DELETE", "/subjects/11111111-1111-4111-8111-111111111111", ""},
		{"POST", "/subjects/11111111-1111-4111-8111-111111111111/schedules", `{"day_of_week":1,"start_time":"08:00","end_time":"09:00"}`},
		{"GET", "/subjects/11111111-1111-4111-8111-111111111111/schedules", ""},
		{"DELETE", "/schedules/11111111-1111-4111-8111-111111111111", ""},
	}
	for _, tc := range paths {
		if w := f.do(t, tc.method, tc.path, tc.body, ""); w.Code != 401 {
			t.Fatalf("%s %s: esperado 401, got %d", tc.method, tc.path, w.Code)
		}
	}
	// Token malformado y de otro secreto también 401.
	for _, tok := range []string{"invalido", mintToken(t, testUserA, "otro-secreto-32-chars-long-xxxx", time.Hour)} {
		if w := f.do(t, "GET", "/curricula", "", tok); w.Code != 401 {
			t.Fatalf("token malo: esperado 401, got %d %s", w.Code, w.Body.String())
		}
	}
}

func TestHandler_FlujoCompleto(t *testing.T) {
	f := newHTTPFixture()
	tok := mintToken(t, testUserA, testSecret, time.Hour)

	w := f.do(t, "POST", "/curricula", `{"name":"Malla"}`, tok)
	if w.Code != 201 {
		t.Fatalf("POST curricula 201, got %d %s", w.Code, w.Body.String())
	}
	curID := decodeID(t, w)

	if w := f.do(t, "GET", "/curricula", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), curID) {
		t.Fatalf("list contiene: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/curricula/"+curID, "", tok); w.Code != 200 {
		t.Fatalf("get 200, got %d", w.Code)
	}
	if w := f.do(t, "PATCH", "/curricula/"+curID, `{"name":"Malla v2"}`, tok); w.Code != 200 || !strings.Contains(w.Body.String(), "Malla v2") {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "POST", "/curricula/"+curID+"/subjects", `{"name":"Cálculo","status":"approved","average_decimal_places":1}`, tok)
	if w.Code != 201 {
		t.Fatalf("POST subject 201, got %d %s", w.Code, w.Body.String())
	}
	subID := decodeID(t, w)

	if w := f.do(t, "GET", "/curricula/"+curID+"/subjects", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), subID) {
		t.Fatalf("list subjects: %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/subjects/"+subID, "", tok); w.Code != 200 {
		t.Fatalf("get subject 200, got %d", w.Code)
	}
	if w := f.do(t, "PATCH", "/subjects/"+subID, `{"credits":10}`, tok); w.Code != 200 || !strings.Contains(w.Body.String(), "10") {
		t.Fatalf("patch subject: %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "POST", "/subjects/"+subID+"/schedules", `{"day_of_week":2,"start_time":"10:00","end_time":"11:30"}`, tok)
	if w.Code != 201 {
		t.Fatalf("POST schedule 201, got %d %s", w.Code, w.Body.String())
	}
	schID := decodeID(t, w)
	if w := f.do(t, "GET", "/subjects/"+subID+"/schedules", "", tok); w.Code != 200 || !strings.Contains(w.Body.String(), schID) {
		t.Fatalf("list schedules: %d %s", w.Code, w.Body.String())
	}

	if w := f.do(t, "DELETE", "/schedules/"+schID, "", tok); w.Code != 204 {
		t.Fatalf("delete schedule 204, got %d", w.Code)
	}
	if w := f.do(t, "DELETE", "/subjects/"+subID, "", tok); w.Code != 204 {
		t.Fatalf("delete subject 204, got %d", w.Code)
	}
	if w := f.do(t, "DELETE", "/curricula/"+curID, "", tok); w.Code != 204 {
		t.Fatalf("delete curricula 204, got %d", w.Code)
	}
	if w := f.do(t, "GET", "/curricula/"+curID, "", tok); w.Code != 404 {
		t.Fatalf("tras delete 404, got %d", w.Code)
	}
}

func TestHandler_CrossUser_404(t *testing.T) {
	f := newHTTPFixture()
	tokA := mintToken(t, testUserA, testSecret, time.Hour)
	tokB := mintToken(t, testUserB, testSecret, time.Hour)

	curID := decodeID(t, f.do(t, "POST", "/curricula", `{"name":"Privada"}`, tokA))
	subID := decodeID(t, f.do(t, "POST", "/curricula/"+curID+"/subjects", `{"name":"Secreta"}`, tokA))
	schID := decodeID(t, f.do(t, "POST", "/subjects/"+subID+"/schedules", `{"day_of_week":1,"start_time":"08:00","end_time":"09:00"}`, tokA))

	cases := []struct{ method, path, body string }{
		{"GET", "/curricula/" + curID, ""},
		{"PATCH", "/curricula/" + curID, `{"name":"Hack"}`},
		{"DELETE", "/curricula/" + curID, ""},
		{"POST", "/curricula/" + curID + "/subjects", `{"name":"Hack"}`},
		{"GET", "/curricula/" + curID + "/subjects", ""},
		{"GET", "/subjects/" + subID, ""},
		{"PATCH", "/subjects/" + subID, `{"name":"Hack"}`},
		{"DELETE", "/subjects/" + subID, ""},
		{"POST", "/subjects/" + subID + "/schedules", `{"day_of_week":1,"start_time":"08:00","end_time":"09:00"}`},
		{"GET", "/subjects/" + subID + "/schedules", ""},
		{"DELETE", "/schedules/" + schID, ""},
	}
	for _, tc := range cases {
		w := f.do(t, tc.method, tc.path, tc.body, tokB)
		if w.Code != 404 {
			t.Fatalf("B %s %s: esperado 404 (nunca 403), got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "forbidden") {
			t.Fatalf("B %s %s: no debe filtrar con 403", tc.method, tc.path)
		}
	}
	// A sigue viendo todo.
	if w := f.do(t, "GET", "/curricula/"+curID, "", tokA); w.Code != 200 {
		t.Fatalf("A get 200, got %d", w.Code)
	}
}

func TestHandler_Validaciones_400(t *testing.T) {
	f := newHTTPFixture()
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	curID := decodeID(t, f.do(t, "POST", "/curricula", `{"name":"M"}`, tok))
	subID := decodeID(t, f.do(t, "POST", "/curricula/"+curID+"/subjects", `{"name":"M"}`, tok))

	cases := []struct {
		name, method, path, body, wantCode string
	}{
		{"curricula nombre vacío", "POST", "/curricula", `{"name":""}`, "invalid_name"},
		{"curricula sin body", "POST", "/curricula", `{invalid`, "bad_request"},
		{"curricula uuid malo", "GET", "/curricula/no-uuid", "", "invalid_uuid"},
		{"subject sin name", "POST", "/curricula/" + curID + "/subjects", `{"credits":5}`, "invalid_name"},
		{"subject decimal 3", "POST", "/curricula/" + curID + "/subjects", `{"name":"X","average_decimal_places":3}`, "invalid_decimal_places"},
		{"subject status malo", "POST", "/curricula/" + curID + "/subjects", `{"name":"X","status":"zzz"}`, "invalid_status"},
		{"subject patch vacío", "PATCH", "/subjects/" + subID, `{}`, "bad_request"},
		{"subject malla ajena-uuid-mal", "POST", "/curricula/no-uuid/subjects", `{"name":"X"}`, "invalid_uuid"},
		{"schedule día 7", "POST", "/subjects/" + subID + "/schedules", `{"day_of_week":7,"start_time":"08:00","end_time":"09:00"}`, "invalid_day_of_week"},
		{"schedule día -1", "POST", "/subjects/" + subID + "/schedules", `{"day_of_week":-1,"start_time":"08:00","end_time":"09:00"}`, "invalid_day_of_week"},
		{"schedule end==start", "POST", "/subjects/" + subID + "/schedules", `{"day_of_week":1,"start_time":"09:00","end_time":"09:00"}`, "invalid_time_range"},
		{"schedule end<start", "POST", "/subjects/" + subID + "/schedules", `{"day_of_week":1,"start_time":"10:00","end_time":"09:00"}`, "invalid_time_range"},
		{"schedule hora mala", "POST", "/subjects/" + subID + "/schedules", `{"day_of_week":1,"start_time":"x","end_time":"09:00"}`, "invalid_time_format"},
		{"schedule sin día", "POST", "/subjects/" + subID + "/schedules", `{"start_time":"08:00","end_time":"09:00"}`, "invalid_day_of_week"},
		{"schedule uuid malo", "DELETE", "/schedules/no-uuid", "", "invalid_uuid"},
	}
	for _, tc := range cases {
		w := f.do(t, tc.method, tc.path, tc.body, tok)
		if w.Code != 400 || !strings.Contains(w.Body.String(), tc.wantCode) {
			t.Fatalf("%s: esperado 400 %s, got %d %s", tc.name, tc.wantCode, w.Code, w.Body.String())
		}
	}
}

func TestHandler_DoblePOST_Concurrente(t *testing.T) {
	f := newHTTPFixture()
	tok := mintToken(t, testUserA, testSecret, time.Hour)
	const n = 2
	codes := make([]int, n)
	ids := make([]string, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := f.do(t, "POST", "/curricula", `{"name":"Concurrente"}`, tok)
			codes[i] = w.Code
			if w.Code == 201 {
				var body map[string]string
				_ = json.Unmarshal(w.Body.Bytes(), &body)
				ids[i] = body["id"]
			}
		}(i)
	}
	wg.Wait()
	for _, c := range codes {
		if c != 201 {
			t.Fatalf("doble POST: esperado 2×201, got %v", codes)
		}
	}
	if ids[0] == "" || ids[0] == ids[1] {
		t.Fatalf("doble POST debe crear 2 recursos distintos, got %v", ids)
	}
}
