package service

// Integración contra PostgreSQL del gate de login (EMAIL_VERIFICATION_REQUIRED),
// del backfill 008 y de la confirmación por restablecimiento de contraseña.
// Mismo arnés que auth_integration_test.go (AUTH_TEST_DATABASE_URL, BD vacía
// dedicada, esquema mínimo + db/migrations/*.sql, incluidas 007 y 008).

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
	"golang.org/x/crypto/bcrypt"
)

func insertRawUser(t *testing.T, pool *pgxpool.Pool, email string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO identity.users (email, password_hash) VALUES ($1, 'x') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func verifiedAtOf(t *testing.T, pool *pgxpool.Pool, id string) *time.Time {
	t.Helper()
	var at *time.Time
	if err := pool.QueryRow(context.Background(), `SELECT email_verified_at FROM identity.users WHERE id = $1`, id).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

type backfillRow struct {
	rows   int
	cutoff time.Time
	marked int
}

func readBackfill(t *testing.T, pool *pgxpool.Pool) backfillRow {
	t.Helper()
	var b backfillRow
	ctx := context.Background()
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.email_verification_backfill`).Scan(&b.rows); err != nil {
		t.Fatal(err)
	}
	if b.rows == 1 {
		if err := pool.QueryRow(ctx, `SELECT cutoff, accounts_marked FROM identity.email_verification_backfill`).Scan(&b.cutoff, &b.marked); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

// rearmBackfill simula una base anterior a 008 (sin fila de control).
func rearmBackfill(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `DELETE FROM identity.email_verification_backfill`); err != nil {
		t.Fatal(err)
	}
}

func TestAuthIntegration_EmailVerification_BackfillRunsOnceWithCutoff(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	migration := mustReadMigration(t, "008_email_verification_backfill.sql")

	// Base nueva (arnés: 008 aplicada sin cuentas): fila de control con 0.
	if b := readBackfill(t, pool); b.rows != 1 || b.marked != 0 {
		t.Fatalf("base nueva: %+v", b)
	}

	// Base existente antes de 008: cuentas antiguas sin confirmar y una ya confirmada.
	rearmBackfill(t, pool)
	old1 := insertRawUser(t, pool, "old1@test.invalid")
	old2 := insertRawUser(t, pool, "old2@test.invalid")
	keep := insertRawUser(t, pool, "keep@test.invalid")
	prior := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE identity.users SET email_verified_at = $2 WHERE id = $1`, keep, prior); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, migration); err != nil {
		t.Fatalf("aplicar 008: %v", err)
	}
	b := readBackfill(t, pool)
	if b.rows != 1 || b.marked != 2 {
		t.Fatalf("debe marcar exactamente las 2 cuentas sin fecha: %+v", b)
	}
	for _, id := range []string{old1, old2} {
		if at := verifiedAtOf(t, pool, id); at == nil || !at.Equal(b.cutoff) {
			t.Fatalf("cuenta antigua debe quedar confirmada con la fecha del corte: %v", at)
		}
	}
	if at := verifiedAtOf(t, pool, keep); at == nil || !at.Equal(prior) {
		t.Fatalf("una confirmación previa no se sobrescribe: %v", at)
	}

	// Cuenta creada después del corte: sin confirmar, también tras reaplicar 008.
	newID := insertRawUser(t, pool, "new@test.invalid")
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, migration); err != nil {
			t.Fatalf("reaplicar 008: %v", err)
		}
	}
	if at := verifiedAtOf(t, pool, newID); at != nil {
		t.Fatal("reaplicar 008 nunca confirma cuentas posteriores al corte")
	}
	if again := readBackfill(t, pool); again != b {
		t.Fatalf("la fila de control no cambia al reaplicar: %+v vs %+v", again, b)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity.email_verification_backfill (id, cutoff, accounts_marked) VALUES (false, now(), 0)`); err == nil {
		t.Fatal("la tabla de control admite una sola fila")
	}
}

// El corte espera a las inserciones en curso (quedan antes del corte) y las
// que llegan después del COMMIT quedan sin confirmar.
func TestAuthIntegration_EmailVerification_BackfillLocksConcurrentInserts(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	migration := mustReadMigration(t, "008_email_verification_backfill.sql")
	rearmBackfill(t, pool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var inFlight string
	if err := tx.QueryRow(ctx, `INSERT INTO identity.users (email, password_hash) VALUES ('inflight@test.invalid', 'x') RETURNING id`).Scan(&inFlight); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := pool.Exec(ctx, migration)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("008 debe esperar a la inserción en curso, terminó antes: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("008: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("008 no terminó tras liberar el bloqueo")
	}
	if verifiedAtOf(t, pool, inFlight) == nil {
		t.Fatal("una cuenta confirmada en la base al tomar el bloqueo queda antes del corte")
	}
	after := insertRawUser(t, pool, "after@test.invalid")
	if verifiedAtOf(t, pool, after) != nil {
		t.Fatal("una cuenta posterior al corte queda sin confirmar")
	}
}

func TestAuthIntegration_EmailVerification_BackfillConcurrentRunsSerialize(t *testing.T) {
	pool := setupIntegrationPool(t)
	migration := mustReadMigration(t, "008_email_verification_backfill.sql")
	rearmBackfill(t, pool)
	for i := 0; i < 3; i++ {
		insertRawUser(t, pool, "c"+string(rune('a'+i))+"@test.invalid")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := pool.Exec(context.Background(), migration)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ejecuciones simultáneas de 008 deben serializarse sin error: %v", err)
		}
	}
	if b := readBackfill(t, pool); b.rows != 1 || b.marked != 3 {
		t.Fatalf("una sola ejecución efectiva: %+v", b)
	}
}

func TestAuthIntegration_EmailVerification_BackfillRequires007(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	rearmBackfill(t, pool)
	if _, err := pool.Exec(ctx, `ALTER TABLE identity.users DROP COLUMN email_verified_at`); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(ctx, mustReadMigration(t, "008_email_verification_backfill.sql"))
	if err == nil || !strings.Contains(err.Error(), "007_email_verification.sql") {
		t.Fatalf("008 sin 007 debe fallar con un mensaje claro: %v", err)
	}
	if b := readBackfill(t, pool); b.rows != 0 {
		t.Fatal("el fallo no deja fila de control (transacción revertida)")
	}
}

func TestAuthIntegration_EmailVerification_GateSchemaReady(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	if ok, err := repository.EmailVerificationGateSchemaReady(ctx, pool); err != nil || !ok {
		t.Fatalf("con 007 y 008: %v %v", ok, err)
	}
	rearmBackfill(t, pool)
	if ok, _ := repository.EmailVerificationGateSchemaReady(ctx, pool); ok {
		t.Fatal("sin la fila del backfill el gate no puede activarse")
	}
	if _, err := pool.Exec(ctx, `DROP TABLE identity.email_verification_backfill`); err != nil {
		t.Fatal(err)
	}
	if ok, err := repository.EmailVerificationGateSchemaReady(ctx, pool); err != nil || ok {
		t.Fatalf("sin la tabla 008: %v %v", ok, err)
	}
}

func TestAuthIntegration_EmailVerification_GateLoginRefreshFlow(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	migration := mustReadMigration(t, "008_email_verification_backfill.sql")

	// Cuenta antigua + backfill (base existente antes de 008).
	oldUser := env.register(t, "legacy@test.invalid")
	rearmBackfill(t, env.pool)
	if _, err := env.pool.Exec(ctx, migration); err != nil {
		t.Fatal(err)
	}
	// Cuenta nueva, posterior al corte, con una sesión anterior a activar el gate.
	newUser := env.register(t, "fresh@test.invalid")
	preGate, err := env.auth.Login(ctx, "fresh@test.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("sin gate el login no cambia: %v", err)
	}

	// Activar el gate.
	env.auth.SetEmailVerificationGate(env.repo)
	if _, err := env.auth.Login(ctx, "legacy@test.invalid", "Pass1234"); err != nil {
		t.Fatalf("cuenta antigua backfilleada debe iniciar sesión: %v", err)
	}
	if _, err := env.auth.Login(ctx, "fresh@test.invalid", "Pass1234"); serviceCode(err) != utils.ErrEmailNotVerified {
		t.Fatalf("cuenta nueva sin confirmar: %v", err)
	}
	if _, err := env.auth.Login(ctx, "fresh@test.invalid", "WrongPass1"); serviceCode(err) != utils.ErrInvalidCredentials {
		t.Fatalf("contraseña incorrecta: %v", err)
	}
	if _, err := env.auth.Login(ctx, "ghost@test.invalid", "Pass1234"); serviceCode(err) != utils.ErrInvalidCredentials {
		t.Fatalf("cuenta inexistente: %v", err)
	}
	if _, err := env.auth.Refresh(ctx, preGate.RefreshToken); serviceCode(err) != utils.ErrEmailNotVerified {
		t.Fatalf("la sesión previa no se renueva: %v", err)
	}

	// Token inválido o reemplazado no confirma; el reenvío y la confirmación sí.
	first := env.lastToken(t)
	env.skipCooldown(t, newUser.ID)
	if err := env.svc.Request(ctx, "fresh@test.invalid"); err != nil {
		t.Fatal(err)
	}
	if out, _ := env.svc.Confirm(ctx, first); out != repository.ConfirmInvalid {
		t.Fatalf("token reemplazado: %s", out)
	}
	if _, err := env.auth.Login(ctx, "fresh@test.invalid", "Pass1234"); serviceCode(err) != utils.ErrEmailNotVerified {
		t.Fatal("un token inválido no desbloquea el login")
	}
	if out, _ := env.svc.Confirm(ctx, env.lastToken(t)); out != repository.ConfirmVerified {
		t.Fatalf("confirmar: %s", out)
	}
	login, err := env.auth.Login(ctx, "fresh@test.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("tras confirmar debe iniciar sesión: %v", err)
	}
	if _, err := env.auth.Refresh(ctx, login.RefreshToken); err != nil {
		t.Fatalf("refresh tras confirmar: %v", err)
	}
	if _, err := env.auth.Refresh(ctx, preGate.RefreshToken); err != nil {
		t.Fatalf("la sesión previa continúa tras confirmar (el refresh no se revocó): %v", err)
	}
	if verified, err := env.repo.IsEmailVerified(ctx, "00000000-0000-0000-0000-000000000000"); err != nil || verified {
		t.Fatalf("usuario inexistente: %v %v", verified, err)
	}
	_ = oldUser
}

func TestAuthIntegration_EmailVerification_PasswordResetMarksVerified(t *testing.T) {
	env := newIntegrationVerificationEnv(t)
	ctx := context.Background()
	resets := repository.NewPasswordResetRepository(env.pool)
	resets.EnableEmailVerification()
	issue := func(userID, code string) {
		t.Helper()
		if _, err := env.pool.Exec(ctx, `DELETE FROM identity.password_resets WHERE user_id = $1`, userID); err != nil {
			t.Fatal(err)
		}
		hash, _ := bcrypt.GenerateFromPassword([]byte(code), bcrypt.MinCost)
		if ok, err := resets.Issue(ctx, userID, string(hash)); err != nil || !ok {
			t.Fatalf("issue: %v %v", ok, err)
		}
	}
	newHash := func(p string) string {
		h, _ := bcrypt.GenerateFromPassword([]byte(p), bcrypt.MinCost)
		return string(h)
	}

	user := env.register(t, "reset@test.invalid")
	issue(user.ID, "12345678")
	// Código incorrecto: ni contraseña ni confirmación.
	if ok, err := resets.Complete(ctx, "reset@test.invalid", "00000000", newHash("NewPass99")); err != nil || ok {
		t.Fatalf("código incorrecto: %v %v", ok, err)
	}
	if env.verifiedAt(t, user.ID) != nil {
		t.Fatal("un código incorrecto no confirma el correo")
	}
	// Código correcto: cambia la contraseña y confirma, en la misma sentencia.
	if ok, err := resets.Complete(ctx, "reset@test.invalid", "12345678", newHash("NewPass99")); err != nil || !ok {
		t.Fatalf("reset: %v %v", ok, err)
	}
	if env.verifiedAt(t, user.ID) == nil {
		t.Fatal("un restablecimiento exitoso confirma el correo")
	}
	env.auth.SetEmailVerificationGate(env.repo)
	if _, err := env.auth.Login(ctx, "reset@test.invalid", "NewPass99"); err != nil {
		t.Fatalf("tras restablecer, login con el gate activo: %v", err)
	}
	if _, err := env.auth.Login(ctx, "reset@test.invalid", "Pass1234"); serviceCode(err) != utils.ErrInvalidCredentials {
		t.Fatal("la contraseña anterior deja de servir")
	}

	// Una confirmación previa se conserva.
	confirmed := env.register(t, "kept@test.invalid")
	prior := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	if _, err := env.pool.Exec(ctx, `UPDATE identity.users SET email_verified_at = $2 WHERE id = $1`, confirmed.ID, prior); err != nil {
		t.Fatal(err)
	}
	issue(confirmed.ID, "87654321")
	if ok, err := resets.Complete(ctx, "kept@test.invalid", "87654321", newHash("NewPass99")); err != nil || !ok {
		t.Fatal(err)
	}
	if at := env.verifiedAt(t, confirmed.ID); at == nil || !at.Equal(prior) {
		t.Fatalf("la fecha previa se conserva: %v", at)
	}

	// Sin EnableEmailVerification (migración 007 ausente al arrancar): igual que antes.
	plain := repository.NewPasswordResetRepository(env.pool)
	other := env.register(t, "plain@test.invalid")
	hash, _ := bcrypt.GenerateFromPassword([]byte("11112222"), bcrypt.MinCost)
	if ok, err := plain.Issue(ctx, other.ID, string(hash)); err != nil || !ok {
		t.Fatal(err)
	}
	if ok, err := plain.Complete(ctx, "plain@test.invalid", "11112222", newHash("NewPass99")); err != nil || !ok {
		t.Fatal(err)
	}
	if env.verifiedAt(t, other.ID) != nil {
		t.Fatal("sin la opción activada el restablecimiento no toca email_verified_at")
	}
}
