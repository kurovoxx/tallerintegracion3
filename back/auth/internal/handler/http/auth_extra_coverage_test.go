package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// Reutiliza mocks de auth_handler_test.go (mockUserRepoH, mockRefreshH),
// drive_handler_test.go (mockDriveRepo, mockDriveProvider) y
// profile_handler_test.go (mockProfileRepoHandler) — mismo package.

func newCoverageJWT(t *testing.T) *service.JWTService {
	t.Helper()
	svc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("coverage-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	return svc
}

func doCoverageJSON(h func(c *gin.Context), method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h(c)
	return w
}

func assertCoverageError(t *testing.T, w *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("esperado %d, got %d %s", wantStatus, w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, w.Body.String())
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("esperado {error:{code,message}}, got %s", w.Body.String())
	}
	if errObj["code"] == "" || errObj["message"] == "" {
		t.Fatalf("error.code/message vacíos: %s", w.Body.String())
	}
}

// --- Login: payloads inválidos -> 400 ---

func TestCoverage_Login_JSONMalformado_400(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	w := doCoverageJSON(h.Login, "POST", "/auth/login", `{invalid json`)
	assertCoverageError(t, w, http.StatusBadRequest)
}

func TestCoverage_Login_SinCampos_400(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	for _, body := range []string{`{}`, `{"email":"a@b.cl"}`, `{"password":"Pass1234"}`, `{"email":"","password":""}`} {
		w := doCoverageJSON(h.Login, "POST", "/auth/login", body)
		assertCoverageError(t, w, http.StatusBadRequest)
	}
}

func TestCoverage_Login_EmailInexistente_401(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	w := doCoverageJSON(h.Login, "POST", "/auth/login", `{"email":"nadie@test.invalid","password":"Pass1234"}`)
	assertCoverageError(t, w, http.StatusUnauthorized)
	if !strings.Contains(w.Body.String(), "invalid_credentials") {
		t.Fatalf("se esperaba invalid_credentials, got %s", w.Body.String())
	}
}

// --- Register: huecos 400 ---

func TestCoverage_Register_JSONMalformado_400(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	w := doCoverageJSON(h.Register, "POST", "/auth/register", `{invalid`)
	assertCoverageError(t, w, http.StatusBadRequest)
}

func TestCoverage_Register_CamposFaltantes_400(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	for _, body := range []string{`{}`, `{"email":"a@b.cl"}`, `{"password":"Pass1234"}`} {
		w := doCoverageJSON(h.Register, "POST", "/auth/register", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: esperado 400, got %d %s", body, w.Code, w.Body.String())
		}
	}
}

func TestCoverage_Register_VisibilityInvalida_400(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	w := doCoverageJSON(h.Register, "POST", "/auth/register", `{"email":"vis@test.invalid","password":"Pass1234","visibility":"invisible"}`)
	assertCoverageError(t, w, http.StatusBadRequest)
	if !strings.Contains(w.Body.String(), "invalid_visibility") {
		t.Fatalf("se esperaba invalid_visibility, got %s", w.Body.String())
	}
}

// --- Refresh: huecos 400/401 ---

func TestCoverage_Refresh_JSONMalformado_400(t *testing.T) {
	svc := service.NewAuthService(newMockUserRepoH(), &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	w := doCoverageJSON(h.Refresh, "POST", "/auth/refresh", `{invalid`)
	assertCoverageError(t, w, http.StatusBadRequest)
}

func TestCoverage_Refresh_Expirado_401(t *testing.T) {
	repo := newMockUserRepoH()
	refreshRepo := &mockRefreshH{tokens: make(map[string]*repository.RefreshToken)}
	jwtSvc := newCoverageJWT(t)
	svc := service.NewAuthService(repo, refreshRepo, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	// register
	if w := doCoverageJSON(h.Register, "POST", "/auth/register", `{"email":"exp@test.invalid","password":"Pass1234"}`); w.Code != 201 {
		t.Fatalf("register %d %s", w.Code, w.Body.String())
	}
	user, _ := repo.GetByEmail(context.Background(), "exp@test.invalid")
	raw, hash, _ := repository.GenerateRawToken()
	if _, err := refreshRepo.Create(context.Background(), user.ID, hash, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"refresh_token": raw})
	w := doCoverageJSON(h.Refresh, "POST", "/auth/refresh", string(body))
	assertCoverageError(t, w, http.StatusUnauthorized)
}

func TestCoverage_Refresh_Rotado_Reutilizado_401(t *testing.T) {
	repo := newMockUserRepoH()
	refreshRepo := &mockRefreshH{tokens: make(map[string]*repository.RefreshToken)}
	svc := service.NewAuthService(repo, refreshRepo, newCoverageJWT(t), 900, 604800)
	h := NewAuthHandler(svc)
	if w := doCoverageJSON(h.Register, "POST", "/auth/register", `{"email":"rot@test.invalid","password":"Pass1234"}`); w.Code != 201 {
		t.Fatalf("register %d", w.Code)
	}
	wLogin := doCoverageJSON(h.Login, "POST", "/auth/login", `{"email":"rot@test.invalid","password":"Pass1234"}`)
	if wLogin.Code != 200 {
		t.Fatalf("login %d %s", wLogin.Code, wLogin.Body.String())
	}
	var lr map[string]any
	_ = json.Unmarshal(wLogin.Body.Bytes(), &lr)
	rt := lr["refresh_token"].(string)
	body, _ := json.Marshal(map[string]string{"refresh_token": rt})
	if w := doCoverageJSON(h.Refresh, "POST", "/auth/refresh", string(body)); w.Code != 200 {
		t.Fatalf("primer refresh 200, got %d", w.Code)
	}
	w2 := doCoverageJSON(h.Refresh, "POST", "/auth/refresh", string(body))
	assertCoverageError(t, w2, http.StatusUnauthorized)
}

// --- GET /auth/me montado como en main.go ---

func mountMeForCoverage(jwtSvc *service.JWTService) *gin.Engine {
	r := gin.New()
	mw := middleware.NewAuthMiddleware(jwtSvc)
	r.GET("/auth/me", mw.RequireAuth(), func(c *gin.Context) {
		uid, _ := middleware.GetUserID(c)
		c.JSON(http.StatusOK, gin.H{"user_id": uid})
	})
	return r
}

func TestCoverage_Me_ConToken_200(t *testing.T) {
	jwtSvc := newCoverageJWT(t)
	r := mountMeForCoverage(jwtSvc)
	tok, err := jwtSvc.GenerarAccessToken(service.UsuarioAutenticado{ID: "user-123"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body["user_id"] != "user-123" {
		t.Fatalf("user_id esperado, got %s", w.Body.String())
	}
}

func TestCoverage_Me_SinToken_401(t *testing.T) {
	r := mountMeForCoverage(newCoverageJWT(t))
	for _, hdr := range []string{"", "Bearer ", "Bearer invalido", "Token abc"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/auth/me", nil)
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		r.ServeHTTP(w, req)
		assertCoverageError(t, w, http.StatusUnauthorized)
	}
}

// --- GET /health (público, como en main.go) ---

func TestCoverage_Health_200(t *testing.T) {
	r := gin.New()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "auth-service"})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d", w.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "UP" || body["service"] != "auth-service" {
		t.Fatalf("payload inesperado %s", w.Body.String())
	}
}

// --- Rutas protegidas sin token -> 401 (tabla §1) ---

func TestCoverage_Protegidas_SinToken_401(t *testing.T) {
	jwtSvc := newCoverageJWT(t)
	mw := middleware.NewAuthMiddleware(jwtSvc)
	driveH := NewDriveHandler(service.NewDriveOAuthService(&mockDriveRepo{}, &mockDriveProvider{}))
	calH := NewCalendarHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{}))
	profRepo := newMockRepoHandler()
	profH := NewProfileHandler(service.NewProfileService(profRepo))

	r := gin.New()
	r.GET("/auth/google-drive/status", mw.RequireAuth(), driveH.Status)
	r.DELETE("/auth/google-drive/connection", mw.RequireAuth(), driveH.Disconnect)
	r.GET("/auth/google-calendar/status", mw.RequireAuth(), calH.Status)
	r.GET("/profile/me", mw.RequireAuth(), profH.GetProfile)
	r.PATCH("/profile/me", mw.RequireAuth(), profH.PatchProfile)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/auth/google-drive/status"},
		{"DELETE", "/auth/google-drive/connection"},
		{"GET", "/auth/google-calendar/status"},
		{"GET", "/profile/me"},
		{"PATCH", "/profile/me"},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: esperado 401, got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// --- Internas sin API key -> 401 (nunca abiertas) ---

func TestCoverage_Internas_SinKey_401(t *testing.T) {
	calSvc := service.NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{})
	oauthH := NewInternalOAuthHandler(calSvc)
	usersH := NewInternalUsersHandler(service.NewUsersService(&fakeUserDirectory{}))

	r := gin.New()
	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalKey("test-key"))
	internal.GET("/oauth/calendar-token", oauthH.GetCalendarToken)
	internal.POST("/oauth/calendar-revoked", oauthH.ReportCalendarRevoked)
	internal.GET("/users/lookup", usersH.Lookup)

	cases := []struct{ method, path, body string }{
		{"GET", "/internal/oauth/calendar-token?user_id=u1", ""},
		{"POST", "/internal/oauth/calendar-revoked", `{"user_id":"u1"}`},
		{"GET", "/internal/users/lookup?ids=u1", ""},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: esperado 401 sin key, got %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	// Con key correcta al menos no es 401 (lookup vacío -> 200 []).
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/internal/users/lookup", nil)
	req.Header.Set("X-Internal-Key", "test-key")
	r.ServeHTTP(w, req)
	if w.Code != 200 || w.Body.String() != "[]" {
		t.Fatalf("con key válida lookup vacío debe ser 200 [], got %d %s", w.Code, w.Body.String())
	}
}

// --- Drive Status handler: conectado / desconectado / sin user ---

func TestCoverage_DriveStatus_Conectado_Desconectado(t *testing.T) {
	now := time.Now()
	_ = now
	connected := &mockDriveRepo{connections: map[string]*model.OAuthConnection{
		"user-123": {AccessToken: "tok"},
	}}
	h := NewDriveHandler(service.NewDriveOAuthService(connected, &mockDriveProvider{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/google-drive/status", nil)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Status(c)
	if w.Code != 200 {
		t.Fatalf("conectado 200, got %d", w.Code)
	}
	var body map[string]bool
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || !body["connected"] {
		t.Fatalf("connected=true esperado, got %s", w.Body.String())
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control esperado, got %q", w.Header().Get("Cache-Control"))
	}
	if strings.Contains(w.Body.String(), "tok") {
		t.Fatal("status no debe exponer tokens")
	}

	h2 := NewDriveHandler(service.NewDriveOAuthService(&mockDriveRepo{}, &mockDriveProvider{}))
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/auth/google-drive/status", nil)
	c2.Set(middleware.ContextUserIDKey, "user-123")
	h2.Status(c2)
	if w2.Code != 200 {
		t.Fatalf("desconectado 200, got %d", w2.Code)
	}
	var body2 map[string]bool
	_ = json.Unmarshal(w2.Body.Bytes(), &body2)
	if body2["connected"] {
		t.Fatalf("connected=false esperado, got %s", w2.Body.String())
	}

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest("GET", "/auth/google-drive/status", nil)
	h2.Status(c3)
	assertCoverageError(t, w3, http.StatusUnauthorized)
}

// --- Concurrencia: doble registro simultáneo no duplica ---

type concUserRepo struct {
	mu    sync.Mutex
	users map[string]*model.User
}

func newConcUserRepo() *concUserRepo { return &concUserRepo{users: map[string]*model.User{}} }

func (m *concUserRepo) CreateUser(ctx context.Context, email, passwordHash, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[email]; ok {
		return nil, &mockErrH{msg: "email_taken: duplicate"}
	}
	u := &model.User{ID: "550e8400-e29b-41d4-a716-446655440001", Email: email, CreatedAt: time.Now()}
	m.users[email] = u
	return u, nil
}

func (m *concUserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if u, ok := m.users[strings.ToLower(strings.TrimSpace(email))]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *concUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.users[strings.ToLower(email)]
	return ok, nil
}

func TestCoverage_RegistroConcurrente_NoDuplica(t *testing.T) {
	repo := newConcUserRepo()
	svc := service.NewAuthService(repo, &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	const n = 2
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Register(context.Background(), "conc@test.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
			errs[i] = err
		}(i)
	}
	wg.Wait()
	ok, dup := 0, 0
	for _, err := range errs {
		if err == nil {
			ok++
			continue
		}
		if se, isSE := err.(*service.ServiceError); isSE && se.Code == "email_taken" {
			dup++
			continue
		}
		t.Fatalf("error inesperado: %v", err)
	}
	if ok != 1 || dup != n-1 {
		t.Fatalf("doble registro debe dar 1 éxito + %d email_taken, got ok=%d dup=%d errs=%v", n-1, ok, dup, errs)
	}
}

// --- PATCH /profile/me con token real via middleware: 401 expirado/inválido ---

func TestCoverage_Profile_ConMiddleware_TokenInvalido_401(t *testing.T) {
	jwtSvc := newCoverageJWT(t)
	profRepo := newMockRepoHandler()
	profH := NewProfileHandler(service.NewProfileService(profRepo))
	r := gin.New()
	r.GET("/profile/me", middleware.NewAuthMiddleware(jwtSvc).RequireAuth(), profH.GetProfile)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/profile/me", nil)
	req.Header.Set("Authorization", "Bearer token-invalido")
	r.ServeHTTP(w, req)
	assertCoverageError(t, w, http.StatusUnauthorized)
}
