package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"golang.org/x/crypto/bcrypt"
)

func init() { gin.SetMode(gin.TestMode) }

// mocks para handler (reutiliza lógica de service test)

type mockUserRepoH struct {
	users map[string]*model.User
}

func newMockUserRepoH() *mockUserRepoH {
	return &mockUserRepoH{users: make(map[string]*model.User)}
}

func (m *mockUserRepoH) CreateUser(ctx context.Context, email, passwordHash, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, ok := m.users[email]; ok {
		return nil, &mockErrH{msg: "email_taken: duplicate"}
	}
	u := &model.User{
		ID:           "550e8400-e29b-41d4-a716-446655440001",
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}
	// ID único por email para distinguir
	if email != "handler@test.invalid" {
		u.ID = "660e8400-e29b-41d4-a716-446655440002"
		if strings.Contains(email, "dup") {
			u.ID = "770e8400-e29b-41d4-a716-446655440003"
		}
	}
	m.users[email] = u
	return u, nil
}

func (m *mockUserRepoH) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if u, ok := m.users[email]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockUserRepoH) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	_, ok := m.users[strings.ToLower(email)]
	return ok, nil
}

type mockErrH struct{ msg string }

func (e *mockErrH) Error() string { return e.msg }

type mockRefreshH struct {
	tokens map[string]*repository.RefreshToken
}

func newMockRefreshHWithStore() *mockRefreshH {
	return &mockRefreshH{tokens: make(map[string]*repository.RefreshToken)}
}

func (m *mockRefreshH) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (string, error) {
	if m.tokens == nil {
		m.tokens = make(map[string]*repository.RefreshToken)
	}
	id := fmt.Sprintf("mock-id-%d", len(m.tokens)+1)
	m.tokens[id] = &repository.RefreshToken{
		ID:        id,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
		Revoked:   false,
		CreatedAt: time.Now(),
	}
	return id, nil
}
func (m *mockRefreshH) CountByUser(ctx context.Context, userID string) (int, error) {
	c := 0
	for _, t := range m.tokens {
		if t.UserID == userID {
			c++
		}
	}
	return c, nil
}
func (m *mockRefreshH) FindByRawToken(ctx context.Context, raw string) (*repository.RefreshToken, error) {
	for _, t := range m.tokens {
		if err := bcrypt.CompareHashAndPassword([]byte(t.TokenHash), []byte(raw)); err == nil {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *mockRefreshH) Revoke(ctx context.Context, id string) error {
	if t, ok := m.tokens[id]; ok {
		t.Revoked = true
	}
	return nil
}
func (m *mockRefreshH) Rotate(ctx context.Context, oldID, userID, newHash string, newExpires time.Time) (string, error) {
	if old, ok := m.tokens[oldID]; ok {
		if old.Revoked {
			return "", fmt.Errorf("revoked")
		}
		if time.Now().After(old.ExpiresAt) {
			return "", fmt.Errorf("expired")
		}
		old.Revoked = true
	}
	newID := fmt.Sprintf("mock-id-%d", len(m.tokens)+1)
	m.tokens[newID] = &repository.RefreshToken{
		ID:        newID,
		UserID:    userID,
		TokenHash: newHash,
		ExpiresAt: newExpires,
		Revoked:   false,
		CreatedAt: time.Now(),
	}
	return newID, nil
}

func newJWTForHandler(t *testing.T) *service.JWTService {
	t.Helper()
	svc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	return svc
}



// Test 1: Register exitoso sin role
func TestAuthHandler_Register_SinRole_201(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)

	body, _ := json.Marshal(map[string]string{"email": "handler@test.invalid", "password": "Pass1234"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.Register(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("esperado 201, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["email"] != "handler@test.invalid" {
		t.Fatalf("email %v", resp["email"])
	}
	if _, hasRole := resp["role"]; hasRole {
		t.Fatal("respuesta no debe contener role")
	}
	if resp["id"] == "" {
		t.Fatal("id vacío")
	}
}

// Test 2: Register no exige role (body sin role debe funcionar)
func TestAuthHandler_Register_NoExigeRole(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)

	// Enviar solo email/password, sin role ni display_name
	body, _ := json.Marshal(map[string]string{"email": "norole@test.invalid", "password": "Pass1234"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != 201 {
		t.Fatalf("sin role debe dar 201, got %d %s", w.Code, w.Body.String())
	}
}

// Test 3: Register no devuelve role (ya verificado arriba, pero explícito)
func TestAuthHandler_Register_NoDevuelveRole(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	body, _ := json.Marshal(map[string]string{"email": "noreturn@test.invalid", "password": "Pass1234"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["role"]; ok {
		t.Fatal("no debe devolver role")
	}
}

// Test 4: Email duplicado 409
func TestAuthHandler_Register_Duplicado_409(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	body, _ := json.Marshal(map[string]string{"email": "dup@test.invalid", "password": "Pass1234"})
	// primer registro
	w1 := httptest.NewRecorder()
	c1, _ := gin.CreateTestContext(w1)
	c1.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c1.Request.Header.Set("Content-Type", "application/json")
	h.Register(c1)
	if w1.Code != 201 {
		t.Fatalf("primer registro 201, got %d", w1.Code)
	}
	// segundo con mismo email
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Register(c2)
	if w2.Code != 409 {
		t.Fatalf("duplicado debe ser 409, got %d %s", w2.Code, w2.Body.String())
	}
	var errResp map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &errResp)
	if _, ok := errResp["error"]; !ok {
		t.Fatal("esperado {error:{code}} en 409")
	}
}

// Test 5: Email inválido 400
func TestAuthHandler_Register_EmailInvalido_400(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	body, _ := json.Marshal(map[string]string{"email": "no-email", "password": "Pass1234"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d", w.Code)
	}
}

// Test 6: Contraseña inválida 400
func TestAuthHandler_Register_PasswordInvalida_400(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	body, _ := json.Marshal(map[string]string{"email": "valid@test.invalid", "password": "123"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d", w.Code)
	}
}

// Test 7: Login válido sin role genera JWT sin role
func TestAuthHandler_Login_Valido_SinRole(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	// Registrar usuario real con hash bcrypt para que Login funcione
	// Necesitamos crear usuario via repo con hash real
	// En lugar de mock simple, usamos el repo mock pero con password hasheado via bcrypt
	// Para simplificar, insertamos directamente con CreateUser que hashea en service, pero login compara hash
	// Nuestro mock almacena passwordHash tal cual lo genera service (ya hasheado)
	// Así que registramos via service para que el hash quede guardado
	h := NewAuthHandler(svc)
	// Register
	bodyReg, _ := json.Marshal(map[string]string{"email": "login@test.invalid", "password": "Pass1234"})
	wReg := httptest.NewRecorder()
	cReg, _ := gin.CreateTestContext(wReg)
	cReg.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(bodyReg))
	cReg.Request.Header.Set("Content-Type", "application/json")
	h.Register(cReg)
	if wReg.Code != 201 {
		t.Fatalf("register para login %d %s", wReg.Code, wReg.Body.String())
	}
	// Login
	bodyLogin, _ := json.Marshal(map[string]string{"email": "login@test.invalid", "password": "Pass1234"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewReader(bodyLogin))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != 200 {
		t.Fatalf("login esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["access_token"] == "" || resp["refresh_token"] == "" {
		t.Fatal("tokens vacíos")
	}
	// Verificar JWT sin role
	tokenStr := resp["access_token"].(string)
	parser := jwt.NewParser()
	tok, _, _ := parser.ParseUnverified(tokenStr, jwt.MapClaims{})
	claims := tok.Claims.(jwt.MapClaims)
	if _, ok := claims["role"]; ok {
		t.Fatal("JWT no debe contener role")
	}
	if claims["user_id"] == "" || claims["iss"] != "apuntes-auth" || claims["aud"] == nil || claims["iat"] == nil || claims["exp"] == nil {
		t.Fatalf("claims incompletos %v", claims)
	}
}

// Test 8: Login credenciales inválidas 401
func TestAuthHandler_Login_CredencialesInvalidas_401(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	// Registrar
	bodyReg, _ := json.Marshal(map[string]string{"email": "login2@test.invalid", "password": "Pass1234"})
	wReg := httptest.NewRecorder()
	cReg, _ := gin.CreateTestContext(wReg)
	cReg.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(bodyReg))
	cReg.Request.Header.Set("Content-Type", "application/json")
	h.Register(cReg)
	// Login con password mala
	bodyBad, _ := json.Marshal(map[string]string{"email": "login2@test.invalid", "password": "WrongPass1"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewReader(bodyBad))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Login(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401, got %d %s", w.Code, w.Body.String())
	}
}

// Test 9: No existe test que requiera role inválido — este test verifica que enviar role no causa invalid_role
func TestAuthHandler_Register_RoleIgnorado_No400(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	// Enviar role extra (antes era invalid_role), ahora debe ser ignorado y dar 201
	body, _ := json.Marshal(map[string]interface{}{"email": "roleignored@test.invalid", "password": "Pass1234", "role": "admin"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Register(c)
	// Como el handler ya no tiene campo Role, el JSON con role será ignorado (gin no falla por campo extra)
	if w.Code != 201 {
		t.Fatalf("role extra debe ser ignorado y dar 201, got %d %s", w.Code, w.Body.String())
	}
}

// Tests para POST /auth/refresh
func TestAuthHandler_Refresh_Valido_200(t *testing.T) {
	repo := newMockUserRepoH()
	refreshRepo := &mockRefreshH{tokens: make(map[string]*repository.RefreshToken)}
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, refreshRepo, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	// Register + Login para obtener refresh_token
	bodyReg, _ := json.Marshal(map[string]string{"email": "refresh@test.invalid", "password": "Pass1234"})
	wReg := httptest.NewRecorder()
	cReg, _ := gin.CreateTestContext(wReg)
	cReg.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(bodyReg))
	cReg.Request.Header.Set("Content-Type", "application/json")
	h.Register(cReg)
	bodyLogin, _ := json.Marshal(map[string]string{"email": "refresh@test.invalid", "password": "Pass1234"})
	wLogin := httptest.NewRecorder()
	cLogin, _ := gin.CreateTestContext(wLogin)
	cLogin.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewReader(bodyLogin))
	cLogin.Request.Header.Set("Content-Type", "application/json")
	h.Login(cLogin)
	var loginResp map[string]interface{}
	json.Unmarshal(wLogin.Body.Bytes(), &loginResp)
	refreshToken := loginResp["refresh_token"].(string)
	// Refresh
	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c)
	if w.Code != 200 {
		t.Fatalf("refresh válido esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["access_token"] == "" || resp["refresh_token"] == "" || resp["expires_in"] == nil {
		t.Fatalf("respuesta refresh incompleta %v", resp)
	}
	// Verificar JWT sin role
	tok, _, _ := jwt.NewParser().ParseUnverified(resp["access_token"].(string), jwt.MapClaims{})
	if _, ok := tok.Claims.(jwt.MapClaims)["role"]; ok {
		t.Fatal("JWT no debe contener role")
	}
}

func TestAuthHandler_Refresh_Inexistente_401(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{tokens: make(map[string]*repository.RefreshToken)}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	body, _ := json.Marshal(map[string]string{"refresh_token": "noexiste1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401, got %d %s", w.Code, w.Body.String())
	}
}

func TestAuthHandler_Refresh_BodyVacio_400(t *testing.T) {
	repo := newMockUserRepoH()
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, &mockRefreshH{}, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 body vacío, got %d", w.Code)
	}
}

func TestAuthHandler_Refresh_Revocado_401(t *testing.T) {
	repo := newMockUserRepoH()
	refreshRepo := &mockRefreshH{tokens: make(map[string]*repository.RefreshToken)}
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("handler-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	svc := service.NewAuthService(repo, refreshRepo, jwtSvc, 900, 604800)
	h := NewAuthHandler(svc)
	// Register + Login
	bodyReg, _ := json.Marshal(map[string]string{"email": "revokedh@test.invalid", "password": "Pass1234"})
	wReg := httptest.NewRecorder()
	cReg, _ := gin.CreateTestContext(wReg)
	cReg.Request = httptest.NewRequest("POST", "/auth/register", bytes.NewReader(bodyReg))
	cReg.Request.Header.Set("Content-Type", "application/json")
	h.Register(cReg)
	bodyLogin, _ := json.Marshal(map[string]string{"email": "revokedh@test.invalid", "password": "Pass1234"})
	wLogin := httptest.NewRecorder()
	cLogin, _ := gin.CreateTestContext(wLogin)
	cLogin.Request = httptest.NewRequest("POST", "/auth/login", bytes.NewReader(bodyLogin))
	cLogin.Request.Header.Set("Content-Type", "application/json")
	h.Login(cLogin)
	var lr map[string]interface{}
	json.Unmarshal(wLogin.Body.Bytes(), &lr)
	refreshToken := lr["refresh_token"].(string)
	// Primer refresh ok (rota y revoca viejo)
	body, _ := json.Marshal(map[string]string{"refresh_token": refreshToken})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c)
	// Segundo intento con mismo viejo debe ser 401
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(body))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.Refresh(c2)
	if w2.Code != 401 {
		t.Fatalf("esperado 401 por revocado, got %d %s", w2.Code, w2.Body.String())
	}
}

// Evitar imports no usados
var _ = repository.GenerateRawToken
var _ = bcrypt.CompareHashAndPassword
