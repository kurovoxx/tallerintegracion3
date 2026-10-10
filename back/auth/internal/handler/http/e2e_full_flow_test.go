package http

// E2E: engine Gin montado como cmd/server/main.go con dependencias moqueadas.
// OAuth Google siempre moqueado (ningún test toca red externa).
// Flujos: register->login->me->refresh->logout y forgot->reset con código
// capturado del mock. Asserts de status + body en cada paso.

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
	"golang.org/x/crypto/bcrypt"
)

// --- Providers OAuth moqueados (sin red) ---

type e2eDriveProvider struct{}

func (e2eDriveProvider) Exchange(ctx context.Context, code string) (*service.DriveOAuthResult, error) {
	return e2eDriveExchange(code)
}

func (e2eDriveProvider) ExchangeWithRedirect(ctx context.Context, code, _ string) (*service.DriveOAuthResult, error) {
	return e2eDriveExchange(code)
}

func e2eDriveExchange(code string) (*service.DriveOAuthResult, error) {
	switch strings.TrimSpace(code) {
	case "valid-drive-code":
		exp := time.Now().Add(time.Hour)
		refresh := "e2e-drive-refresh"
		email := "drive@example.com"
		return &service.DriveOAuthResult{AccessToken: "e2e-drive-access", RefreshToken: &refresh, ExpiresAt: &exp, ExternalEmail: &email}, nil
	case "google-down":
		return nil, service.ErrGoogleUnavailable
	default:
		return nil, service.ErrInvalidOAuthCode
	}
}

type e2eCalendarProvider struct{}

func (e2eCalendarProvider) Exchange(ctx context.Context, code string) (*service.CalendarOAuthResult, error) {
	return e2eCalendarExchange(code)
}

func (e2eCalendarProvider) ExchangeWithRedirect(ctx context.Context, code, _ string) (*service.CalendarOAuthResult, error) {
	return e2eCalendarExchange(code)
}

func (e2eCalendarProvider) Refresh(ctx context.Context, _ string) (*service.CalendarOAuthResult, error) {
	exp := time.Now().Add(time.Hour)
	return &service.CalendarOAuthResult{AccessToken: "e2e-cal-refreshed", ExpiresAt: &exp}, nil
}

func e2eCalendarExchange(code string) (*service.CalendarOAuthResult, error) {
	switch strings.TrimSpace(code) {
	case "valid-cal-code":
		exp := time.Now().Add(time.Hour)
		refresh := "e2e-cal-refresh"
		email := "cal@example.com"
		return &service.CalendarOAuthResult{AccessToken: "e2e-cal-access", RefreshToken: &refresh, ExpiresAt: &exp, ExternalEmail: &email}, nil
	case "google-down":
		return nil, service.ErrGoogleUnavailable
	default:
		return nil, service.ErrInvalidOAuthCode
	}
}

// --- Repo OAuth en memoria (drive + calendar) ---

type e2eOAuthRepo struct {
	mu    sync.Mutex
	conns map[string]*model.OAuthConnection // key userID|provider
}

func newE2EOAuthRepo() *e2eOAuthRepo {
	return &e2eOAuthRepo{conns: map[string]*model.OAuthConnection{}}
}

func (m *e2eOAuthRepo) key(userID, provider string) string { return userID + "|" + provider }

func (m *e2eOAuthRepo) GetByUserIDAndProvider(_ context.Context, userID, provider string) (*model.OAuthConnection, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[m.key(userID, provider)]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (m *e2eOAuthRepo) UpsertGoogleDriveConnection(_ context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[m.key(userID, model.ProviderGoogleDrive)] = &model.OAuthConnection{
		UserID: userID, Provider: model.ProviderGoogleDrive,
		AccessToken: accessToken, RefreshToken: refreshToken,
		ExpiresAt: expiresAt, ExternalAccountEmail: externalEmail,
	}
	return nil
}

func (m *e2eOAuthRepo) UpdateGoogleDriveAccessToken(_ context.Context, userID, accessToken string, _ *string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[m.key(userID, model.ProviderGoogleDrive)]; ok {
		c.AccessToken = accessToken
		c.ExpiresAt = &expiresAt
	}
	return nil
}

func (m *e2eOAuthRepo) MarkGoogleDriveConnectionRevoked(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[m.key(userID, model.ProviderGoogleDrive)]; ok {
		now := time.Now()
		c.RevokedAt = &now
	}
	return nil
}

func (m *e2eOAuthRepo) DeleteGoogleDriveConnection(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.conns, m.key(userID, model.ProviderGoogleDrive))
	return nil
}

func (m *e2eOAuthRepo) UpsertGoogleCalendarConnection(_ context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns[m.key(userID, model.ProviderGoogleCalendar)] = &model.OAuthConnection{
		UserID: userID, Provider: model.ProviderGoogleCalendar,
		AccessToken: accessToken, RefreshToken: refreshToken,
		ExpiresAt: expiresAt, ExternalAccountEmail: externalEmail,
	}
	return nil
}

func (m *e2eOAuthRepo) UpdateGoogleCalendarAccessToken(_ context.Context, userID, accessToken string, _ *string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[m.key(userID, model.ProviderGoogleCalendar)]; ok {
		c.AccessToken = accessToken
		c.ExpiresAt = &expiresAt
	}
	return nil
}

func (m *e2eOAuthRepo) MarkGoogleCalendarConnectionRevoked(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.conns[m.key(userID, model.ProviderGoogleCalendar)]; ok {
		now := time.Now()
		c.RevokedAt = &now
	}
	return nil
}

// --- Reset store + mailer en memoria (captura código) ---

type e2eResetStore struct {
	mu     sync.Mutex
	hashes map[string]string // userID -> code hash
	users  *mockUserRepoH
}

func newE2EResetStore(users *mockUserRepoH) *e2eResetStore {
	return &e2eResetStore{hashes: map[string]string{}, users: users}
}

func (s *e2eResetStore) Issue(_ context.Context, userID, hash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hashes[userID] = hash
	return true, nil
}

func (s *e2eResetStore) Complete(_ context.Context, email, code, passwordHash string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, _ := s.users.GetByEmail(context.Background(), email)
	if u == nil {
		return false, nil
	}
	h, ok := s.hashes[u.ID]
	if !ok {
		return false, nil
	}
	if err := bcrypt.CompareHashAndPassword([]byte(h), []byte(code)); err != nil {
		return false, nil
	}
	// Simular UPDATE identity.users + revoke sessions + consumir código.
	u.PasswordHash = passwordHash
	delete(s.hashes, u.ID)
	return true, nil
}

type e2eResetMailer struct {
	mu    sync.Mutex
	codes map[string]string
}

func newE2EResetMailer() *e2eResetMailer { return &e2eResetMailer{codes: map[string]string{}} }

func (m *e2eResetMailer) Ready() bool { return true }

func (m *e2eResetMailer) SendResetCode(_ context.Context, email, code string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.codes[strings.ToLower(email)] = code
	return nil
}

func (m *e2eResetMailer) codeFor(email string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.codes[strings.ToLower(email)]
}

// --- Directorio de usuarios backed por mockUserRepoH ---

type e2eUserDirectory struct{ users *mockUserRepoH }

func (d *e2eUserDirectory) GetPublicByIDs(_ context.Context, ids []string) ([]model.PublicUser, error) {
	var out []model.PublicUser
	for _, id := range ids {
		for _, u := range d.users.users {
			if u.ID == id {
				out = append(out, model.PublicUser{UserID: u.ID, Email: u.Email})
			}
		}
	}
	if out == nil {
		out = []model.PublicUser{}
	}
	return out, nil
}

// --- Engine completo como main.go ---

type e2eFixture struct {
	engine      *gin.Engine
	users       *mockUserRepoH
	refresh     *mockRefreshH
	profiles    *mockProfileRepoHandler
	resetStore  *e2eResetStore
	resetMailer *e2eResetMailer
	oauthRepo   *e2eOAuthRepo
	jwt         *service.JWTService
}

func newE2EFixture(t *testing.T) *e2eFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("e2e-test-secret-32-chars-long!!"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	users := newMockUserRepoH()
	refresh := &mockRefreshH{tokens: map[string]*repository.RefreshToken{}}
	authSvc := service.NewAuthService(users, refresh, jwtSvc, 900, 604800)
	authH := NewAuthHandler(authSvc)

	resetStore := newE2EResetStore(users)
	resetMailer := newE2EResetMailer()
	resetH := NewPasswordResetHandler(service.NewPasswordResetService(users, resetStore, resetMailer))

	profiles := newMockRepoHandler()
	profileH := NewProfileHandler(service.NewProfileService(profiles))

	oauthRepo := newE2EOAuthRepo()
	driveH := NewDriveHandlerWithGoogleConfig(service.NewDriveOAuthService(oauthRepo, e2eDriveProvider{}), "e2e-client-id", "http://localhost/cb")
	calSvc := service.NewCalendarOAuthService(oauthRepo, e2eCalendarProvider{})
	calH := NewCalendarHandler(calSvc)
	internalOAuthH := NewInternalOAuthHandler(calSvc)
	internalUsersH := NewInternalUsersHandler(service.NewUsersService(&e2eUserDirectory{users: users}))

	mw := middleware.NewAuthMiddleware(jwtSvc)
	r := gin.New()
	r.POST("/auth/register", authH.Register)
	r.POST("/auth/login", authH.Login)
	r.POST("/auth/refresh", authH.Refresh)
	r.POST("/auth/logout", authH.Logout)
	r.POST("/auth/forgot-password", resetH.Forgot)
	r.POST("/auth/reset-password", resetH.Reset)
	r.POST("/auth/google-drive/connect", mw.RequireAuth(), driveH.Connect)
	r.GET("/auth/google-drive/status", mw.RequireAuth(), driveH.Status)
	r.DELETE("/auth/google-drive/connection", mw.RequireAuth(), driveH.Disconnect)
	r.POST("/auth/google-calendar/connect", mw.RequireAuth(), calH.Connect)
	r.GET("/auth/google-calendar/status", mw.RequireAuth(), calH.Status)

	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalKey("e2e-internal-key"))
	internal.GET("/oauth/calendar-token", internalOAuthH.GetCalendarToken)
	internal.POST("/oauth/calendar-revoked", internalOAuthH.ReportCalendarRevoked)
	internal.GET("/users/lookup", internalUsersH.Lookup)

	protected := r.Group("")
	protected.Use(mw.RequireAuth())
	protected.GET("/auth/me", func(c *gin.Context) {
		uid, _ := middleware.GetUserID(c)
		c.JSON(http.StatusOK, gin.H{"user_id": uid})
	})
	protected.GET("/profile/me", profileH.GetProfile)
	protected.PATCH("/profile/me", profileH.PatchProfile)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "auth-service"})
	})
	r.GET("/auth/google-config", driveH.GetGoogleConfig)

	return &e2eFixture{engine: r, users: users, refresh: refresh, profiles: profiles, resetStore: resetStore, resetMailer: resetMailer, oauthRepo: oauthRepo, jwt: jwtSvc}
}

func (f *e2eFixture) do(t *testing.T, method, path, body, token, internalKey string) *httptest.ResponseRecorder {
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
	if internalKey != "" {
		req.Header.Set("X-Internal-Key", internalKey)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func mustE2EJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("respuesta no es JSON: %v (%s)", err, w.Body.String())
	}
	return body
}

// Flujo feliz + errores: register -> login -> me -> refresh -> logout.
func TestE2E_RegisterLoginMeRefreshLogout(t *testing.T) {
	f := newE2EFixture(t)

	w := f.do(t, "POST", "/auth/register", `{"email":"e2e@test.invalid","password":"Pass1234"}`, "", "")
	if w.Code != 201 {
		t.Fatalf("register 201, got %d %s", w.Code, w.Body.String())
	}
	regBody := mustE2EJSON(t, w)
	userID, _ := regBody["id"].(string)
	if userID == "" {
		t.Fatalf("register sin id: %s", w.Body.String())
	}
	// Simular profile creado en la tx de register.
	f.profiles.store[userID] = &model.Profile{UserID: userID, DisplayName: "e2e", Visibility: "private"}

	// Duplicado -> 409 (sin forma de error distinta).
	w = f.do(t, "POST", "/auth/register", `{"email":"e2e@test.invalid","password":"Pass1234"}`, "", "")
	if w.Code != 409 {
		t.Fatalf("duplicado 409, got %d %s", w.Code, w.Body.String())
	}

	// Login inválido -> 401.
	w = f.do(t, "POST", "/auth/login", `{"email":"e2e@test.invalid","password":"WrongPass1"}`, "", "")
	if w.Code != 401 {
		t.Fatalf("login inválido 401, got %d %s", w.Code, w.Body.String())
	}

	w = f.do(t, "POST", "/auth/login", `{"email":"e2e@test.invalid","password":"Pass1234"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("login 200, got %d %s", w.Code, w.Body.String())
	}
	loginBody := mustE2EJSON(t, w)
	access, _ := loginBody["access_token"].(string)
	refreshTok, _ := loginBody["refresh_token"].(string)
	if access == "" || refreshTok == "" {
		t.Fatalf("tokens vacíos: %s", w.Body.String())
	}

	// /auth/me sin token -> 401, con token -> 200 coherente.
	w = f.do(t, "GET", "/auth/me", "", "", "")
	if w.Code != 401 {
		t.Fatalf("me sin token 401, got %d", w.Code)
	}
	w = f.do(t, "GET", "/auth/me", "", access, "")
	if w.Code != 200 {
		t.Fatalf("me 200, got %d %s", w.Code, w.Body.String())
	}
	if me := mustE2EJSON(t, w); me["user_id"] != userID {
		t.Fatalf("me user_id=%v esperado %s (%s)", me["user_id"], userID, w.Body.String())
	}

	// /profile/me con token -> 200.
	w = f.do(t, "GET", "/profile/me", "", access, "")
	if w.Code != 200 {
		t.Fatalf("profile/me 200, got %d %s", w.Code, w.Body.String())
	}

	// Refresh válido rota; reutilizar el viejo -> 401.
	w = f.do(t, "POST", "/auth/refresh", `{"refresh_token":"`+refreshTok+`"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("refresh 200, got %d %s", w.Code, w.Body.String())
	}
	newRefresh := mustE2EJSON(t, w)["refresh_token"].(string)
	if newRefresh == "" || newRefresh == refreshTok {
		t.Fatalf("refresh debe rotar: %s", w.Body.String())
	}
	w = f.do(t, "POST", "/auth/refresh", `{"refresh_token":"`+refreshTok+`"}`, "", "")
	if w.Code != 401 {
		t.Fatalf("refresh reutilizado 401, got %d %s", w.Code, w.Body.String())
	}

	// Logout idempotente 204 + refresh posterior 401.
	w = f.do(t, "POST", "/auth/logout", `{"refresh_token":"`+newRefresh+`"}`, "", "")
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("logout 204 sin body, got %d %q", w.Code, w.Body.String())
	}
	w = f.do(t, "POST", "/auth/logout", `{"refresh_token":"`+newRefresh+`"}`, "", "")
	if w.Code != 204 {
		t.Fatalf("logout repetido 204, got %d", w.Code)
	}
	w = f.do(t, "POST", "/auth/refresh", `{"refresh_token":"`+newRefresh+`"}`, "", "")
	if w.Code != 401 {
		t.Fatalf("refresh tras logout 401, got %d %s", w.Code, w.Body.String())
	}
}

// Flujo forgot -> reset con código capturado del mock.
func TestE2E_ForgotReset(t *testing.T) {
	f := newE2EFixture(t)
	w := f.do(t, "POST", "/auth/register", `{"email":"reset@test.invalid","password":"Pass1234"}`, "", "")
	if w.Code != 201 {
		t.Fatalf("register %d %s", w.Code, w.Body.String())
	}

	// Forgot a existente y a desconocido: ambos 200 con el mismo mensaje genérico.
	wKnown := f.do(t, "POST", "/auth/forgot-password", `{"email":"reset@test.invalid"}`, "", "")
	wUnknown := f.do(t, "POST", "/auth/forgot-password", `{"email":"nadie@test.invalid"}`, "", "")
	if wKnown.Code != 200 || wUnknown.Code != 200 {
		t.Fatalf("forgot 200 ambos, got %d y %d", wKnown.Code, wUnknown.Code)
	}
	if wKnown.Body.String() != wUnknown.Body.String() {
		t.Fatal("forgot no debe filtrar existencia de cuenta")
	}
	code := f.resetMailer.codeFor("reset@test.invalid")
	if len(code) != 8 {
		t.Fatalf("código capturado de 8 dígitos esperado, got %q", code)
	}

	// Reset con código inválido -> 400.
	w = f.do(t, "POST", "/auth/reset-password", `{"email":"reset@test.invalid","code":"00000000","password":"NewPass123"}`, "", "")
	if w.Code != 400 {
		t.Fatalf("reset inválido 400, got %d %s", w.Code, w.Body.String())
	}
	// Reset válido -> 200.
	w = f.do(t, "POST", "/auth/reset-password", `{"email":"reset@test.invalid","code":"`+code+`","password":"NewPass123"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("reset 200, got %d %s", w.Code, w.Body.String())
	}
	// Reuso del mismo código -> 400.
	w = f.do(t, "POST", "/auth/reset-password", `{"email":"reset@test.invalid","code":"`+code+`","password":"NewPass123"}`, "", "")
	if w.Code != 400 {
		t.Fatalf("reset reusado 400, got %d %s", w.Code, w.Body.String())
	}
	// Login con nueva clave OK, con vieja 401.
	w = f.do(t, "POST", "/auth/login", `{"email":"reset@test.invalid","password":"NewPass123"}`, "", "")
	if w.Code != 200 {
		t.Fatalf("login nueva clave 200, got %d %s", w.Code, w.Body.String())
	}
	w = f.do(t, "POST", "/auth/login", `{"email":"reset@test.invalid","password":"Pass1234"}`, "", "")
	if w.Code != 401 {
		t.Fatalf("login vieja clave 401, got %d %s", w.Code, w.Body.String())
	}
}

// OAuth moqueado + perfil + internas + health, con un error por grupo.
func TestE2E_OAuthPerfilInternasHealth(t *testing.T) {
	f := newE2EFixture(t)
	w := f.do(t, "POST", "/auth/register", `{"email":"oauth@test.invalid","password":"Pass1234"}`, "", "")
	if w.Code != 201 {
		t.Fatalf("register %d", w.Code)
	}
	userID := mustE2EJSON(t, w)["id"].(string)
	f.profiles.store[userID] = &model.Profile{UserID: userID, DisplayName: "oauth", Visibility: "private"}

	w = f.do(t, "POST", "/auth/login", `{"email":"oauth@test.invalid","password":"Pass1234"}`, "", "")
	access := mustE2EJSON(t, w)["access_token"].(string)

	// Drive: inválido 400, válido 200, status conectado, disconnect 204.
	if w := f.do(t, "POST", "/auth/google-drive/connect", `{"oauth_code":"bad"}`, access, ""); w.Code != 400 {
		t.Fatalf("drive inválido 400, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "POST", "/auth/google-drive/connect", `{"oauth_code":"valid-drive-code"}`, access, ""); w.Code != 200 {
		t.Fatalf("drive válido 200, got %d %s", w.Code, w.Body.String())
	} else if !strings.Contains(w.Body.String(), `"connected":true`) {
		t.Fatalf("drive connected=true, got %s", w.Body.String())
	}
	if w := f.do(t, "GET", "/auth/google-drive/status", "", access, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":true`) {
		t.Fatalf("drive status conectado, got %d %s", w.Code, w.Body.String())
	}
	// Evitar revoke HTTP real a Google (best-effort en prod): vaciar tokens
	// antes del DELETE; la fila existe igual y se verifica el borrado local.
	f.oauthRepo.mu.Lock()
	if c, ok := f.oauthRepo.conns[userID+"|"+model.ProviderGoogleDrive]; ok {
		c.AccessToken = ""
		c.RefreshToken = nil
	}
	f.oauthRepo.mu.Unlock()
	if w := f.do(t, "DELETE", "/auth/google-drive/connection", "", access, ""); w.Code != 204 {
		t.Fatalf("drive disconnect 204, got %d %s", w.Code, w.Body.String())
	}

	// Calendar: caído 502, válido 200, status con email.
	if w := f.do(t, "POST", "/auth/google-calendar/connect", `{"oauth_code":"google-down"}`, access, ""); w.Code != 502 {
		t.Fatalf("calendar caído 502, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "POST", "/auth/google-calendar/connect", `{"oauth_code":"valid-cal-code"}`, access, ""); w.Code != 200 {
		t.Fatalf("calendar válido 200, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/auth/google-calendar/status", "", access, ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"connected":true`) {
		t.Fatalf("calendar status, got %d %s", w.Code, w.Body.String())
	}

	// Perfil: visibility inválida 400, patch válido 200.
	if w := f.do(t, "PATCH", "/profile/me", `{"visibility":"invisible"}`, access, ""); w.Code != 400 {
		t.Fatalf("profile visibility 400, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "PATCH", "/profile/me", `{"display_name":"Nuevo"}`, access, ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Nuevo") {
		t.Fatalf("profile patch 200, got %d %s", w.Code, w.Body.String())
	}

	// Internas: sin key 401; con key lookup 200 ecalendar-token 200.
	if w := f.do(t, "GET", "/internal/users/lookup?ids="+userID, "", "", ""); w.Code != 401 {
		t.Fatalf("internal sin key 401, got %d", w.Code)
	}
	if w := f.do(t, "GET", "/internal/users/lookup?ids="+userID, "", "", "e2e-internal-key"); w.Code != 200 {
		t.Fatalf("internal lookup 200, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/internal/oauth/calendar-token?user_id="+userID, "", "", "e2e-internal-key"); w.Code != 200 || !strings.Contains(w.Body.String(), "access_token") {
		t.Fatalf("internal token 200, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/internal/oauth/calendar-token", "", "", "e2e-internal-key"); w.Code != 400 {
		t.Fatalf("internal sin user_id 400, got %d", w.Code)
	}

	// Health + google-config públicos.
	if w := f.do(t, "GET", "/health", "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"UP"`) {
		t.Fatalf("health 200 UP, got %d %s", w.Code, w.Body.String())
	}
	if w := f.do(t, "GET", "/auth/google-config", "", "", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "client_id") {
		t.Fatalf("google-config 200, got %d %s", w.Code, w.Body.String())
	}
	// Protegida con token inválido -> 401, coherencia resultado = función.
	if w := f.do(t, "GET", "/auth/me", "", "invalido", ""); w.Code != 401 {
		t.Fatalf("me inválido 401, got %d", w.Code)
	}
}
