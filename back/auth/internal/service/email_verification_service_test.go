package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

// verificationStoreFake simula identity.email_verifications en memoria con
// la misma semántica que el repositorio (un token por usuario, uso único).
type verificationStoreFake struct {
	mu            sync.Mutex
	issueOutcome  repository.IssueOutcome
	issueErr      error
	confirmErr    error
	releaseErr    error
	issued        map[string][]byte // userID -> hash vigente
	released      []string
	confirmCalls  int
	confirmResult map[string]repository.ConfirmOutcome // hex(hash) -> resultado
}

func newVerificationStoreFake() *verificationStoreFake {
	return &verificationStoreFake{
		issueOutcome:  repository.IssueIssued,
		issued:        map[string][]byte{},
		confirmResult: map[string]repository.ConfirmOutcome{},
	}
}

func (f *verificationStoreFake) Issue(_ context.Context, userID string, hash []byte, _ time.Duration) (repository.IssueOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.issueErr != nil {
		return "", f.issueErr
	}
	if f.issueOutcome == repository.IssueIssued {
		f.issued[userID] = append([]byte(nil), hash...)
	}
	return f.issueOutcome, nil
}

func (f *verificationStoreFake) ReleaseCooldown(_ context.Context, userID string, hash []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.issued[userID], hash) {
		return errors.New("release with a hash that is not current")
	}
	f.released = append(f.released, userID)
	return f.releaseErr
}

func (f *verificationStoreFake) Confirm(_ context.Context, hash []byte) (repository.ConfirmOutcome, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.confirmCalls++
	if f.confirmErr != nil {
		return "", "", f.confirmErr
	}
	if out, ok := f.confirmResult[hex.EncodeToString(hash)]; ok {
		return out, "user-1", nil
	}
	for uid, h := range f.issued {
		if bytes.Equal(h, hash) {
			delete(f.issued, uid)
			return repository.ConfirmVerified, uid, nil
		}
	}
	return repository.ConfirmInvalid, "", nil
}

// verificationMailerFake simula el SMTP y guarda lo que se habría enviado.
type verificationMailerFake struct {
	mu    sync.Mutex
	ready bool
	err   error
	sent  []VerificationEmail
}

func (m *verificationMailerFake) Ready() bool { return m.ready }
func (m *verificationMailerFake) SendVerificationEmail(_ context.Context, e VerificationEmail) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, e)
	return m.err
}

func newTestVerificationService(t *testing.T, users verificationUserLookup, store EmailVerificationStore, mailer VerificationMailer, cfg EmailVerificationConfig) *EmailVerificationService {
	t.Helper()
	svc, err := NewEmailVerificationService(users, store, mailer, cfg)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	svc.spawn = func(f func()) { f() } // envío síncrono en pruebas (el cupo se aplica igual)
	return svc
}

func TestVerificationToken_EntropyFormatAndHash(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		raw, hash, err := generateVerificationToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) != 43 || !validVerificationTokenFormat(raw) {
			t.Fatalf("token con formato inesperado: len=%d", len(raw))
		}
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(decoded) != 32 {
			t.Fatal("el token debe codificar 32 bytes aleatorios (256 bits)")
		}
		want := sha256.Sum256([]byte(raw))
		if !bytes.Equal(hash, want[:]) || bytes.Equal(hash, []byte(raw)) {
			t.Fatal("se persiste el SHA-256 del token, nunca el token")
		}
		if !bytes.Equal(hashVerificationToken(raw), hash) {
			t.Fatal("el hash debe ser determinista para poder buscarlo")
		}
		if seen[raw] {
			t.Fatal("token repetido")
		}
		seen[raw] = true
	}
}

func TestVerificationToken_RejectsMalformed(t *testing.T) {
	good, _, _ := generateVerificationToken()
	if !validVerificationTokenFormat(good) {
		t.Fatal("token válido rechazado")
	}
	// 43 caracteres = 258 bits: los 2 bits bajos del último deben ser 0. Con
	// uno de ellos en 1 el valor decodifica igual sin Strict, pero no es un
	// token que el servidor haya podido emitir.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, good[42])
	nonCanonical := good[:42] + string(alphabet[last|1])
	for _, tc := range []string{
		"", "abc", good[:42], good + "A", strings.Repeat("=", 43),
		good[:42] + "+", good[:42] + "/", good[:20] + " " + good[21:],
		nonCanonical,
	} {
		if validVerificationTokenFormat(tc) {
			t.Fatalf("formato inválido aceptado: %q", tc)
		}
	}
}

func TestEmailVerificationConfig_Validation(t *testing.T) {
	store, mailer := newVerificationStoreFake(), &verificationMailerFake{ready: true}
	for _, tc := range []struct {
		name string
		cfg  EmailVerificationConfig
		ok   bool
	}{
		{"defaults", EmailVerificationConfig{}, true},
		{"https link", EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/verificar-correo?src=mail"}, true},
		{"http localhost", EmailVerificationConfig{LinkBaseURL: "http://localhost:8081/verificar"}, true},
		{"http loopback ip", EmailVerificationConfig{LinkBaseURL: "http://127.0.0.1:8081/verificar"}, true},
		{"http remote", EmailVerificationConfig{LinkBaseURL: "http://ti3.example.cl/verificar"}, false},
		{"relative", EmailVerificationConfig{LinkBaseURL: "/verificar"}, false},
		{"fragment", EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/#/verificar"}, false},
		{"userinfo", EmailVerificationConfig{LinkBaseURL: "https://user:pass@ti3.example.cl/v"}, false},
		{"token preset", EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/v?token=x"}, false},
		{"other scheme", EmailVerificationConfig{LinkBaseURL: "sigma-academy://verificar"}, false},
		{"ttl min", EmailVerificationConfig{TTL: 15 * time.Minute}, true},
		{"ttl max", EmailVerificationConfig{TTL: 7 * 24 * time.Hour}, true},
		{"ttl short", EmailVerificationConfig{TTL: time.Minute}, false},
		{"ttl long", EmailVerificationConfig{TTL: 8 * 24 * time.Hour}, false},
		{"ttl malformed", EmailVerificationConfig{TTL: -1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := NewEmailVerificationService(newMockUserRepo(), store, mailer, tc.cfg)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v", err)
			}
			if err == nil && tc.cfg.TTL == 0 && svc.TTL() != 24*time.Hour {
				t.Fatalf("TTL por defecto %s", svc.TTL())
			}
		})
	}
}

func TestEmailVerification_LinkKeepsQueryAndEncodesToken(t *testing.T) {
	svc := newTestVerificationService(t, newMockUserRepo(), newVerificationStoreFake(), &verificationMailerFake{ready: true},
		EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/verificar-correo?src=mail"})
	token, _, _ := generateVerificationToken()
	u, err := url.Parse(svc.link(token))
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("token") != token || u.Query().Get("src") != "mail" || u.Host != "ti3.example.cl" {
		t.Fatalf("enlace inesperado: %s", u)
	}
	noLink := newTestVerificationService(t, newMockUserRepo(), newVerificationStoreFake(), &verificationMailerFake{ready: true}, EmailVerificationConfig{})
	if noLink.link(token) != "" {
		t.Fatal("sin EMAIL_VERIFICATION_URL no hay enlace")
	}
}

func TestEmailVerification_Request(t *testing.T) {
	users := newMockUserRepo()
	users.usersByEmail["user@example.com"] = &model.User{ID: "user-1", Email: "user@example.com"}
	for _, tc := range []struct {
		name, email  string
		outcome      repository.IssueOutcome
		issueErr     error
		ready        bool
		sendErr      error
		wantCode     string
		wantSend     bool
		wantReleased bool
	}{
		{name: "registered", email: " USER@example.com ", outcome: repository.IssueIssued, ready: true, wantSend: true},
		{name: "unknown stays generic", email: "nobody@example.com", outcome: repository.IssueIssued, ready: true},
		{name: "throttled stays generic", email: "user@example.com", outcome: repository.IssueThrottled, ready: true},
		{name: "already verified stays generic", email: "user@example.com", outcome: repository.IssueAlreadyVerified, ready: true},
		{name: "deleted meanwhile stays generic", email: "user@example.com", outcome: repository.IssueUserNotFound, ready: true},
		{name: "smtp failure stays generic and releases cooldown", email: "user@example.com", outcome: repository.IssueIssued, ready: true,
			sendErr: &SMTPError{Stage: "auth", Code: 535}, wantSend: true, wantReleased: true},
		{name: "db failure on issue stays generic", email: "user@example.com", issueErr: &pgconn.PgError{Code: "40001"}, ready: true},
		{name: "mail disabled", email: "user@example.com", outcome: repository.IssueIssued, wantCode: "mail_unavailable"},
		{name: "mail disabled unknown", email: "nobody@example.com", outcome: repository.IssueIssued, wantCode: "mail_unavailable"},
		{name: "malformed email", email: "no-es-correo", outcome: repository.IssueIssued, ready: true, wantCode: "invalid_email"},
		{name: "header injection", email: "user@example.com\r\nBcc: x@example.com", outcome: repository.IssueIssued, ready: true, wantCode: "invalid_email"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newVerificationStoreFake()
			store.issueOutcome, store.issueErr = tc.outcome, tc.issueErr
			mailer := &verificationMailerFake{ready: tc.ready, err: tc.sendErr}
			svc := newTestVerificationService(t, users, store, mailer, EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/v"})
			err := svc.Request(context.Background(), tc.email)
			var se *ServiceError
			if tc.wantCode != "" {
				if !errors.As(err, &se) || se.Code != tc.wantCode {
					t.Fatalf("want %s, got %v", tc.wantCode, err)
				}
			} else if err != nil {
				t.Fatalf("debe ser genérico (nil), got %v", err)
			}
			if (len(mailer.sent) == 1) != tc.wantSend || len(mailer.sent) > 1 {
				t.Fatalf("envíos=%d", len(mailer.sent))
			}
			if tc.wantSend {
				e := mailer.sent[0]
				if e.Recipient != "user@example.com" || e.ExpiresIn != 24*time.Hour || !strings.Contains(e.Link, "token="+e.Token) {
					t.Fatalf("correo inesperado: %+v", e)
				}
				if !bytes.Equal(store.issued["user-1"], hashVerificationToken(e.Token)) {
					t.Fatal("el hash guardado debe corresponder al token enviado")
				}
			}
			if (len(store.released) == 1) != tc.wantReleased {
				t.Fatalf("release=%v", store.released)
			}
		})
	}
}

func TestEmailVerification_RequestLookupFailureIsInternal(t *testing.T) {
	svc := newTestVerificationService(t, failingLookup{}, newVerificationStoreFake(), &verificationMailerFake{ready: true}, EmailVerificationConfig{})
	err := svc.Request(context.Background(), "user@example.com")
	var se *ServiceError
	if err == nil || errors.As(err, &se) {
		t.Fatalf("un fallo de lectura (igual para cualquier correo) debe ser 500, got %v", err)
	}
}

type failingLookup struct{}

func (failingLookup) GetByEmail(context.Context, string) (*model.User, error) {
	return nil, errors.New("connection refused")
}

func TestEmailVerification_RequestDispatchesOutsideRequest(t *testing.T) {
	users := newMockUserRepo()
	users.usersByEmail["user@example.com"] = &model.User{ID: "user-1", Email: "user@example.com"}
	mailer := &verificationMailerFake{ready: true}
	store := newVerificationStoreFake()
	svc := newTestVerificationService(t, users, store, mailer, EmailVerificationConfig{})
	var queued []func()
	svc.spawn = func(f func()) { queued = append(queued, f) }
	ctx, cancel := context.WithCancel(context.Background())
	if err := svc.Request(ctx, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	cancel() // la petición HTTP terminó
	if len(queued) != 1 || len(mailer.sent) != 0 || len(store.issued) != 0 {
		t.Fatal("ni el token ni el envío deben ocurrir dentro de la petición")
	}
	if len(svc.slots) != 1 {
		t.Fatal("el cupo se ocupa antes de lanzar el trabajo")
	}
	queued[0]()
	if len(mailer.sent) != 1 {
		t.Fatal("el envío debe completarse aunque la petición original se haya cancelado")
	}
	if len(svc.slots) != 0 {
		t.Fatal("el cupo se libera al terminar")
	}
}

func TestEmailVerification_SendForNewUser(t *testing.T) {
	user := &model.User{ID: "user-1", Email: "user@example.com"}
	store := newVerificationStoreFake()
	mailer := &verificationMailerFake{ready: true}
	svc := newTestVerificationService(t, newMockUserRepo(), store, mailer, EmailVerificationConfig{})
	svc.SendForNewUser(context.Background(), user)
	if len(mailer.sent) != 1 || mailer.sent[0].Link != "" || mailer.sent[0].Token == "" {
		t.Fatalf("registro debe enviar la confirmación: %+v", mailer.sent)
	}
	// SMTP deshabilitado: no emite token ni envía nada.
	store2, off := newVerificationStoreFake(), &verificationMailerFake{}
	newTestVerificationService(t, newMockUserRepo(), store2, off, EmailVerificationConfig{}).SendForNewUser(context.Background(), user)
	if len(store2.issued) != 0 || len(off.sent) != 0 {
		t.Fatal("sin SMTP no se emiten tokens")
	}
	svc.SendForNewUser(context.Background(), nil) // no debe entrar en pánico
}

func TestEmailVerification_Confirm(t *testing.T) {
	store := newVerificationStoreFake()
	svc := newTestVerificationService(t, newMockUserRepo(), store, &verificationMailerFake{ready: true}, EmailVerificationConfig{})
	for _, bad := range []string{"", "corto", strings.Repeat("a", 44), strings.Repeat("*", 43)} {
		out, err := svc.Confirm(context.Background(), bad)
		if err != nil || out != repository.ConfirmInvalid {
			t.Fatalf("%q: %v %v", bad, out, err)
		}
	}
	if store.confirmCalls != 0 {
		t.Fatal("un token mal formado no debe llegar a la base")
	}
	for _, want := range []repository.ConfirmOutcome{repository.ConfirmExpired, repository.ConfirmUsed, repository.ConfirmAlreadyVerified, repository.ConfirmVerified} {
		token, hash, _ := generateVerificationToken()
		store.confirmResult[hex.EncodeToString(hash)] = want
		out, err := svc.Confirm(context.Background(), "  "+token+"\n")
		if err != nil || out != want {
			t.Fatalf("want %s got %s %v", want, out, err)
		}
	}
	store.confirmErr = errors.New("db down")
	token, _, _ := generateVerificationToken()
	if _, err := svc.Confirm(context.Background(), token); err == nil {
		t.Fatal("un error de base debe propagarse (500)")
	}
}

// Los logs no deben contener tokens, hashes, correos, contraseñas ni textos
// de error del servidor SMTP o de PostgreSQL.
func TestEmailVerification_LogsNeverLeakSecrets(t *testing.T) {
	buf := captureServiceLogs(t)
	users := newMockUserRepo()
	users.usersByEmail["secret.person@example.com"] = &model.User{ID: "user-1", Email: "secret.person@example.com"}
	store := newVerificationStoreFake()
	smtpText := &textproto.Error{Code: 535, Msg: "5.7.8 Username and Password not accepted for secret.person@example.com pass=hunter2"}
	mailer := &verificationMailerFake{ready: true, err: smtpFailure("auth", smtpText)}
	svc := newTestVerificationService(t, users, store, mailer, EmailVerificationConfig{LinkBaseURL: "https://ti3.example.cl/v"})
	_ = svc.Request(context.Background(), "secret.person@example.com")
	mailer.err = nil
	store.released = nil
	_ = svc.Request(context.Background(), "secret.person@example.com")
	sentToken := mailer.sent[1].Token
	_, _ = svc.Confirm(context.Background(), sentToken)
	_, _ = svc.Confirm(context.Background(), sentToken)
	store.issueErr = &pgconn.PgError{Code: "23505", Message: "duplicate key", Detail: "Key (token_hash)=(\\xdeadbeef) already exists."}
	_ = svc.Request(context.Background(), "secret.person@example.com")
	svc.SendForNewUser(context.Background(), &model.User{ID: "user-1", Email: "secret.person@example.com"})

	out := buf.String()
	for _, e := range mailer.sent {
		for _, forbidden := range []string{e.Token, e.Link, hex.EncodeToString(hashVerificationToken(e.Token))} {
			if strings.Contains(out, forbidden) {
				t.Fatalf("el log filtra un token/hash/enlace:\n%s", out)
			}
		}
	}
	for _, forbidden := range []string{"secret.person", "hunter2", "Username and Password", "deadbeef", "duplicate key"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("el log filtra %q:\n%s", forbidden, out)
		}
	}
	for _, want := range []string{
		"[auth] email-verification op=SendVerificationEmail user_id=user-1 step=send status=error smtp_stage=auth smtp_code=535",
		"op=SendVerificationEmail user_id=user-1 step=send status=ok outcome=sent",
		"op=VerifyEmail user_id=user-1 step=confirm status=ok outcome=verified",
		"op=VerifyEmail user_id=- step=confirm status=rejected outcome=invalid",
		"step=persist status=error sqlstate=23505",
		"op=Register user_id=user-1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en:\n%s", want, out)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		24 * time.Hour: "24 horas", time.Hour: "1 hora", 15 * time.Minute: "15 minutos", 90 * time.Minute: "90 minutos",
	} {
		if got := humanDuration(d); got != want {
			t.Fatalf("%s: %q", d, got)
		}
	}
}
