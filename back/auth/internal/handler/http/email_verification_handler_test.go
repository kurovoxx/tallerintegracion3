package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// verificationStoreH es un store en memoria con un único token vigente por
// usuario y uso único, suficiente para probar el contrato HTTP.
type verificationStoreH struct {
	mu         sync.Mutex
	tokens     map[string]string // hash -> userID
	used       map[string]bool
	verified   map[string]bool
	expired    map[string]bool
	confirmErr error
}

func newVerificationStoreH() *verificationStoreH {
	return &verificationStoreH{tokens: map[string]string{}, used: map[string]bool{}, verified: map[string]bool{}, expired: map[string]bool{}}
}

func (s *verificationStoreH) Issue(_ context.Context, userID string, hash []byte, _ time.Duration) (repository.IssueOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.verified[userID] {
		return repository.IssueAlreadyVerified, nil
	}
	for h, uid := range s.tokens {
		if uid == userID {
			delete(s.tokens, h)
		}
	}
	s.tokens[string(hash)] = userID
	return repository.IssueIssued, nil
}

func (s *verificationStoreH) ReleaseCooldown(context.Context, string, []byte) error { return nil }

func (s *verificationStoreH) Confirm(_ context.Context, hash []byte) (repository.ConfirmOutcome, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.confirmErr != nil {
		return "", "", s.confirmErr
	}
	uid, ok := s.tokens[string(hash)]
	switch {
	case !ok:
		return repository.ConfirmInvalid, "", nil
	case s.used[string(hash)] && s.verified[uid]:
		return repository.ConfirmAlreadyVerified, uid, nil
	case s.expired[string(hash)]:
		return repository.ConfirmExpired, uid, nil
	}
	s.used[string(hash)], s.verified[uid] = true, true
	return repository.ConfirmVerified, uid, nil
}

type verificationMailerH struct {
	ready bool
	err   error
	sent  chan service.VerificationEmail
	gate  chan struct{} // si no es nil, el envío queda retenido hasta cerrarlo
}

func newVerificationMailerH(ready bool) *verificationMailerH {
	return &verificationMailerH{ready: ready, sent: make(chan service.VerificationEmail, 10)}
}

func (m *verificationMailerH) Ready() bool { return m.ready }
func (m *verificationMailerH) SendVerificationEmail(_ context.Context, e service.VerificationEmail) error {
	m.sent <- e
	if m.gate != nil {
		<-m.gate
	}
	return m.err
}

func (m *verificationMailerH) wait(t *testing.T) service.VerificationEmail {
	t.Helper()
	select {
	case e := <-m.sent:
		return e
	case <-time.After(3 * time.Second):
		t.Fatal("no se envió el correo")
		return service.VerificationEmail{}
	}
}

func (m *verificationMailerH) assertNothingSent(t *testing.T) {
	t.Helper()
	select {
	case <-m.sent:
		t.Fatal("no debía enviarse correo")
	case <-time.After(100 * time.Millisecond):
	}
}

type verificationEnv struct {
	router *gin.Engine
	users  *mockUserRepoH
	store  *verificationStoreH
	mailer *verificationMailerH
}

func newVerificationEnv(t *testing.T, mailReady bool) *verificationEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	env := &verificationEnv{users: newMockUserRepoH(), store: newVerificationStoreH(), mailer: newVerificationMailerH(mailReady)}
	svc, err := service.NewEmailVerificationService(env.users, env.store, env.mailer,
		service.EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/verificar-correo"})
	if err != nil {
		t.Fatal(err)
	}
	authSvc := service.NewAuthService(env.users, &mockRefreshH{}, newCoverageJWT(t), 900, 604800)
	authSvc.SetEmailVerifier(svc)
	h := NewEmailVerificationHandler(svc)
	env.router = gin.New()
	env.router.POST("/auth/register", NewAuthHandler(authSvc).Register)
	env.router.POST("/auth/send-verification-email", h.Send)
	env.router.POST("/auth/verify-email", h.Verify)
	return env
}

func (e *verificationEnv) post(path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	e.router.ServeHTTP(w, req)
	return w
}

func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("respuesta no es JSON: %s", w.Body.String())
	}
	return out
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	errObj, _ := decodeBody(t, w)["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	return code
}

func TestEmailVerificationHandler_RegisterSendsConfirmationAndKeepsContract(t *testing.T) {
	env := newVerificationEnv(t, true)
	w := env.post("/auth/register", `{"email":"New.User@example.com","password":"Pass1234"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("register %d %s", w.Code, w.Body.String())
	}
	body := decodeBody(t, w)
	if len(body) != 3 || body["id"] == "" || body["email"] != "new.user@example.com" || body["created_at"] == "" {
		t.Fatalf("el contrato 201 {id,email,created_at} no debe cambiar: %v", body)
	}
	mail := env.mailer.wait(t)
	if mail.Recipient != "new.user@example.com" || !strings.HasPrefix(mail.Link, "https://ti3.example.cl/verificar-correo?token=") {
		t.Fatalf("correo de registro inesperado: %+v", mail)
	}
	if w := env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`); w.Code != http.StatusOK || decodeBody(t, w)["status"] != "verified" {
		t.Fatalf("verify %d %s", w.Code, w.Body.String())
	}
}

// El 201 llega mientras el SMTP sigue en curso (retenido) y no depende de
// que termine ni de que falle después.
func TestEmailVerificationHandler_RegisterDoesNotWaitForSMTP(t *testing.T) {
	env := newVerificationEnv(t, true)
	env.mailer.gate = make(chan struct{})
	env.mailer.err = &service.SMTPError{Stage: "data", Code: 451}
	defer close(env.mailer.gate)
	start := time.Now()
	w := env.post("/auth/register", `{"email":"slow@example.com","password":"Pass1234"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("register %d %s", w.Code, w.Body.String())
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("el registro esperó al SMTP: %s", elapsed)
	}
	// El envío empezó en segundo plano y sigue retenido después del 201.
	if mail := env.mailer.wait(t); mail.Recipient != "slow@example.com" {
		t.Fatalf("destinatario inesperado %q", mail.Recipient)
	}
}

func TestEmailVerificationHandler_RegisterSucceedsWhenMailFailsOrIsDisabled(t *testing.T) {
	failing := newVerificationEnv(t, true)
	failing.mailer.err = &service.SMTPError{Stage: "dial"}
	if w := failing.post("/auth/register", `{"email":"a@example.com","password":"Pass1234"}`); w.Code != http.StatusCreated {
		t.Fatalf("un fallo SMTP no debe romper el registro: %d", w.Code)
	}
	failing.mailer.wait(t)
	// Reintentar el registro no duplica la cuenta: sigue siendo email_taken.
	if w := failing.post("/auth/register", `{"email":"a@example.com","password":"Pass1234"}`); w.Code != http.StatusConflict {
		t.Fatalf("reintento debe ser 409, got %d", w.Code)
	}

	disabled := newVerificationEnv(t, false)
	if w := disabled.post("/auth/register", `{"email":"b@example.com","password":"Pass1234"}`); w.Code != http.StatusCreated {
		t.Fatalf("sin SMTP el registro debe seguir funcionando: %d", w.Code)
	}
	disabled.mailer.assertNothingSent(t)
}

func TestEmailVerificationHandler_SendIsGenericAndNoStore(t *testing.T) {
	env := newVerificationEnv(t, true)
	env.users.users["known@example.com"] = &model.User{ID: "user-known", Email: "known@example.com"}
	known := env.post("/auth/send-verification-email", `{"email":"Known@Example.com"}`)
	env.mailer.wait(t)
	unknown := env.post("/auth/send-verification-email", `{"email":"unknown@example.com"}`)
	env.mailer.assertNothingSent(t)
	if known.Code != http.StatusOK || unknown.Code != http.StatusOK || known.Body.String() != unknown.Body.String() {
		t.Fatalf("la respuesta no debe revelar si la cuenta existe:\n%d %s\n%d %s", known.Code, known.Body, unknown.Code, unknown.Body)
	}
	if known.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("Cache-Control: no-store")
	}
	if !strings.Contains(decodeBody(t, known)["message"].(string), "Si el correo está registrado") {
		t.Fatalf("mensaje genérico: %s", known.Body)
	}
}

func TestEmailVerificationHandler_SendErrors(t *testing.T) {
	env := newVerificationEnv(t, true)
	for _, tc := range []struct {
		body string
		code string
	}{
		{`{bad json`, "bad_request"},
		{`{}`, "bad_request"},
		{`{"email":""}`, "bad_request"},
		{`{"email":"no-es-correo"}`, "invalid_email"},
		{`{"email":"` + strings.Repeat("a", 5000) + `@example.com"}`, "bad_request"},
	} {
		w := env.post("/auth/send-verification-email", tc.body)
		if w.Code != http.StatusBadRequest || errorCode(t, w) != tc.code {
			t.Fatalf("%.40s: %d %s", tc.body, w.Code, w.Body.String())
		}
	}
	disabled := newVerificationEnv(t, false)
	w := disabled.post("/auth/send-verification-email", `{"email":"x@example.com"}`)
	if w.Code != http.StatusServiceUnavailable || errorCode(t, w) != "mail_unavailable" {
		t.Fatalf("sin SMTP: %d %s", w.Code, w.Body.String())
	}
}

func TestEmailVerificationHandler_VerifyOutcomes(t *testing.T) {
	env := newVerificationEnv(t, true)
	env.users.users["user@example.com"] = &model.User{ID: "user-1", Email: "user@example.com"}
	env.post("/auth/send-verification-email", `{"email":"user@example.com"}`)
	first := env.mailer.wait(t)
	env.post("/auth/send-verification-email", `{"email":"user@example.com"}`)
	second := env.mailer.wait(t)

	// El reenvío reemplaza el token anterior.
	if w := env.post("/auth/verify-email", `{"token":"`+first.Token+`"}`); w.Code != http.StatusBadRequest || errorCode(t, w) != ErrInvalidVerificationToken {
		t.Fatalf("token reemplazado: %d %s", w.Code, w.Body.String())
	}
	w := env.post("/auth/verify-email", `{"token":"`+second.Token+`"}`)
	if w.Code != http.StatusOK || decodeBody(t, w)["status"] != "verified" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("verify: %d %s", w.Code, w.Body.String())
	}
	if w := env.post("/auth/verify-email", `{"token":"`+second.Token+`"}`); w.Code != http.StatusOK || decodeBody(t, w)["status"] != "already_verified" {
		t.Fatalf("reabrir el enlace: %d %s", w.Code, w.Body.String())
	}
	// Cuenta ya confirmada: el reenvío responde igual y no envía nada.
	if w := env.post("/auth/send-verification-email", `{"email":"user@example.com"}`); w.Code != http.StatusOK {
		t.Fatalf("reenvío a cuenta confirmada: %d", w.Code)
	}
	env.mailer.assertNothingSent(t)

	for _, body := range []string{`{bad`, `{}`, `{"token":""}`} {
		if w := env.post("/auth/verify-email", body); w.Code != http.StatusBadRequest || errorCode(t, w) != "bad_request" {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body.String())
		}
	}
	for _, token := range []string{"corto", strings.Repeat("A", 43), first.Token + "x"} {
		if w := env.post("/auth/verify-email", `{"token":"`+token+`"}`); w.Code != http.StatusBadRequest || errorCode(t, w) != ErrInvalidVerificationToken {
			t.Fatalf("%s: %d %s", token, w.Code, w.Body.String())
		}
	}
}

func TestEmailVerificationHandler_VerifyExpiredAndInternalError(t *testing.T) {
	env := newVerificationEnv(t, true)
	env.users.users["user@example.com"] = &model.User{ID: "user-1", Email: "user@example.com"}
	env.post("/auth/send-verification-email", `{"email":"user@example.com"}`)
	mail := env.mailer.wait(t)
	env.store.mu.Lock()
	for h := range env.store.tokens {
		env.store.expired[h] = true
	}
	env.store.mu.Unlock()
	w := env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`)
	if w.Code != http.StatusBadRequest || errorCode(t, w) != ErrInvalidVerificationToken {
		t.Fatalf("expirado: %d %s", w.Code, w.Body.String())
	}
	env.store.confirmErr = errors.New("connection reset")
	w = env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`)
	if w.Code != http.StatusInternalServerError || errorCode(t, w) != "internal_error" || strings.Contains(w.Body.String(), "connection reset") {
		t.Fatalf("error interno: %d %s", w.Code, w.Body.String())
	}
}

func TestEmailVerificationHandler_LogsAreUsefulAndSafe(t *testing.T) {
	buf := &syncBuffer{}
	prev := log.Writer()
	log.SetOutput(buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	env := newVerificationEnv(t, true)
	env.users.users["private.person@example.com"] = &model.User{ID: "user-1", Email: "private.person@example.com"}
	env.post("/auth/send-verification-email", `{"email":"private.person@example.com"}`)
	mail := env.mailer.wait(t)
	env.post("/auth/send-verification-email", `{"email":"no-es-correo"}`)
	env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`)
	env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`)
	env.post("/auth/verify-email", `{"token":"invalido"}`)
	env.store.confirmErr = errors.New("db down")
	env.post("/auth/verify-email", `{"token":"`+mail.Token+`"}`)
	// El log "send ok" lo escribe la goroutine de envío después de volver del mailer.
	const sentLine = "[auth] email-verification op=SendVerificationEmail user_id=user-1 step=send status=ok outcome=sent"
	for deadline := time.Now().Add(3 * time.Second); !strings.Contains(buf.String(), sentLine) && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	out := buf.String()
	for _, want := range []string{
		"[auth] POST /auth/send-verification-email op=SendVerificationEmail status=200 user_id=- accepted=true",
		"[auth] POST /auth/send-verification-email op=SendVerificationEmail status=400 user_id=- step=service code=invalid_email",
		"[auth] POST /auth/verify-email op=VerifyEmail status=200 user_id=- outcome=verified",
		"[auth] POST /auth/verify-email op=VerifyEmail status=200 user_id=- outcome=already_verified",
		"[auth] POST /auth/verify-email op=VerifyEmail status=400 user_id=- code=invalid_verification_token outcome=invalid",
		"[auth] POST /auth/verify-email op=VerifyEmail status=500 user_id=- step=confirm code=internal_error",
		sentLine,
		"duration_ms=",
	} {
		mustContain(t, out, want)
	}
	for _, forbidden := range []string{mail.Token, mail.Link, "private.person", "db down"} {
		mustNotContain(t, out, forbidden)
	}
}

// syncBuffer permite leer el log mientras otra goroutine escribe.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
