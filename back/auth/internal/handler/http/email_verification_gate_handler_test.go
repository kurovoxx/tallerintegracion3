package http

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// IsEmailVerified convierte el store en memoria en el gate de Login/Refresh:
// una cuenta queda confirmada al canjear su token (verificationStoreH.Confirm).
func (s *verificationStoreH) IsEmailVerified(_ context.Context, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verified[userID], nil
}

// newGatedEnv arma registro, confirmación, login y refresh con el gate
// encendido (EMAIL_VERIFICATION_REQUIRED=true). ungated comparte repos y
// representa la versión de Auth previa a la activación.
func newGatedEnv(t *testing.T) (env *verificationEnv, ungated *gin.Engine) {
	t.Helper()
	env = newVerificationEnv(t, true)
	refresh := &mockRefreshH{}
	jwtSvc := newCoverageJWT(t)
	svc, err := service.NewEmailVerificationService(env.users, env.store, env.mailer, service.EmailVerificationConfig{})
	if err != nil {
		t.Fatal(err)
	}
	gated := service.NewAuthService(env.users, refresh, jwtSvc, 900, 604800)
	gated.SetEmailVerifier(svc)
	gated.SetEmailVerificationGate(env.store)
	h := NewEmailVerificationHandler(svc)
	env.router = gin.New()
	env.router.POST("/auth/register", NewAuthHandler(gated).Register)
	env.router.POST("/auth/login", NewAuthHandler(gated).Login)
	env.router.POST("/auth/refresh", NewAuthHandler(gated).Refresh)
	env.router.POST("/auth/verify-email", h.Verify)

	plain := service.NewAuthService(env.users, refresh, jwtSvc, 900, 604800)
	ungated = gin.New()
	ungated.POST("/auth/login", NewAuthHandler(plain).Login)
	return env, ungated
}

func TestE2E_EmailVerificationGate_RegisterLoginVerifyLogin(t *testing.T) {
	buf := captureLogs(t)
	env, _ := newGatedEnv(t)
	creds := `{"email":"gate.user@example.com","password":"Pass1234"}`
	if w := env.post("/auth/register", creds); w.Code != http.StatusCreated {
		t.Fatalf("register %d %s", w.Code, w.Body.String())
	}
	mail := env.mailer.wait(t)

	w := env.post("/auth/login", creds)
	if w.Code != http.StatusForbidden || errorCode(t, w) != "email_not_verified" {
		t.Fatalf("login sin confirmar: %d %s", w.Code, w.Body.String())
	}
	msg, _ := decodeBody(t, w)["error"].(map[string]any)["message"].(string)
	if !strings.Contains(msg, "Confirma tu correo") || strings.Contains(w.Body.String(), "access_token") {
		t.Fatalf("respuesta sin tokens y con mensaje para Flutter: %s", w.Body.String())
	}
	// Credenciales incorrectas siguen siendo 401 aunque la cuenta no esté confirmada.
	if w := env.post("/auth/login", `{"email":"gate.user@example.com","password":"WrongPass1"}`); w.Code != http.StatusUnauthorized || errorCode(t, w) != "invalid_credentials" {
		t.Fatalf("contraseña incorrecta: %d %s", w.Code, w.Body.String())
	}
	if w := env.post("/auth/login", `{"email":"nadie@example.com","password":"Pass1234"}`); w.Code != http.StatusUnauthorized || errorCode(t, w) != "invalid_credentials" {
		t.Fatalf("cuenta inexistente: %d %s", w.Code, w.Body.String())
	}

	if w := env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`); w.Code != http.StatusOK {
		t.Fatalf("verify %d %s", w.Code, w.Body.String())
	}
	w = env.post("/auth/login", creds)
	if w.Code != http.StatusOK {
		t.Fatalf("login confirmado: %d %s", w.Code, w.Body.String())
	}
	refresh, _ := decodeBody(t, w)["refresh_token"].(string)
	if w := env.post("/auth/refresh", `{"refresh_token":"`+refresh+`"}`); w.Code != http.StatusOK {
		t.Fatalf("refresh confirmado: %d %s", w.Code, w.Body.String())
	}
	out := buf.String()
	mustContain(t, out, "code=email_not_verified")
	mustNotContain(t, out, mail.Token)
	mustNotContain(t, out, refresh)
}

func TestE2E_EmailVerificationGate_RefreshOfPreviousSessionIs401(t *testing.T) {
	env, ungated := newGatedEnv(t)
	creds := `{"email":"old.session@example.com","password":"Pass1234"}`
	if w := env.post("/auth/register", creds); w.Code != http.StatusCreated {
		t.Fatalf("register %d", w.Code)
	}
	mail := env.mailer.wait(t)
	// Sesión iniciada antes de activar el gate.
	w := postTo(ungated, "/auth/login", creds)
	if w.Code != http.StatusOK {
		t.Fatalf("login previo: %d %s", w.Code, w.Body.String())
	}
	refresh, _ := decodeBody(t, w)["refresh_token"].(string)
	// Con el gate activo, la sesión no se renueva: 401 para que Flutter la cierre.
	w = env.post("/auth/refresh", `{"refresh_token":"`+refresh+`"}`)
	if w.Code != http.StatusUnauthorized || errorCode(t, w) != "email_not_verified" {
		t.Fatalf("refresh sin confirmar: %d %s", w.Code, w.Body.String())
	}
	// Tras confirmar, el mismo refresh vuelve a servir (no se revocó).
	if w := env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`); w.Code != http.StatusOK {
		t.Fatalf("verify %d", w.Code)
	}
	if w := env.post("/auth/refresh", `{"refresh_token":"`+refresh+`"}`); w.Code != http.StatusOK {
		t.Fatalf("refresh tras confirmar: %d %s", w.Code, w.Body.String())
	}
}

func postTo(r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	e := &verificationEnv{router: r}
	return e.post(path, body)
}

type failingGateH struct{}

func (failingGateH) IsEmailVerified(context.Context, string) (bool, error) {
	return false, errors.New("db down")
}

// failingUsersH hace fallar la lectura del usuario (error no tipado → 500).
type failingUsersH struct{ *mockUserRepoH }

func (failingUsersH) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, errors.New("get by email: connection refused")
}

// Los logs de login nunca incluyen el correo, la contraseña ni los tokens,
// en ningún resultado, y conservan el motivo (code=...).
func TestLogging_LoginNeverLogsEmailOrSecrets(t *testing.T) {
	const email, password = "private.login@example.com", "Pass1234Secret"
	creds := `{"email":"` + email + `","password":"` + password + `"}`

	env, ungated := newGatedEnv(t)
	if w := env.post("/auth/register", creds); w.Code != http.StatusCreated {
		t.Fatalf("register %d", w.Code)
	}
	mail := env.mailer.wait(t)
	// Se captura desde aquí: esta prueba cubre solo los logs de login (el
	// registro tiene su propio formato, fuera del alcance de este cambio).
	// syncBuffer: la goroutine de envío del registro aún puede escribir.
	buf := &syncBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	cases := []struct {
		router *gin.Engine
		body   string
		status int
		code   string
	}{
		{env.router, `{"email":"` + email + `"`, http.StatusBadRequest, "code=bad_request"},
		{env.router, `{"email":"` + email + `","password":"WrongPass1"}`, http.StatusUnauthorized, "code=invalid_credentials"},
		{env.router, `{"email":"otra.persona@example.com","password":"` + password + `"}`, http.StatusUnauthorized, "code=invalid_credentials"},
		{env.router, creds, http.StatusForbidden, "code=email_not_verified"},
	}
	var tokens []string
	for _, tc := range cases {
		if w := postTo(tc.router, "/auth/login", tc.body); w.Code != tc.status {
			t.Fatalf("%s: status %d %s", tc.code, w.Code, w.Body.String())
		}
	}
	w := postTo(ungated, "/auth/login", creds)
	if w.Code != http.StatusOK {
		t.Fatalf("login ok: %d", w.Code)
	}
	body := decodeBody(t, w)
	for _, k := range []string{"access_token", "refresh_token"} {
		tok, _ := body[k].(string)
		tokens = append(tokens, tok)
	}

	// Error interno tipado (fallo del gate) y no tipado (fallo del repositorio).
	gateFail := service.NewAuthService(env.users, &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	gateFail.SetEmailVerificationGate(failingGateH{})
	repoFail := service.NewAuthService(failingUsersH{env.users}, &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	for _, svc := range []*service.AuthService{gateFail, repoFail} {
		r := gin.New()
		r.POST("/auth/login", NewAuthHandler(svc).Login)
		if w := postTo(r, "/auth/login", creds); w.Code != http.StatusInternalServerError {
			t.Fatalf("error interno: %d %s", w.Code, w.Body.String())
		}
	}

	out := buf.String()
	for _, want := range []string{
		"auth login fail code=bad_request ip=",
		"auth login fail code=invalid_credentials ip=",
		"auth login fail code=email_not_verified ip=",
		"auth login fail code=internal_error ip=",
		"auth login ok ip=",
	} {
		mustContain(t, out, want)
	}
	for _, forbidden := range append([]string{email, "otra.persona", "private.login", "email=", password, "WrongPass1", mail.Token}, tokens...) {
		mustNotContain(t, out, forbidden)
	}
}
