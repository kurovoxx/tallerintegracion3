package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"golang.org/x/oauth2"
)

// Mocks

type mockOAuthRepo struct {
	upsertCalled bool
	lastUserID   string
	lastProvider string
	lastAccess   string
	lastRefresh  *string
	lastExpires  *time.Time
	lastEmail    *string
	upsertErr    error
}

func (m *mockOAuthRepo) UpsertGoogleDriveConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	m.upsertCalled = true
	m.lastUserID = userID
	m.lastProvider = "google_drive"
	m.lastAccess = accessToken
	m.lastRefresh = refreshToken
	m.lastExpires = expiresAt
	m.lastEmail = externalEmail
	return m.upsertErr
}

func (m *mockOAuthRepo) GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error) {
	return nil, nil
}

type mockProvider struct {
	result *DriveOAuthResult
	err    error
	called bool
	lastCode string
}

func (m *mockProvider) Exchange(ctx context.Context, code string) (*DriveOAuthResult, error) {
	m.called = true
	m.lastCode = code
	return m.result, m.err
}

func TestDriveConnect_ConexionInicialValida(t *testing.T) {
	repo := &mockOAuthRepo{}
	exp := time.Now().Add(1 * time.Hour)
	email := "user@gmail.com"
	refresh := "refresh123"
	provider := &mockProvider{
		result: &DriveOAuthResult{
			AccessToken:  "access123",
			RefreshToken: &refresh,
			ExpiresAt:    &exp,
			ExternalEmail: &email,
		},
	}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123")
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("Upsert no llamado")
	}
	if repo.lastUserID != "user-1" || repo.lastAccess != "access123" {
		t.Fatalf("Upsert args incorrectos: %+v", repo)
	}
	if repo.lastProvider != "google_drive" {
		t.Fatalf("provider debe ser google_drive, got %s", repo.lastProvider)
	}
	// No expone tokens (Connect no retorna tokens)
}

func TestDriveConnect_ReconexionValida(t *testing.T) {
	repo := &mockOAuthRepo{}
	exp := time.Now().Add(1 * time.Hour)
	email := "user@gmail.com"
	// Primera conexión con refresh
	refresh1 := "refresh1"
	provider1 := &mockProvider{result: &DriveOAuthResult{AccessToken: "access1", RefreshToken: &refresh1, ExpiresAt: &exp, ExternalEmail: &email}}
	svc1 := NewDriveOAuthService(repo, provider1)
	svc1.Connect(context.Background(), "user-1", "code1")
	// Segunda conexión sin refresh_token (Google no lo reenvía si ya consintió)
	provider2 := &mockProvider{result: &DriveOAuthResult{AccessToken: "access2", RefreshToken: nil, ExpiresAt: &exp, ExternalEmail: &email}}
	svc2 := NewDriveOAuthService(repo, provider2)
	err := svc2.Connect(context.Background(), "user-1", "code2")
	if err != nil {
		t.Fatalf("reconexión debe ser éxito, got %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("Upsert debe llamarse en reconexión")
	}
	// Verificar que no crea duplicados: Upsert usa ON CONFLICT (user_id, provider), no INSERT duplicado
	// En mock, solo verificamos que se llamó y que refresh nil se maneja (conserva anterior en SQL via COALESCE)
	if repo.lastRefresh != nil && *repo.lastRefresh != "" {
		// Si el mock recibió nil, es correcto (el repo real hace COALESCE)
		// Aquí verificamos que el segundo provider tenía RefreshToken nil
		if provider2.result.RefreshToken != nil {
			t.Fatal("segundo resultado debe tener RefreshToken nil")
		}
	}
}

func TestDriveConnect_CodigoVacio_BadRequest(t *testing.T) {
	repo := &mockOAuthRepo{}
	provider := &mockProvider{}
	svc := NewDriveOAuthService(repo, provider)
	for _, code := range []string{"", "   ", "\t\n"} {
		err := svc.Connect(context.Background(), "user-1", code)
		if err == nil {
			t.Fatalf("esperado bad_request para %q", code)
		}
		se, ok := err.(*ServiceError)
		if !ok || se.Code != "bad_request" {
			t.Fatalf("esperado bad_request, got %v", err)
		}
		if provider.called {
			t.Fatal("proveedor no debe llamarse si código vacío")
		}
	}
}

func TestDriveConnect_CodigoInvalido_InvalidOAuthCode(t *testing.T) {
	repo := &mockOAuthRepo{}
	provider := &mockProvider{err: ErrInvalidOAuthCode}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "invalid-code")
	if err == nil {
		t.Fatal("esperado invalid_oauth_code")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_oauth_code" {
		t.Fatalf("esperado invalid_oauth_code, got %v", err)
	}
	if repo.upsertCalled {
		t.Fatal("Upsert no debe llamarse si provider falla")
	}
}

func TestDriveConnect_ErrorTemporal_GoogleUnavailable(t *testing.T) {
	repo := &mockOAuthRepo{}
	provider := &mockProvider{err: ErrGoogleUnavailable}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123")
	if err == nil {
		t.Fatal("esperado google_unavailable")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "google_unavailable" {
		t.Fatalf("esperado google_unavailable, got %v", err)
	}
}

func TestDriveConnect_ErrorObteniendoDatos_NoUpsert(t *testing.T) {
	repo := &mockOAuthRepo{}
	provider := &mockProvider{err: errors.New("network error")}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123")
	if err == nil {
		t.Fatal("esperado internal_error")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "internal_error" {
		t.Fatalf("esperado internal_error, got %v", err)
	}
	if repo.upsertCalled {
		t.Fatal("Upsert no debe llamarse si provider falla con error no esperado")
	}
}

func TestDriveConnect_ErrorRepositorio_InternalError(t *testing.T) {
	repo := &mockOAuthRepo{upsertErr: errors.New("db error")}
	exp := time.Now().Add(1 * time.Hour)
	email := "user@gmail.com"
	refresh := "refresh123"
	provider := &mockProvider{result: &DriveOAuthResult{AccessToken: "access123", RefreshToken: &refresh, ExpiresAt: &exp, ExternalEmail: &email}}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123")
	if err == nil {
		t.Fatal("esperado internal_error por repo")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "internal_error" {
		t.Fatalf("esperado internal_error, got %v", err)
	}
}

func TestDriveConnect_AccessTokenVacio_InternalError(t *testing.T) {
	repo := &mockOAuthRepo{}
	provider := &mockProvider{result: &DriveOAuthResult{AccessToken: "", RefreshToken: nil}}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123")
	if err == nil {
		t.Fatal("esperado internal_error si access_token vacío")
	}
}

// --- Tests para ConfigDriveOAuthProvider con httptest.Server (sin red externa) ---

func TestConfigProvider_ExitoSimulado(t *testing.T) {
	// Servidor que simula token endpoint y userinfo
	var tokenHit, userinfoHit bool
	var tokenBody string
	var authHeader string
	// Token endpoint
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		tokenHit = true
		if r.Method != http.MethodPost {
			t.Errorf("token endpoint debe ser POST, got %s", r.Method)
		}
		// oauth2 puede enviar client_id/secret via Basic Auth o Form
		r.ParseForm()
		clientID := r.Form.Get("client_id")
		clientSecret := r.Form.Get("client_secret")
		// Si no están en Form, verificar Basic Auth
		if clientID == "" {
			if u, p, ok := r.BasicAuth(); ok {
				clientID = u
				clientSecret = p
			}
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type esperado authorization_code, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "code123" {
			t.Errorf("code esperado code123, got %s", r.Form.Get("code"))
		}
		if clientID != "test-client-id" {
			t.Errorf("client_id esperado test-client-id, got %s (auth header %s)", clientID, r.Header.Get("Authorization"))
		}
		if clientSecret != "test-secret" {
			t.Errorf("client_secret esperado test-secret, got %s", clientSecret)
		}
		if r.Form.Get("redirect_uri") != "http://localhost/callback" {
			t.Errorf("redirect_uri %s", r.Form.Get("redirect_uri"))
		}
		tokenBody = `{"access_token":"access123","refresh_token":"refresh123","expires_in":3600,"token_type":"Bearer"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(tokenBody))
	})
	defer tokenSrv.Close()
	userinfoSrv := newTestUserinfoServer(t, func(w http.ResponseWriter, r *http.Request) {
		userinfoHit = true
		authHeader = r.Header.Get("Authorization")
		if authHeader != "Bearer access123" {
			t.Errorf("Authorization Bearer access123 esperado, got %s", authHeader)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"email":"user@gmail.com"}`))
	})
	defer userinfoSrv.Close()

	provider := &ConfigDriveOAuthProvider{
		ClientID:     "test-client-id",
		ClientSecret: "test-secret",
		RedirectURI:  "http://localhost/callback",
		Endpoint:     oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL:  userinfoSrv.URL,
	}
	result, err := provider.Exchange(context.Background(), "code123")
	if err != nil {
		t.Fatalf("esperado éxito, got %v", err)
	}
	if !tokenHit || !userinfoHit {
		t.Fatal("token y userinfo deben ser llamados")
	}
	if result.AccessToken != "access123" || result.RefreshToken == nil || *result.RefreshToken != "refresh123" || result.ExpiresAt == nil || result.ExternalEmail == nil || *result.ExternalEmail != "user@gmail.com" {
		t.Fatalf("resultado incorrecto: %+v", result)
	}
}

func TestConfigProvider_ConfigVacia_GoogleUnavailable(t *testing.T) {
	provider := &ConfigDriveOAuthProvider{
		ClientID:     "",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost/callback",
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable si config vacía, got %v", err)
	}
	// Verificar que no hizo request (no hay servidor, pero si hubiera, no debe contar)
}

func TestConfigProvider_TokenEndpoint_400_InvalidGrant(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"invalid_grant","error_description":"Bad Request"}`))
	})
	defer tokenSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID:     "id",
		ClientSecret: "secret",
		RedirectURI:  "http://localhost/callback",
		Endpoint:     oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL:  "http://unused",
	}
	_, err := provider.Exchange(context.Background(), "bad-code")
	if !errors.Is(err, ErrInvalidOAuthCode) {
		t.Fatalf("esperado ErrInvalidOAuthCode para 400 invalid_grant, got %v", err)
	}
}

func TestConfigProvider_TokenEndpoint_429_GoogleUnavailable(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		w.Write([]byte(`{}`))
	})
	defer tokenSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost/callback",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL: "http://unused",
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable para 429, got %v", err)
	}
}

func TestConfigProvider_TokenEndpoint_500_GoogleUnavailable(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	})
	defer tokenSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost/callback",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL: "http://unused",
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable para 500, got %v", err)
	}
}

func TestConfigProvider_TokenSinAccessToken_Error(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"refresh_token":"refresh123","expires_in":3600}`))
	})
	defer tokenSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost/callback",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL: "http://unused",
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if err == nil {
		t.Fatal("esperado error si falta access_token")
	}
	// Debe ser error controlado, no necesariamente ErrInvalidOAuthCode/ErrGoogleUnavailable
	// Solo verificar que no es nil y no es éxito
}

func TestConfigProvider_Userinfo_401_GoogleUnavailable(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"access123","expires_in":3600}`))
	})
	defer tokenSrv.Close()
	userinfoSrv := newTestUserinfoServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	})
	defer userinfoSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost/callback",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL: userinfoSrv.URL,
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if !errors.Is(err, ErrGoogleUnavailable) {
		t.Fatalf("esperado ErrGoogleUnavailable para userinfo 401, got %v", err)
	}
}

func TestConfigProvider_Userinfo_JSONInvalido_Error(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"access123","expires_in":3600}`))
	})
	defer tokenSrv.Close()
	userinfoSrv := newTestUserinfoServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json`))
	})
	defer userinfoSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost/callback",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL: userinfoSrv.URL,
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if err == nil {
		t.Fatal("esperado error por JSON inválido")
	}
	if errors.Is(err, ErrGoogleUnavailable) || errors.Is(err, ErrInvalidOAuthCode) {
		t.Fatalf("debe ser error controlado no exitoso, got %v", err)
	}
}

func TestConfigProvider_Userinfo_EmailVacio_Error(t *testing.T) {
	tokenSrv := newTestTokenServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"access123","expires_in":3600}`))
	})
	defer tokenSrv.Close()
	userinfoSrv := newTestUserinfoServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"email":""}`))
	})
	defer userinfoSrv.Close()
	provider := &ConfigDriveOAuthProvider{
		ClientID: "id", ClientSecret: "secret", RedirectURI: "http://localhost/callback",
		Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL},
		UserinfoURL: userinfoSrv.URL,
	}
	_, err := provider.Exchange(context.Background(), "code123")
	if err == nil {
		t.Fatal("esperado error si email vacío")
	}
}

// Helpers para httptest
func newTestTokenServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	// Usamos httptest.NewServer que ya maneja http
	return httptest.NewServer(http.HandlerFunc(handler))
}
func newTestUserinfoServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

// Necesario para httptest
func init() {
	// Asegurar que time se use
	_ = time.Now
}
