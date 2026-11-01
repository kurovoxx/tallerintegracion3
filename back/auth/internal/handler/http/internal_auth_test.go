package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// JWT real sin DB: la verificación es solo criptográfica.
func testJWTSvc(t *testing.T, dur time.Duration) *service.JWTService {
	t.Helper()
	svc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("secreto-test-356"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     dur,
	})
	if err != nil {
		t.Fatalf("config JWT: %v", err)
	}
	return svc
}

func mintTestToken(t *testing.T, svc *service.JWTService, userID string) string {
	t.Helper()
	res, err := svc.GenerarAccessToken(service.UsuarioAutenticado{ID: userID})
	if err != nil {
		t.Fatalf("firmar token: %v", err)
	}
	return res.AccessToken
}

func TestInternalVerify_OK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := testJWTSvc(t, time.Hour)
	h := NewInternalAuthHandler(svc)
	tok := mintTestToken(t, svc, "u1")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/internal/auth/verify",
		strings.NewReader(`{"token":"`+tok+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Verify(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON inválido: %v", err)
	}
	if got["valid"] != true || got["user_id"] != "u1" {
		t.Fatalf("payload inesperado: %v", got)
	}
}

func TestInternalVerify_Rechazos(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := testJWTSvc(t, time.Hour)
	h := NewInternalAuthHandler(svc)
	call := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/internal/auth/verify", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.Verify(c)
		return w
	}

	if w := call(`{"token":"malo"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("token malo: esperado 401, got %d %s", w.Code, w.Body.String())
	}
	if w := call(`{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("sin token: esperado 400, got %d %s", w.Code, w.Body.String())
	}
	// Expirado: token de 1ns de vida.
	short := testJWTSvc(t, time.Nanosecond)
	time.Sleep(10 * time.Millisecond)
	expired := mintTestToken(t, short, "u1")
	w := call(`{"token":"` + expired + `"}`)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), "token_expired") {
		t.Fatalf("expirado: esperado 401 token_expired, got %d %s", w.Code, w.Body.String())
	}
}

func TestInternalUsers_GetByID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	name := "Ada"
	h := NewInternalUsersHandler(service.NewUsersService(&fakeUserDirectory{
		out: []model.PublicUser{{UserID: "u1", Email: "u1@example.com", DisplayName: &name}},
	}))
	call := func(id string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/internal/users/"+id, nil)
		c.Params = gin.Params{{Key: "id", Value: id}}
		h.GetByID(c)
		return w
	}

	w := call("u1")
	if w.Code != http.StatusOK {
		t.Fatalf("existente: esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var got model.PublicUser
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Email != "u1@example.com" {
		t.Fatalf("payload inesperado: %s err=%v", w.Body.String(), err)
	}

	// Repo vacío → 404.
	h404 := NewInternalUsersHandler(service.NewUsersService(&fakeUserDirectory{}))
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/internal/users/nope", nil)
	c2.Params = gin.Params{{Key: "id", Value: "nope"}}
	h404.GetByID(c2)
	if w2.Code != http.StatusNotFound || !strings.Contains(w2.Body.String(), "user_not_found") {
		t.Fatalf("inexistente: esperado 404 user_not_found, got %d %s", w2.Code, w2.Body.String())
	}
}

// Las rutas nuevas también exigen X-Internal-Key (nunca abiertas).
func TestInternal356_SinKey_401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := testJWTSvc(t, time.Hour)
	r := gin.New()
	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalKey("test-key"))
	internal.POST("/auth/verify", NewInternalAuthHandler(svc).Verify)
	internal.GET("/users/:id", NewInternalUsersHandler(service.NewUsersService(&fakeUserDirectory{})).GetByID)

	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/internal/auth/verify", `{"token":"x"}`},
		{"GET", "/internal/users/u1", ""},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: esperado 401 sin key, got %d", tc.method, tc.path, w.Code)
		}
	}
}
