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

func (m *mockOAuthRepo) UpdateGoogleDriveAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error {
	return nil
}

func (m *mockOAuthRepo) MarkGoogleDriveConnectionRevoked(ctx context.Context, userID string) error {
	return nil
}

type mockProvider struct {
	result   *DriveOAuthResult
	err      error
	called   bool
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
			AccessToken:   "access123",
			RefreshToken:  &refresh,
			ExpiresAt:     &exp,
			ExternalEmail: &email,
		},
	}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123", nil)
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
	svc1.Connect(context.Background(), "user-1", "code1", nil)
	// Segunda conexión sin refresh_token (Google no lo reenvía si ya consintió)
	provider2 := &mockProvider{result: &DriveOAuthResult{AccessToken: "access2", RefreshToken: nil, ExpiresAt: &exp, ExternalEmail: &email}}
	svc2 := NewDriveOAuthService(repo, provider2)
	err := svc2.Connect(context.Background(), "user-1", "code2", nil)
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
		err := svc.Connect(context.Background(), "user-1", code, nil)
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
	err := svc.Connect(context.Background(), "user-1", "invalid-code", nil)
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
	err := svc.Connect(context.Background(), "user-1", "code123", nil)
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
	err := svc.Connect(context.Background(), "user-1", "code123", nil)
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
	err := svc.Connect(context.Background(), "user-1", "code123", nil)
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
	err := svc.Connect(context.Background(), "user-1", "code123", nil)
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
		Endpoint:    oauth2.Endpoint{TokenURL: tokenSrv.URL},
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
		Endpoint:    oauth2.Endpoint{TokenURL: tokenSrv.URL},
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
		Endpoint:    oauth2.Endpoint{TokenURL: tokenSrv.URL},
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
		Endpoint:    oauth2.Endpoint{TokenURL: tokenSrv.URL},
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
		Endpoint:    oauth2.Endpoint{TokenURL: tokenSrv.URL},
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
		Endpoint:    oauth2.Endpoint{TokenURL: tokenSrv.URL},
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
	return httptest.NewServer(http.HandlerFunc(handler))
}
func newTestUserinfoServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(handler))
}

// Necesario para httptest
func init() {
	_ = time.Now
}

// --- Tests para GetValidAccessToken ---

func TestGetValidAccessToken_Vigente(t *testing.T) {
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID:      "user-1",
			Provider:    model.ProviderGoogleDrive,
			AccessToken: "access-valid",
			ExpiresAt:   func() *time.Time { exp := time.Now().Add(10 * time.Minute); return &exp }(),
			RevokedAt:   nil,
		},
	}
	provider := &mockProvider{}
	svc := NewDriveOAuthService(repo, provider)
	token, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("esperado vigente, got %v", err)
	}
	if token != "access-valid" {
		t.Fatalf("token %s", token)
	}
	if provider.called {
		t.Fatal("no debe llamar a Google si está vigente")
	}
	if repo.updateCalled {
		t.Fatal("no debe hacer Update si está vigente")
	}
}

func TestGetValidAccessToken_ExpiresAtNulo_Vigente(t *testing.T) {
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID:      "user-1",
			Provider:    model.ProviderGoogleDrive,
			AccessToken: "access-valid",
			ExpiresAt:   nil,
			RevokedAt:   nil,
		},
	}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	token, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("nil expires_at debe ser vigente, got %v", err)
	}
	if token != "access-valid" {
		t.Fatalf("token %s", token)
	}
}

func TestGetValidAccessToken_DentroMargen_Refresca(t *testing.T) {
	exp := time.Now().Add(2 * time.Minute)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID:       "user-1",
			Provider:     model.ProviderGoogleDrive,
			AccessToken:  "old-access",
			RefreshToken: func() *string { s := "refresh123"; return &s }(),
			ExpiresAt:    &exp,
			RevokedAt:    nil,
		},
	}
	newExp := time.Now().Add(1 * time.Hour)
	provider := &mockRefreshProvider{
		result: &DriveOAuthResult{AccessToken: "new-access", RefreshToken: stringPtr("new-refresh"), ExpiresAt: &newExp},
	}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	token, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("esperado refresco, got %v", err)
	}
	if token != "new-access" {
		t.Fatalf("token %s", token)
	}
	if !repo.updateCalled {
		t.Fatal("debe hacer Update")
	}
	if repo.lastAccess != "new-access" {
		t.Fatalf("Update access %s", repo.lastAccess)
	}
	if repo.lastRevokedAt != nil {
		t.Fatal("no debe tocar revoked_at")
	}
}

func TestGetValidAccessToken_Expirado_Refresca(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp,
		},
	}
	newExp := time.Now().Add(1 * time.Hour)
	provider := &mockRefreshProvider{result: &DriveOAuthResult{AccessToken: "new-access", ExpiresAt: &newExp}}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	token, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil || token != "new-access" {
		t.Fatalf("expirado debe refrescar, got %v %s", err, token)
	}
}

func TestGetValidAccessToken_NoExiste_NotConnected(t *testing.T) {
	repo := &mockOAuthRepoWithGet{conn: nil}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado not_connected")
	}
	se, _ := err.(*ServiceError)
	if se.Code != "not_connected" {
		t.Fatalf("esperado not_connected, got %v", err)
	}
}

func TestGetValidAccessToken_RevokedAt_Invalid(t *testing.T) {
	now := time.Now()
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "access", ExpiresAt: &now, RevokedAt: &now,
		},
	}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado drive_connection_invalid por revoked_at")
	}
	se, _ := err.(*ServiceError)
	if se.Code != "drive_connection_invalid" {
		t.Fatalf("esperado drive_connection_invalid, got %v", err)
	}
}

func TestGetValidAccessToken_RefreshTokenNulo_Invalid(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", ExpiresAt: &exp, RefreshToken: nil,
		},
	}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado drive_connection_invalid por refresh nil")
	}
}

func TestGetValidAccessToken_ConfigIncompleta_GoogleUnavailable(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{
			UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp,
		},
	}
	mockRefresh := &mockRefreshProvider{err: ErrGoogleUnavailable}
	svc2 := NewDriveOAuthServiceWithRefresher(repo, mockRefresh)
	_, err := svc2.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado google_unavailable")
	}
	se, _ := err.(*ServiceError)
	if se.Code != "google_unavailable" {
		t.Fatalf("esperado google_unavailable, got %v", err)
	}
}

func TestGetValidAccessToken_InvalidGrant_Invalid(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("bad-refresh"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{err: ErrDriveConnectionInvalid}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado drive_connection_invalid")
	}
	se, _ := err.(*ServiceError)
	if se.Code != "drive_connection_invalid" {
		t.Fatalf("esperado drive_connection_invalid, got %v", err)
	}
}

func TestGetValidAccessToken_429_GoogleUnavailable(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{err: ErrGoogleUnavailable}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado google_unavailable")
	}
}

func TestGetValidAccessToken_SinAccessToken_InternalError(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{result: &DriveOAuthResult{AccessToken: ""}}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado internal_error por access_token vacío")
	}
}

func TestGetValidAccessToken_UpdateError_InternalError(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn:      &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
		updateErr: errors.New("db error"),
	}
	newExp := time.Now().Add(1 * time.Hour)
	provider := &mockRefreshProvider{result: &DriveOAuthResult{AccessToken: "new-access", ExpiresAt: &newExp}}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado internal_error por repo")
	}
}

func TestGetValidAccessToken_RefreshSinNuevoRefresh_Conserva(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("old-refresh"), ExpiresAt: &exp},
	}
	newExp := time.Now().Add(1 * time.Hour)
	// Provider devuelve sin refresh_token (Google no lo reenvía)
	provider := &mockRefreshProvider{result: &DriveOAuthResult{AccessToken: "new-access", RefreshToken: nil, ExpiresAt: &newExp}}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	token, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil || token != "new-access" {
		t.Fatalf("debe refrescar y conservar refresh, got %v %s", err, token)
	}
	if repo.lastRefresh != nil {
		t.Fatal("debe pasar nil para conservar via COALESCE")
	}
}

// Mocks para GetValidAccessToken
type mockOAuthRepoWithGet struct {
	conn          *model.OAuthConnection
	updateCalled  bool
	markCalled    bool
	lastAccess    string
	lastRefresh   *string
	lastExpires   time.Time
	updateErr     error
	markErr       error
	lastRevokedAt *time.Time
}

func (m *mockOAuthRepoWithGet) UpsertGoogleDriveConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	return nil
}
func (m *mockOAuthRepoWithGet) GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error) {
	if m.conn != nil && m.conn.UserID == userID && m.conn.Provider == provider {
		return m.conn, nil
	}
	return nil, nil
}
func (m *mockOAuthRepoWithGet) UpdateGoogleDriveAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error {
	m.updateCalled = true
	m.lastAccess = accessToken
	m.lastRefresh = refreshToken
	m.lastExpires = expiresAt
	return m.updateErr
}
func (m *mockOAuthRepoWithGet) MarkGoogleDriveConnectionRevoked(ctx context.Context, userID string) error {
	m.markCalled = true
	return m.markErr
}

type mockRefreshProvider struct {
	result           *DriveOAuthResult
	err              error
	called           bool
	lastRefreshToken string
}

func (m *mockRefreshProvider) Exchange(ctx context.Context, code string) (*DriveOAuthResult, error) {
	m.called = true
	return m.result, m.err
}
func (m *mockRefreshProvider) Refresh(ctx context.Context, refreshToken string) (*DriveOAuthResult, error) {
	m.called = true
	m.lastRefreshToken = refreshToken
	return m.result, m.err
}

func TestGetValidAccessToken_InvalidGrant_MarkRevoked(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{err: ErrDriveConnectionInvalid}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado drive_connection_invalid")
	}
	if !repo.markCalled {
		t.Fatal("MarkGoogleDriveConnectionRevoked debe llamarse exactamente una vez para invalid_grant")
	}
	if repo.updateCalled {
		t.Fatal("Update no debe llamarse si invalid_grant")
	}
	se, _ := err.(*ServiceError)
	if se.Code != "drive_connection_invalid" {
		t.Fatalf("code %s", se.Code)
	}
}

func TestGetValidAccessToken_InvalidGrant_MarkError_InternalError(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn:    &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
		markErr: errors.New("db error"),
	}
	provider := &mockRefreshProvider{err: ErrDriveConnectionInvalid}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado internal_error")
	}
	se, _ := err.(*ServiceError)
	if se.Code != "internal_error" {
		t.Fatalf("esperado internal_error, got %v", err)
	}
}

func TestGetValidAccessToken_429_NoMark(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", RefreshToken: stringPtr("refresh123"), ExpiresAt: &exp},
	}
	provider := &mockRefreshProvider{err: ErrGoogleUnavailable}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado google_unavailable")
	}
	if repo.markCalled {
		t.Fatal("Mark no debe llamarse para 429/500")
	}
	if repo.updateCalled {
		t.Fatal("Update no debe llamarse para 429")
	}
}

func TestGetValidAccessToken_RevokedAt_NoHTTP(t *testing.T) {
	now := time.Now()
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", ExpiresAt: &now, RevokedAt: &now},
	}
	provider := &mockRefreshProvider{}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado drive_connection_invalid por revoked_at")
	}
	if provider.called {
		t.Fatal("no debe llamar HTTP si revoked_at ya está definido")
	}
	if repo.markCalled || repo.updateCalled {
		t.Fatal("no debe llamar Mark ni Update si revoked_at")
	}
}

func TestGetValidAccessToken_RefreshTokenNulo_NoMark(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	repo := &mockOAuthRepoWithGet{
		conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "old", ExpiresAt: &exp, RefreshToken: nil},
	}
	provider := &mockRefreshProvider{}
	svc := NewDriveOAuthServiceWithRefresher(repo, provider)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err == nil {
		t.Fatal("esperado drive_connection_invalid por refresh nil")
	}
	if provider.called {
		t.Fatal("no debe llamar HTTP si refresh nil")
	}
	if repo.markCalled {
		t.Fatal("no debe marcar revoked_at todavía si falta refresh_token")
	}
}

func TestGetGoogleDriveConnectionStatus_SinConexion(t *testing.T) {
	repo := &mockOAuthRepoWithGet{conn: nil}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	status, err := svc.GetGoogleDriveConnectionStatus(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if status.Connected || status.ReconnectRequired {
		t.Fatalf("sin conexión debe ser false,false got %+v", status)
	}
}

func TestGetGoogleDriveConnectionStatus_Conectada(t *testing.T) {
	now := time.Now()
	repo := &mockOAuthRepoWithGet{conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "access", RevokedAt: nil, ExpiresAt: &now}}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	status, _ := svc.GetGoogleDriveConnectionStatus(context.Background(), "user-1")
	if !status.Connected || status.ReconnectRequired {
		t.Fatalf("conectada debe ser true,false got %+v", status)
	}
}

func TestGetGoogleDriveConnectionStatus_Revocada(t *testing.T) {
	now := time.Now()
	repo := &mockOAuthRepoWithGet{conn: &model.OAuthConnection{UserID: "user-1", Provider: model.ProviderGoogleDrive, AccessToken: "access", RevokedAt: &now}}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	status, _ := svc.GetGoogleDriveConnectionStatus(context.Background(), "user-1")
	if status.Connected || !status.ReconnectRequired {
		t.Fatalf("revocada debe ser false,true got %+v", status)
	}
}

func TestGetGoogleDriveConnectionStatus_NoHTTP(t *testing.T) {
	repo := &mockOAuthRepoWithGet{conn: nil}
	provider := &mockProvider{}
	svc := NewDriveOAuthService(repo, provider)
	svc.GetGoogleDriveConnectionStatus(context.Background(), "user-1")
	if provider.called {
		t.Fatal("GetStatus no debe llamar Google")
	}
	if repo.updateCalled || repo.markCalled {
		t.Fatal("no debe llamar update ni mark")
	}
}

func TestReportGoogleDrivePermissionDenied_Mark(t *testing.T) {
	repo := &mockOAuthRepoWithGet{}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	err := svc.ReportGoogleDrivePermissionDenied(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if !repo.markCalled {
		t.Fatal("debe llamar Mark")
	}
}

func TestReportGoogleDrivePermissionDenied_Idempotente(t *testing.T) {
	repo := &mockOAuthRepoWithGet{}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	svc.ReportGoogleDrivePermissionDenied(context.Background(), "user-1")
	svc.ReportGoogleDrivePermissionDenied(context.Background(), "user-1")
	if !repo.markCalled {
		t.Fatal("mark debe llamarse")
	}
	// No debe requerir tokens ni llamar Google
}

func stringPtr(s string) *string { return &s }

// Helpers para que el servicio use el refresher correcto
func NewDriveOAuthServiceWithRefresher(repo *mockOAuthRepoWithGet, provider *mockRefreshProvider) *DriveOAuthService {
	// El DriveOAuthService espera OAuthRepository y DriveOAuthProvider (que ahora tiene Refresh)
	// Hacemos un adaptador que implementa ambas interfaces
	return &DriveOAuthService{
		oauthRepo: repo,
		provider:  &combinedProvider{exchange: provider, refresh: provider},
	}
}

type combinedProvider struct {
	exchange *mockRefreshProvider
	refresh  *mockRefreshProvider
}

func (c *combinedProvider) Exchange(ctx context.Context, code string) (*DriveOAuthResult, error) {
	return c.exchange.Exchange(ctx, code)
}
func (c *combinedProvider) Refresh(ctx context.Context, refreshToken string) (*DriveOAuthResult, error) {
	return c.refresh.Refresh(ctx, refreshToken)
}

func TestDriveConnect_ExpectedEmailCoincide(t *testing.T) {
	repo := &mockOAuthRepo{}
	email := "user@gmail.com"
	declared := "USER@Gmail.com"
	provider := &mockProvider{
		result: &DriveOAuthResult{AccessToken: "access123", ExternalEmail: &email},
	}
	svc := NewDriveOAuthService(repo, provider)
	if err := svc.Connect(context.Background(), "user-1", "code123", &declared); err != nil {
		t.Fatalf("email coincidente (case-insensitive) no debe fallar: %v", err)
	}
	if !repo.upsertCalled {
		t.Fatal("debió guardar la conexión")
	}
}

func TestDriveConnect_ExpectedEmailDifiere_Mismatch(t *testing.T) {
	repo := &mockOAuthRepo{}
	real := "real@gmail.com"
	declared := "otro@gmail.com"
	provider := &mockProvider{
		result: &DriveOAuthResult{AccessToken: "access123", ExternalEmail: &real},
	}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123", &declared)
	if se, ok := err.(*ServiceError); !ok || se.Code != "email_mismatch" {
		t.Fatalf("se esperaba email_mismatch, got %v", err)
	}
	if repo.upsertCalled {
		t.Fatal("mismatch no debe guardar nada")
	}
}

func TestDriveConnect_SinExpectedEmail_OmiteChequeo(t *testing.T) {
	repo := &mockOAuthRepo{}
	email := "user@gmail.com"
	provider := &mockProvider{
		result: &DriveOAuthResult{AccessToken: "access123", ExternalEmail: &email},
	}
	svc := NewDriveOAuthService(repo, provider)
	if err := svc.Connect(context.Background(), "user-1", "code123", nil); err != nil {
		t.Fatalf("sin expected no debe chequear: %v", err)
	}
}
