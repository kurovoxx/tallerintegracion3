package service

// Integración de la confirmación de correo contra PostgreSQL real (mismo
// arnés que auth_integration_test.go: BD vacía y dedicada, AUTH_TEST_DATABASE_URL,
// esquema mínimo + db/migrations/*.sql, incluida 007_email_verification.sql).

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

type integrationVerificationEnv struct {
	pool   *pgxpool.Pool
	auth   *AuthService
	svc    *EmailVerificationService
	repo   *repository.EmailVerificationRepository
	mailer *verificationMailerFake
}

func newIntegrationVerificationEnv(t *testing.T) *integrationVerificationEnv {
	t.Helper()
	pool := setupIntegrationPool(t)
	env := &integrationVerificationEnv{pool: pool, repo: repository.NewEmailVerificationRepository(pool), mailer: &verificationMailerFake{ready: true}}
	env.svc = newTestVerificationService(t, repository.NewUserRepository(pool), env.repo, env.mailer, EmailVerificationConfig{})
	env.auth = newIntegrationAuthService(pool)
	env.auth.SetEmailVerifier(env.svc)
	return env
}

func (e *integrationVerificationEnv) register(t *testing.T, email string) *model.User {
	t.Helper()
	u, err := e.auth.Register(context.Background(), email, "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}
	return u
}

func (e *integrationVerificationEnv) lastToken(t *testing.T) string {
	t.Helper()
	e.mailer.mu.Lock()
	defer e.mailer.mu.Unlock()
	if len(e.mailer.sent) == 0 {
		t.Fatal("no se envió ningún correo")
	}
	return e.mailer.sent[len(e.mailer.sent)-1].Token
}

func (e *integrationVerificationEnv) sentCount() int {
	e.mailer.mu.Lock()
	defer e.mailer.mu.Unlock()
	return len(e.mailer.sent)
}

// Simula que pasó el enfriamiento entre envíos.
func (e *integrationVerificationEnv) skipCooldown(t *testing.T, userID string) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), `UPDATE identity.email_verifications SET requested_at = now() - interval '61 seconds' WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
}

func (e *integrationVerificationEnv) verifiedAt(t *testing.T, userID string) *time.Time {
	t.Helper()
	var at *time.Time
	if err := e.pool.QueryRow(context.Background(), `SELECT email_verified_at FROM identity.users WHERE id = $1`, userID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestAuthIntegration_EmailVerification_MigrationSchema(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	ok, err := repository.EmailVerificationSchemaReady(ctx, pool)
	if err != nil || !ok {
		t.Fatalf("007 debe crear tabla y columna: %v %v", ok, err)
	}
	// Reaplicar la migración es seguro (idempotente).
	migration := mustReadMigration(t, "007_email_verification.sql")
	if _, err := pool.Exec(ctx, migration); err != nil {
		t.Fatalf("reaplicar 007: %v", err)
	}
	// Restricciones: hash de 32 bytes, único y FK con cascada.
	if _, err := pool.Exec(ctx, `INSERT INTO identity.users (id, email, password_hash) VALUES ('00000000-0000-0000-0000-0000000000a1', 'schema@test.invalid', 'x')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity.email_verifications (user_id, token_hash, expires_at) VALUES ('00000000-0000-0000-0000-0000000000a1', '\x00', now())`); err == nil {
		t.Fatal("un hash que no mide 32 bytes debe rechazarse")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity.email_verifications (user_id, token_hash, expires_at) VALUES ('00000000-0000-0000-0000-000000000bad', $1, now())`, make([]byte, 32)); err == nil {
		t.Fatal("la FK a identity.users debe exigirse")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity.email_verifications (user_id, token_hash, expires_at) VALUES ('00000000-0000-0000-0000-0000000000a1', $1, now())`, make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM identity.users WHERE id = '00000000-0000-0000-0000-0000000000a1'`); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM identity.email_verifications`).Scan(&left)
	if left != 0 {
		t.Fatal("borrar la cuenta debe borrar su token (ON DELETE CASCADE)")
	}
}

func TestAuthIntegration_EmailVerification_RegisterPersistsHashAndConfirms(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	user := env.register(t, "flow@test.invalid")
	token := env.lastToken(t)

	var stored []byte
	var expiresSecs float64
	var sends int
	if err := env.pool.QueryRow(ctx, `SELECT token_hash, sends_in_window, EXTRACT(EPOCH FROM expires_at - now())::float8 FROM identity.email_verifications WHERE user_id = $1`, user.ID).Scan(&stored, &sends, &expiresSecs); err != nil {
		t.Fatalf("token no persistido: %v", err)
	}
	if !bytes.Equal(stored, hashVerificationToken(token)) || bytes.Contains(stored, []byte(token)) || sends != 1 {
		t.Fatal("solo debe guardarse el SHA-256 del token")
	}
	expiresIn := time.Duration(expiresSecs * float64(time.Second))
	if expiresIn < 23*time.Hour || expiresIn > 24*time.Hour+time.Minute {
		t.Fatalf("vigencia por defecto 24 h, got %s", expiresIn)
	}
	if env.verifiedAt(t, user.ID) != nil {
		t.Fatal("una cuenta nueva no está confirmada")
	}
	out, err := env.svc.Confirm(ctx, token)
	if err != nil || out != repository.ConfirmVerified {
		t.Fatalf("confirm: %v %v", out, err)
	}
	if env.verifiedAt(t, user.ID) == nil {
		t.Fatal("email_verified_at debe quedar fijado")
	}
	// Reutilizar el token: no vuelve a modificar nada y responde «ya confirmado».
	before := *env.verifiedAt(t, user.ID)
	if out, err := env.svc.Confirm(ctx, token); err != nil || out != repository.ConfirmAlreadyVerified {
		t.Fatalf("reuso: %v %v", out, err)
	}
	if !env.verifiedAt(t, user.ID).Equal(before) {
		t.Fatal("el reuso no debe tocar email_verified_at")
	}
	// Login y refresh no cambian (sin gate en esta tarea).
	login, err := env.auth.Login(ctx, "flow@test.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, err := env.auth.Refresh(ctx, login.RefreshToken); err != nil {
		t.Fatalf("refresh: %v", err)
	}
}

func TestAuthIntegration_EmailVerification_UnverifiedAccountCanStillLogin(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	env.register(t, "nogate@test.invalid")
	if _, err := env.auth.Login(context.Background(), "nogate@test.invalid", "Pass1234"); err != nil {
		t.Fatalf("el login no se bloquea por correo sin confirmar en esta tarea: %v", err)
	}
}

func TestAuthIntegration_EmailVerification_InvalidExpiredAndReplacedTokens(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	user := env.register(t, "tokens@test.invalid")
	first := env.lastToken(t)

	// Token bien formado pero inexistente.
	unknown, _, _ := generateVerificationToken()
	if out, err := env.svc.Confirm(ctx, unknown); err != nil || out != repository.ConfirmInvalid {
		t.Fatalf("inexistente: %v %v", out, err)
	}
	// Reenvío dentro del enfriamiento: no emite ni envía.
	if err := env.svc.Request(ctx, "tokens@test.invalid"); err != nil || env.sentCount() != 1 {
		t.Fatalf("enfriamiento: err=%v envíos=%d", err, env.sentCount())
	}
	// Reenvío pasado el enfriamiento: nuevo token que invalida el anterior.
	env.skipCooldown(t, user.ID)
	if err := env.svc.Request(ctx, "TOKENS@test.invalid"); err != nil || env.sentCount() != 2 {
		t.Fatalf("reenvío: err=%v envíos=%d", err, env.sentCount())
	}
	second := env.lastToken(t)
	if out, _ := env.svc.Confirm(ctx, first); out != repository.ConfirmInvalid {
		t.Fatalf("el token anterior debe quedar invalidado, got %s", out)
	}
	// Expirado.
	if _, err := env.pool.Exec(ctx, `UPDATE identity.email_verifications SET expires_at = now() - interval '1 second' WHERE user_id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if out, err := env.svc.Confirm(ctx, second); err != nil || out != repository.ConfirmExpired {
		t.Fatalf("expirado: %v %v", out, err)
	}
	if env.verifiedAt(t, user.ID) != nil {
		t.Fatal("un token expirado no confirma")
	}
	// Tras expirar, un reenvío emite uno válido.
	env.skipCooldown(t, user.ID)
	if err := env.svc.Request(ctx, "tokens@test.invalid"); err != nil {
		t.Fatal(err)
	}
	if out, _ := env.svc.Confirm(ctx, env.lastToken(t)); out != repository.ConfirmVerified {
		t.Fatalf("nuevo token tras expirar: %s", out)
	}
	// Cuenta ya confirmada: no se emiten más tokens.
	env.skipCooldown(t, user.ID)
	sent := env.sentCount()
	if err := env.svc.Request(ctx, "tokens@test.invalid"); err != nil || env.sentCount() != sent {
		t.Fatal("una cuenta confirmada no recibe más correos")
	}
	if outcome, err := env.repo.Issue(ctx, user.ID, make([]byte, 32), time.Hour); err != nil || outcome != repository.IssueAlreadyVerified {
		t.Fatalf("issue a cuenta confirmada: %v %v", outcome, err)
	}
	if outcome, err := env.repo.Issue(ctx, "00000000-0000-0000-0000-000000000000", make([]byte, 32), time.Hour); err != nil || outcome != repository.IssueUserNotFound {
		t.Fatalf("issue a cuenta inexistente: %v %v", outcome, err)
	}
}

func TestAuthIntegration_EmailVerification_TokenOnlyVerifiesItsOwnAccount(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	alice := env.register(t, "alice@test.invalid")
	aliceToken := env.lastToken(t)
	bob := env.register(t, "bob@test.invalid")
	if out, _ := env.svc.Confirm(ctx, aliceToken); out != repository.ConfirmVerified {
		t.Fatal(out)
	}
	if env.verifiedAt(t, alice.ID) == nil || env.verifiedAt(t, bob.ID) != nil {
		t.Fatal("el token de Alice solo confirma la cuenta de Alice")
	}
}

func TestAuthIntegration_EmailVerification_HourlyLimit(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	user := env.register(t, "limit@test.invalid") // envío 1
	for i := 2; i <= repository.EmailVerificationMaxPerWindow; i++ {
		env.skipCooldown(t, user.ID)
		if err := env.svc.Request(ctx, "limit@test.invalid"); err != nil {
			t.Fatal(err)
		}
	}
	if env.sentCount() != repository.EmailVerificationMaxPerWindow {
		t.Fatalf("envíos=%d", env.sentCount())
	}
	env.skipCooldown(t, user.ID)
	if err := env.svc.Request(ctx, "limit@test.invalid"); err != nil || env.sentCount() != repository.EmailVerificationMaxPerWindow {
		t.Fatal("el sexto envío en la misma hora debe bloquearse (respuesta igual de genérica)")
	}
	// Pasada la ventana, se reinicia el contador.
	if _, err := env.pool.Exec(ctx, `UPDATE identity.email_verifications SET window_started_at = now() - interval '61 minutes', requested_at = now() - interval '61 seconds' WHERE user_id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if err := env.svc.Request(ctx, "limit@test.invalid"); err != nil || env.sentCount() != repository.EmailVerificationMaxPerWindow+1 {
		t.Fatal("nueva ventana: debe volver a enviar")
	}
	var sends int
	_ = env.pool.QueryRow(ctx, `SELECT sends_in_window FROM identity.email_verifications WHERE user_id = $1`, user.ID).Scan(&sends)
	if sends != 1 {
		t.Fatalf("contador de la nueva ventana = %d", sends)
	}
}

func TestAuthIntegration_EmailVerification_SMTPFailureAllowsImmediateRetry(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	env.mailer.err = &SMTPError{Stage: "dial"}
	user := env.register(t, "smtpfail@test.invalid") // el registro no falla
	failedToken := env.lastToken(t)
	var count int
	_ = env.pool.QueryRow(ctx, `SELECT count(*) FROM identity.users WHERE email = 'smtpfail@test.invalid'`).Scan(&count)
	if count != 1 {
		t.Fatal("la cuenta debe existir una sola vez")
	}
	// Reintento inmediato (sin esperar el enfriamiento) cuando SMTP se recupera.
	env.mailer.err = nil
	if err := env.svc.Request(ctx, "smtpfail@test.invalid"); err != nil || env.sentCount() != 2 {
		t.Fatalf("reintento inmediato: err=%v envíos=%d", err, env.sentCount())
	}
	if out, _ := env.svc.Confirm(ctx, failedToken); out != repository.ConfirmInvalid {
		t.Fatal("el token del envío fallido queda reemplazado")
	}
	if out, _ := env.svc.Confirm(ctx, env.lastToken(t)); out != repository.ConfirmVerified {
		t.Fatal("el token reenviado debe confirmar")
	}
	if env.verifiedAt(t, user.ID) == nil {
		t.Fatal("cuenta confirmada")
	}
	// ReleaseCooldown no actúa sobre un token consumido.
	if err := env.repo.ReleaseCooldown(ctx, user.ID, hashVerificationToken(env.lastToken(t))); err != nil {
		t.Fatal(err)
	}
}

// Con los 4 cupos ocupados, registro y reenvío se descartan antes de tocar
// la base: no queda token huérfano ni se gasta enfriamiento o tope por hora,
// y el reenvío funciona en cuanto hay capacidad.
func TestAuthIntegration_EmailVerification_BusyDispatchLeavesNoTraceAndResendWorks(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	fill := func() {
		for i := 0; i < EmailVerificationMaxConcurrentSends; i++ {
			env.svc.slots <- struct{}{}
		}
	}
	drain := func() {
		for i := 0; i < EmailVerificationMaxConcurrentSends; i++ {
			<-env.svc.slots
		}
	}

	// Registro con el sistema ocupado: cuenta creada, sin token.
	fill()
	busy := env.register(t, "busy@test.invalid")
	var rows int
	_ = env.pool.QueryRow(ctx, `SELECT count(*) FROM identity.email_verifications WHERE user_id = $1`, busy.ID).Scan(&rows)
	if rows != 0 || env.sentCount() != 0 {
		t.Fatalf("un despacho descartado no deja token: filas=%d envíos=%d", rows, env.sentCount())
	}
	drain()
	if err := env.svc.Request(ctx, "busy@test.invalid"); err != nil || env.sentCount() != 1 {
		t.Fatalf("reenvío inmediato tras el descarte: err=%v envíos=%d", err, env.sentCount())
	}
	if out, _ := env.svc.Confirm(ctx, env.lastToken(t)); out != repository.ConfirmVerified {
		t.Fatalf("el token del reenvío confirma: %s", out)
	}

	// Reenvío descartado: el token vigente, el enfriamiento y el contador no cambian.
	other := env.register(t, "busy2@test.invalid")
	env.skipCooldown(t, other.ID)
	snapshot := func() (hash []byte, requested time.Time, sends int) {
		if err := env.pool.QueryRow(ctx, `SELECT token_hash, requested_at, sends_in_window FROM identity.email_verifications WHERE user_id = $1`, other.ID).Scan(&hash, &requested, &sends); err != nil {
			t.Fatal(err)
		}
		return
	}
	h1, r1, s1 := snapshot()
	sent := env.sentCount()
	fill()
	if err := env.svc.Request(ctx, "busy2@test.invalid"); err != nil {
		t.Fatalf("respuesta genérica también con el sistema ocupado: %v", err)
	}
	h2, r2, s2 := snapshot()
	if !bytes.Equal(h1, h2) || !r1.Equal(r2) || s1 != s2 || env.sentCount() != sent {
		t.Fatal("un reenvío descartado no debe tocar token, enfriamiento ni tope")
	}
	if out, _ := env.svc.Confirm(ctx, env.lastToken(t)); out != repository.ConfirmVerified {
		t.Fatalf("el token anterior sigue siendo válido tras el descarte: %s", out)
	}
	drain()
}

func TestAuthIntegration_EmailVerification_ConcurrentConfirmConsumesOnce(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	user := env.register(t, "race@test.invalid")
	token := env.lastToken(t)
	const workers = 8
	results := make(chan repository.ConfirmOutcome, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out, err := env.svc.Confirm(ctx, token)
			if err != nil {
				t.Error(err)
			}
			results <- out
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	verified, already := 0, 0
	for out := range results {
		switch out {
		case repository.ConfirmVerified:
			verified++
		case repository.ConfirmAlreadyVerified:
			already++
		default:
			t.Fatalf("resultado inesperado %s", out)
		}
	}
	if verified != 1 || already != workers-1 {
		t.Fatalf("el token se canjea exactamente una vez: verified=%d already=%d", verified, already)
	}
	if env.verifiedAt(t, user.ID) == nil {
		t.Fatal("cuenta confirmada")
	}
}

func TestAuthIntegration_EmailVerification_ConcurrentRequestsIssueOnce(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	user := env.register(t, "burst@test.invalid")
	env.skipCooldown(t, user.ID)
	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := env.svc.Request(ctx, "burst@test.invalid"); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if env.sentCount() != 2 {
		t.Fatalf("una ráfaga simultánea debe emitir un solo token nuevo: envíos=%d", env.sentCount())
	}
	var sends int
	_ = env.pool.QueryRow(ctx, `SELECT sends_in_window FROM identity.email_verifications WHERE user_id = $1`, user.ID).Scan(&sends)
	if sends != 2 {
		t.Fatalf("sends_in_window=%d", sends)
	}
	if out, _ := env.svc.Confirm(ctx, env.lastToken(t)); out != repository.ConfirmVerified {
		t.Fatal("el token emitido en la ráfaga es el vigente")
	}
}

// Emisión y canje simultáneos para la misma cuenta: mismo orden de bloqueo,
// sin interbloqueos ni errores, y el estado final es coherente.
func TestAuthIntegration_EmailVerification_ConcurrentIssueAndConfirm(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		user := env.register(t, "mix"+string(rune('a'+i))+"@test.invalid")
		token := env.lastToken(t)
		env.skipCooldown(t, user.ID)
		var wg sync.WaitGroup
		var confirmOut repository.ConfirmOutcome
		var issueOut repository.IssueOutcome
		var confirmErr, issueErr error
		wg.Add(2)
		go func() { defer wg.Done(); confirmOut, confirmErr = env.svc.Confirm(ctx, token) }()
		go func() {
			defer wg.Done()
			_, hash, _ := generateVerificationToken()
			issueOut, issueErr = env.repo.Issue(ctx, user.ID, hash, time.Hour)
		}()
		wg.Wait()
		if confirmErr != nil || issueErr != nil {
			t.Fatalf("errores concurrentes: %v %v", confirmErr, issueErr)
		}
		verified := env.verifiedAt(t, user.ID) != nil
		switch {
		case confirmOut == repository.ConfirmVerified && issueOut == repository.IssueAlreadyVerified && verified:
		case confirmOut == repository.ConfirmInvalid && issueOut == repository.IssueIssued && !verified:
		default:
			t.Fatalf("estado incoherente: confirm=%s issue=%s verified=%v", confirmOut, issueOut, verified)
		}
	}
}

func TestAuthIntegration_EmailVerification_DatabaseFailures(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	user := env.register(t, "dbfail@test.invalid")
	token := env.lastToken(t)
	// Sin la tabla (p. ej. migración 007 sin aplicar): el registro sigue
	// funcionando, el reenvío responde genérico y el canje es un error interno.
	if _, err := env.pool.Exec(ctx, `ALTER TABLE identity.email_verifications RENAME TO email_verifications_off`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = env.pool.Exec(context.Background(), `ALTER TABLE identity.email_verifications_off RENAME TO email_verifications`)
	})
	sent := env.sentCount()
	env.register(t, "dbfail2@test.invalid")
	if err := env.svc.Request(ctx, "dbfail@test.invalid"); err != nil {
		t.Fatalf("respuesta genérica aunque falle la persistencia: %v", err)
	}
	if env.sentCount() != sent {
		t.Fatal("sin persistir el token no se envía correo")
	}
	if _, err := env.svc.Confirm(ctx, token); err == nil {
		t.Fatal("un fallo de base en el canje debe propagarse como error interno")
	}
	if ok, _ := repository.EmailVerificationSchemaReady(ctx, env.pool); ok {
		t.Fatal("la comprobación de arranque debe detectar la falta de 007")
	}
	// Restaurada la tabla, el token original sigue sirviendo (nada quedó a medias).
	if _, err := env.pool.Exec(ctx, `ALTER TABLE identity.email_verifications_off RENAME TO email_verifications`); err != nil {
		t.Fatal(err)
	}
	if out, err := env.svc.Confirm(ctx, token); err != nil || out != repository.ConfirmVerified {
		t.Fatalf("tras restaurar: %v %v", out, err)
	}
	if env.verifiedAt(t, user.ID) == nil {
		t.Fatal("confirmada")
	}
	// Base inaccesible (pool propio cerrado; el del arnés debe seguir vivo
	// para su DROP SCHEMA): lectura fallida → error 500, igual para cualquier correo.
	down, err := pgxpool.New(ctx, os.Getenv("AUTH_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	down.Close()
	svcDown := newTestVerificationService(t, repository.NewUserRepository(down), repository.NewEmailVerificationRepository(down), env.mailer, EmailVerificationConfig{})
	if err := svcDown.Request(ctx, "dbfail@test.invalid"); err == nil || errors.As(err, new(*ServiceError)) {
		t.Fatalf("base caída: %v", err)
	}
	if _, err := svcDown.Confirm(ctx, token); err == nil {
		t.Fatal("canje con base caída: error interno")
	}
}

func mustReadMigration(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../db/migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
