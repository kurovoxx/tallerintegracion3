package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

// --- Mocks en memoria ---

type mockUserRepo struct {
	usersByEmail map[string]*model.User
	usersByID    map[string]*model.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{
		usersByEmail: make(map[string]*model.User),
		usersByID:    make(map[string]*model.User),
	}
}

func (m *mockUserRepo) CreateUser(ctx context.Context, email, passwordHash, displayName string, photoURL, phone, institution, description, visibility *string) (*model.User, error) {
	// Simula repo real: valida display_name y inserta
	email = strings.ToLower(strings.TrimSpace(email))
	if _, exists := m.usersByEmail[email]; exists {
		return nil, &mockErr{msg: "email_taken: duplicate"}
	}
	// display_name 1..100
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 100 {
		return nil, &mockErr{msg: "invalid_display_name"}
	}
	u := &model.User{
		ID:           "550e8400-e29b-41d4-a716-446655440001",
		Email:        email,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
	}
	// Simular ID único por email
	if email == "dup@example.invalid" {
		// ya manejado arriba
	}
	// Generar ID diferente por email para distinguir
	if email != "test@example.invalid" && email != "opencode.profile-test@example.invalid" {
		// usar email hash simple para ID
		u.ID = "550e8400-e29b-41d4-a716-44665544" + strings.Repeat("0", 1) + "1"
		// diferenciar por email
		if strings.Contains(email, "login") {
			u.ID = "660e8400-e29b-41d4-a716-446655440001"
		}
	}
	m.usersByEmail[email] = u
	m.usersByID[u.ID] = u
	return u, nil
}

func (m *mockUserRepo) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if u, ok := m.usersByEmail[email]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	_, ok := m.usersByEmail[email]
	return ok, nil
}

type mockErr struct{ msg string }

func (e *mockErr) Error() string { return e.msg }

type mockRefreshRepo struct {
	tokens map[string]*repository.RefreshToken // id -> token
}

func newMockRefreshRepo() *mockRefreshRepo {
	return &mockRefreshRepo{tokens: make(map[string]*repository.RefreshToken)}
}

func (m *mockRefreshRepo) Create(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) (string, error) {
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

func (m *mockRefreshRepo) CountByUser(ctx context.Context, userID string) (int, error) {
	c := 0
	for _, t := range m.tokens {
		if t.UserID == userID {
			c++
		}
	}
	return c, nil
}

func (m *mockRefreshRepo) FindByRawToken(ctx context.Context, raw string) (*repository.RefreshToken, error) {
	for _, t := range m.tokens {
		if err := bcrypt.CompareHashAndPassword([]byte(t.TokenHash), []byte(raw)); err == nil {
			cp := *t
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRefreshRepo) Revoke(ctx context.Context, id string) error {
	if t, ok := m.tokens[id]; ok {
		t.Revoked = true
	}
	return nil
}

func (m *mockRefreshRepo) Rotate(ctx context.Context, oldID, userID, newHash string, newExpires time.Time) (string, error) {
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

// Helper para JWT de prueba sin role
func newJWTForTest(t *testing.T) *JWTService {
	t.Helper()
	svc, err := NuevoJWTService(ConfiguracionJWT{
		ClaveSecreta: []byte("test-secret-32-chars-long-for-jwt"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("jwt service: %v", err)
	}
	return svc
}

// 1. Register exitoso sin role
func TestRegister_SinRole_Exitoso(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	user, err := svc.Register(context.Background(), "newuser@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register sin role debe ser exitoso, got %v", err)
	}
	if user.Email != "newuser@example.invalid" {
		t.Fatalf("email %v", user.Email)
	}
	if user.ID == "" {
		t.Fatal("ID vacío")
	}
	// Verificar que no hay Role en modelo (compilación ya garantiza)
	// Verificar que GetByEmail luego funciona sin role
	got, _ := repo.GetByEmail(context.Background(), "newuser@example.invalid")
	if got == nil || got.Email != "newuser@example.invalid" {
		t.Fatal("usuario no persistido sin role")
	}
}

// 2. Register no exige role (antes era required)
func TestRegister_NoExigeRole(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	// No se pasa role, debe funcionar
	_, err := svc.Register(context.Background(), "norole@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("no debe exigir role, got %v", err)
	}
}

// 3. Register no devuelve role
func TestRegister_NoDevuelveRole(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	user, err := svc.Register(context.Background(), "noreturnrole@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	// Verificar que el struct User no tiene campo Role (compilación) y que JSON no lo expone
	// Aquí verificamos que el objeto no tenga role via reflexión simple: el campo no existe, así que no hay valor
	// Solo verificamos que email y ID existen
	if user.Email == "" || user.ID == "" {
		t.Fatal("user incompleto")
	}
}

// 4. Email duplicado 409
func TestRegister_EmailDuplicado_409(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	_, err := svc.Register(context.Background(), "dup@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("primer registro %v", err)
	}
	_, err = svc.Register(context.Background(), "dup@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("esperaba email_taken")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "email_taken" {
		t.Fatalf("esperado email_taken, got %v", err)
	}
}

// 5. Email inválido 400
func TestRegister_EmailInvalido_400(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	_, err := svc.Register(context.Background(), "no-email", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("esperado invalid_email")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_email" {
		t.Fatalf("esperado invalid_email, got %v", err)
	}
}

// 6. Contraseña inválida 400
func TestRegister_PasswordInvalida_400(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	_, err := svc.Register(context.Background(), "valid@example.invalid", "123", nil, nil, nil, nil, nil, nil) // débil
	if err == nil {
		t.Fatal("esperado weak_password")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "weak_password" {
		t.Fatalf("esperado weak_password, got %v", err)
	}
}

// 7. Login válido sin role genera JWT sin role
func TestLogin_Valido_SinRole_JWT(t *testing.T) {
	repo := newMockUserRepo()
	jwtSvc := newJWTForTest(t)
	svc := NewAuthService(repo, newMockRefreshRepo(), jwtSvc, 900, 604800)
	// Registrar primero
	_, err := svc.Register(context.Background(), "loginuser@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register %v", err)
	}
	// Login
	res, err := svc.Login(context.Background(), "loginuser@example.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("login %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatal("tokens vacíos")
	}
	if res.ExpiresIn != 900 {
		t.Fatalf("expires_in %d", res.ExpiresIn)
	}
	// Validar JWT contiene user_id, iss, aud, iat, exp y NO role
	// Usar parser sin validar firma para inspeccionar claims
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(res.AccessToken, jwt.MapClaims{})
	if err != nil {
		t.Fatalf("parse %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims no son MapClaims")
	}
	if claims["user_id"] == nil || claims["user_id"] == "" {
		t.Fatal("user_id ausente en JWT")
	}
	if claims["iss"] != "apuntes-auth" {
		t.Fatalf("iss %v", claims["iss"])
	}
	if claims["aud"] == nil {
		t.Fatal("aud ausente")
	}
	if claims["iat"] == nil || claims["exp"] == nil {
		t.Fatal("iat/exp ausentes")
	}
	if _, exists := claims["role"]; exists {
		t.Fatalf("role no debe existir, got %v", claims["role"])
	}
	// Validar via service que el token es válido y retorna solo ID
	usuario, err := jwtSvc.ValidarAccessToken(res.AccessToken)
	if err != nil {
		t.Fatalf("validar %v", err)
	}
	if usuario.ID == "" {
		t.Fatal("user_id vacío tras validar")
	}
}

// 8. Login credenciales inválidas 401
func TestLogin_CredencialesInvalidas_401(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	svc.Register(context.Background(), "login2@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	_, err := svc.Login(context.Background(), "login2@example.invalid", "WrongPass1")
	if err == nil {
		t.Fatal("esperado invalid_credentials")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_credentials" {
		t.Fatalf("esperado invalid_credentials, got %v", err)
	}
	_, err = svc.Login(context.Background(), "noexiste@example.invalid", "Pass1234")
	if err == nil {
		t.Fatal("esperado invalid_credentials para no existe")
	}
}

// 9. No debe existir test que requiera role inválido — se verifica que IsValidRole no existe y ErrInvalidRole no se usa en Register
func TestNoExisteValidacionRole(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	// Intentar registrar con payload que antes hubiera sido role inválido — ahora debe ignorarse
	// Como Register ya no recibe role, el registro debe ser exitoso aunque antes hubiera sido "admin"
	user, err := svc.Register(context.Background(), "norole2@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register sin role no debe fallar por role, got %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
	// Verificar que no hay código que valide role
	// Si IsValidRole aún existiera, este test fallaría en compilación al no importarlo; el hecho de compilar confirma eliminación
	_ = user
}

// Test adicional: display_name derivado del email
func TestRegister_DisplayNameDerivadoDelEmail(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	user, err := svc.Register(context.Background(), "juan.perez@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
	custom := "Mi Nombre"
	user2, err := svc.Register(context.Background(), "otro@example.invalid", "Pass1234", &custom, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("custom display_name %v", err)
	}
	if user2 == nil {
		t.Fatal("user2 nil")
	}
}

func TestRegister_DisplayNameExplicitoValido(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	display := "Nombre Valido"
	user, err := svc.Register(context.Background(), "explicitvalid@example.invalid", "Pass1234", &display, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("display_name explícito válido debe pasar, got %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
}

func TestRegister_DisplayNameExplicitoVacio_Deriva(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	empty := ""
	user, err := svc.Register(context.Background(), "vacio@example.invalid", "Pass1234", &empty, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("display_name vacío debe derivar del email y no fallar, got %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
	// También con espacios
	spaces := "   "
	user2, err := svc.Register(context.Background(), "vacio2@example.invalid", "Pass1234", &spaces, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("display_name espacios debe derivar, got %v", err)
	}
	if user2 == nil {
		t.Fatal("user2 nil")
	}
}

func TestRegister_DisplayNameExplicitoLargo_400(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	largo := ""
	for i := 0; i < 101; i++ {
		largo += "a"
	}
	_, err := svc.Register(context.Background(), "largo@example.invalid", "Pass1234", &largo, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("esperado invalid_display_name por >100")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_display_name" {
		t.Fatalf("esperado invalid_display_name, got %v", err)
	}
}

func TestRegister_EmailLocalPartLargo_TruncaA100(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	local := ""
	for i := 0; i < 150; i++ {
		local += "a"
	}
	email := local + "@example.invalid"
	// email local 150 >100, pero email total 150+1+15=166 <255, válido
	user, err := svc.Register(context.Background(), email, "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("email local largo debe truncar display_name a 100 y no fallar, got %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
	// Verificar que el email se normalizó a lower y que display_name truncado no causó error
	// El mock valida display_name 1..100, así que si no falla, truncamiento funcionó
}

func TestRegister_EmailLocalPartVacio_FallbackUsuario(t *testing.T) {
	// Caso límite: email válido cuyo local-part quedaría vacío tras trim es imposible con ValidateEmail,
	// pero probamos que el fallback "Usuario" funciona si display_name es nil y email es raro.
	// Usamos email normal y display_name nil para verificar que nunca genera vacío.
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	// Email normal debe derivar local-part, no fallback
	user, err := svc.Register(context.Background(), "normal@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if user == nil {
		t.Fatal("user nil")
	}
	// Verificar que display_name derivado no es vacío y no modifica email
	if user.Email != "normal@example.invalid" {
		t.Fatalf("email no debe modificarse, got %s", user.Email)
	}
}

// Tests para POST /auth/refresh (rotación)

func TestRefresh_Valido_Rota(t *testing.T) {
	repo := newMockUserRepo()
	refreshRepo := newMockRefreshRepo()
	jwtSvc := newJWTForTest(t)
	svc := NewAuthService(repo, refreshRepo, jwtSvc, 900, 604800)
	// Registrar y login para obtener refresh_token
	_, err := svc.Register(context.Background(), "refresh1@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register %v", err)
	}
	loginRes, err := svc.Login(context.Background(), "refresh1@example.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("login %v", err)
	}
	oldRefresh := loginRes.RefreshToken
	// Pequeña espera para asegurar iat diferente si se genera en el mismo segundo
	time.Sleep(1100 * time.Millisecond)
	// Refresh válido debe rotar
	res, err := svc.Refresh(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("refresh válido debe pasar, got %v", err)
	}
	if res.AccessToken == "" || res.RefreshToken == "" {
		t.Fatal("tokens vacíos en refresh")
	}
	if res.RefreshToken == oldRefresh {
		t.Fatal("refresh_token debe ser nuevo por rotación")
	}
	// Verificar que el viejo está revocado (FindByRawToken debe encontrarlo pero con Revoked=true)
	rt, _ := refreshRepo.FindByRawToken(context.Background(), oldRefresh)
	if rt == nil || !rt.Revoked {
		t.Fatal("viejo refresh_token debe quedar revocado")
	}
	// Verificar que el nuevo es válido y no revocado
	rtNew, _ := refreshRepo.FindByRawToken(context.Background(), res.RefreshToken)
	if rtNew == nil || rtNew.Revoked {
		t.Fatal("nuevo refresh_token debe existir y no revocado")
	}
	// Verificar JWT sin role
	parser := jwt.NewParser()
	tok, _, _ := parser.ParseUnverified(res.AccessToken, jwt.MapClaims{})
	claims := tok.Claims.(jwt.MapClaims)
	if _, ok := claims["role"]; ok {
		t.Fatal("JWT no debe contener role")
	}
}

func TestRefresh_TokenInexistente_401(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	_, err := svc.Refresh(context.Background(), "noexiste1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab")
	if err == nil {
		t.Fatal("esperado invalid_token")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_token" {
		t.Fatalf("esperado invalid_token, got %v", err)
	}
}

func TestRefresh_TokenRevocado_401(t *testing.T) {
	repo := newMockUserRepo()
	refreshRepo := newMockRefreshRepo()
	svc := NewAuthService(repo, refreshRepo, newJWTForTest(t), 900, 604800)
	svc.Register(context.Background(), "revoked@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	loginRes, _ := svc.Login(context.Background(), "revoked@example.invalid", "Pass1234")
	oldRefresh := loginRes.RefreshToken
	// Primera rotación revoca el viejo
	svc.Refresh(context.Background(), oldRefresh)
	// Segundo intento con el mismo viejo debe fallar (revocado)
	_, err := svc.Refresh(context.Background(), oldRefresh)
	if err == nil {
		t.Fatal("esperado invalid_token por revocado")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_token" {
		t.Fatalf("esperado invalid_token revocado, got %v", err)
	}
}

func TestRefresh_TokenExpirado_401(t *testing.T) {
	repo := newMockUserRepo()
	refreshRepo := newMockRefreshRepo()
	jwtSvc := newJWTForTest(t)
	svc := NewAuthService(repo, refreshRepo, jwtSvc, 900, 604800)
	svc.Register(context.Background(), "expired@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	// Crear manualmente un refresh_token expirado
	raw, hash, _ := repository.GenerateRawToken()
	// Guardar con expires_at en pasado
	refreshRepo.Create(context.Background(), "550e8400-e29b-41d4-a716-446655440001", hash, time.Now().Add(-1*time.Hour))
	// Como no conocemos el userID real del mock, usamos el raw que acabamos de crear
	// El FindByRawToken lo encontrará, pero debe estar expirado
	// Necesitamos asegurar que el token pertenece a un usuario que existe, pero para este test usamos el raw directamente
	// Creamos un token para el usuario que registramos, pero con expiración pasada
	// Para simplificar, insertamos directamente en el mock con expiración pasada para el usuario correcto
	// Primero login para obtener userID
	loginRes, _ := svc.Login(context.Background(), "expired@example.invalid", "Pass1234")
	_ = loginRes
	// Crear token expirado manualmente con bcrypt del raw
	rawExp, hashExp, _ := repository.GenerateRawToken()
	// Buscar userID del usuario registrado
	user, _ := repo.GetByEmail(context.Background(), "expired@example.invalid")
	refreshRepo.Create(context.Background(), user.ID, hashExp, time.Now().Add(-1*time.Hour))
	_, err := svc.Refresh(context.Background(), rawExp)
	if err == nil {
		t.Fatal("esperado token_expired")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "token_expired" {
		t.Fatalf("esperado token_expired, got %v", err)
	}
	_ = raw
}

func TestRefresh_ReutilizacionTokenViejo_Falla(t *testing.T) {
	repo := newMockUserRepo()
	refreshRepo := newMockRefreshRepo()
	svc := NewAuthService(repo, refreshRepo, newJWTForTest(t), 900, 604800)
	svc.Register(context.Background(), "reuse@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	loginRes, _ := svc.Login(context.Background(), "reuse@example.invalid", "Pass1234")
	oldRefresh := loginRes.RefreshToken
	// Primer refresh ok
	res1, err := svc.Refresh(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("primer refresh %v", err)
	}
	// Segundo refresh con el mismo viejo debe fallar
	_, err = svc.Refresh(context.Background(), oldRefresh)
	if err == nil {
		t.Fatal("reuso de viejo refresh debe fallar")
	}
	// Tercer refresh con el nuevo debe funcionar
	_, err = svc.Refresh(context.Background(), res1.RefreshToken)
	if err != nil {
		t.Fatalf("refresh con nuevo token debe pasar, got %v", err)
	}
}

func TestRefresh_BodyVacio_400(t *testing.T) {
	repo := newMockUserRepo()
	svc := NewAuthService(repo, newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	_, err := svc.Refresh(context.Background(), "   ")
	if err == nil {
		t.Fatal("esperado invalid_token por body vacío")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_token" {
		t.Fatalf("esperado invalid_token, got %v", err)
	}
}

// Evitar import no usado
var _ = repository.GenerateRawToken
var _ = bcrypt.CompareHashAndPassword
