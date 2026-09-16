package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

func init() { gin.SetMode(gin.TestMode) }

type mockDriveRepo struct {
	upsertCalled bool
	upsertErr    error
}

func (m *mockDriveRepo) UpsertGoogleDriveConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	m.upsertCalled = true
	return m.upsertErr
}
func (m *mockDriveRepo) GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error) {
	return nil, nil
}
func (m *mockDriveRepo) UpdateGoogleDriveAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error {
	return nil
}

type mockDriveProvider struct {
	result *service.DriveOAuthResult
	err    error
}

func (m *mockDriveProvider) Exchange(ctx context.Context, code string) (*service.DriveOAuthResult, error) {
	return m.result, m.err
}

func newDriveHandlerWithMocks(repo *mockDriveRepo, provider *mockDriveProvider) *DriveHandler {
	svc := service.NewDriveOAuthService(repo, provider)
	return NewDriveHandler(svc)
}

func newJWTForDriveTest(t *testing.T) *service.JWTService {
	t.Helper()
	svc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("drive-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	return svc
}

func newAuthMiddlewareForDrive(t *testing.T) *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(newJWTForDriveTest(t))
}

// Helper para crear contexto con user_id (simula RequireAuth)
func newDriveContext(token, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// Simular inyección de user_id sin pasar por middleware real, usando el mismo key que middleware usa
	// El middleware usa contextKeyUserID = "user_id" y Gin key "user_id"
	c.Set(middleware.ContextUserIDKey, "user-123")
	return c, w
}

// Test válido
func TestDriveHandler_Connect_Valido_200(t *testing.T) {
	repo := &mockDriveRepo{}
	exp := time.Now().Add(1 * time.Hour)
	email := "user@gmail.com"
	refresh := "refresh123"
	provider := &mockDriveProvider{result: &service.DriveOAuthResult{AccessToken: "access123", RefreshToken: &refresh, ExpiresAt: &exp, ExternalEmail: &email}}
	h := newDriveHandlerWithMocks(repo, provider)
	// Crear contexto con user_id
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{"oauth_code":"code123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["connected"] != true {
		t.Fatalf("connected true esperado, got %v", resp)
	}
	if !repo.upsertCalled {
		t.Fatal("Upsert debe llamarse")
	}
	// Verificar que no expone tokens
	if w.Body.String() != `{"connected":true}` {
		// Permitir espacios, pero no debe contener access_token
		if containsDrive(w.Body.String(), "access_token") || containsDrive(w.Body.String(), "refresh_token") || containsDrive(w.Body.String(), "external_account_email") {
			t.Fatal("respuesta no debe contener tokens ni email")
		}
	}
}

func containsDrive(s, substr string) bool {
	return len(s) >= len(substr) && func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()
}

func TestDriveHandler_Connect_BodyVacio_400(t *testing.T) {
	h := newDriveHandlerWithMocks(&mockDriveRepo{}, &mockDriveProvider{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 para {}, got %d", w.Code)
	}
}

func TestDriveHandler_Connect_CodigoVacio_400(t *testing.T) {
	h := newDriveHandlerWithMocks(&mockDriveRepo{}, &mockDriveProvider{})
	for _, body := range []string{`{"oauth_code":""}`, `{"oauth_code":"   "}`, `{"oauth_code":"\t\n"}`} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(middleware.ContextUserIDKey, "user-123")
		h.Connect(c)
		if w.Code != 400 {
			t.Fatalf("esperado 400 para %s, got %d", body, w.Code)
		}
	}
}

func TestDriveHandler_Connect_JSONMalformado_400(t *testing.T) {
	h := newDriveHandlerWithMocks(&mockDriveRepo{}, &mockDriveProvider{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{invalid`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 malformado, got %d", w.Code)
	}
}

func TestDriveHandler_Connect_InvalidOAuthCode_400(t *testing.T) {
	repo := &mockDriveRepo{}
	provider := &mockDriveProvider{err: service.ErrInvalidOAuthCode}
	h := newDriveHandlerWithMocks(repo, provider)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{"oauth_code":"bad"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 invalid_oauth_code, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Fatal("esperado error code")
	}
}

func TestDriveHandler_Connect_GoogleUnavailable_502(t *testing.T) {
	repo := &mockDriveRepo{}
	provider := &mockDriveProvider{err: service.ErrGoogleUnavailable}
	h := newDriveHandlerWithMocks(repo, provider)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{"oauth_code":"code123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 502 {
		t.Fatalf("esperado 502, got %d", w.Code)
	}
}

func TestDriveHandler_Connect_InternalError_500(t *testing.T) {
	repo := &mockDriveRepo{upsertErr: assertError("db error")}
	provider := &mockDriveProvider{result: &service.DriveOAuthResult{AccessToken: "access123"}}
	h := newDriveHandlerWithMocks(repo, provider)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{"oauth_code":"code123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 500 {
		t.Fatalf("esperado 500, got %d", w.Code)
	}
}

func assertError(msg string) error {
	return &testError{msg: msg}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestDriveHandler_Connect_SinAuth_401(t *testing.T) {
	// Sin user_id en contexto → 401
	repo := &mockDriveRepo{}
	provider := &mockDriveProvider{}
	h := newDriveHandlerWithMocks(repo, provider)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{"oauth_code":"code123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	// No Set user_id
	h.Connect(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401 sin auth, got %d", w.Code)
	}
}

func TestDriveHandler_Connect_NoExponeTokens(t *testing.T) {
	repo := &mockDriveRepo{}
	exp := time.Now().Add(1 * time.Hour)
	email := "user@gmail.com"
	refresh := "refresh123"
	provider := &mockDriveProvider{result: &service.DriveOAuthResult{AccessToken: "access123", RefreshToken: &refresh, ExpiresAt: &exp, ExternalEmail: &email}}
	h := newDriveHandlerWithMocks(repo, provider)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-drive/connect", bytes.NewBufferString(`{"oauth_code":"code123"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	body := w.Body.String()
	if containsDrive(body, "access_token") || containsDrive(body, "refresh_token") || containsDrive(body, "external_account_email") || containsDrive(body, "user@gmail.com") {
		t.Fatalf("respuesta no debe exponer tokens ni email, got %s", body)
	}
}

// Evitar imports no usados
var _ = repository.NewOAuthRepository
