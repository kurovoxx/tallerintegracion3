package service

// Suite de integración handler+service+repository contra Postgres real.
//
// Cómo levantarlo (BD de prueba dedicada y vacía, nunca la de desarrollo):
//
//   docker run -d --name auth-test-db -e POSTGRES_PASSWORD=postgres \
//     -e POSTGRES_USER=postgres -e POSTGRES_DB=auth_test \
//     -p 5433:5432 postgres:16-alpine
//   export AUTH_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5433/auth_test?sslmode=disable'
//   go test ./internal/service/ -run TestAuthIntegration -count=1 -v
//   docker rm -f auth-test-db
//
// Sin AUTH_TEST_DATABASE_URL el test hace skip (único skip permitido).
// Aplica el schema identity mínimo (subset de docker/postgres/init.sql) +
// db/migrations/*.sql en la BD vacía y limpia con DROP SCHEMA identity CASCADE.
// Nota: comparte la BD con TestPasswordResetRepository (mismo CREATE/DROP de
// SCHEMA identity); si fijas ambas URLs a la misma BD, corre con -p 1.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

const integrationSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE identity.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(255) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identity.refresh_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    token_hash varchar(255) NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON identity.refresh_tokens(user_id);
CREATE TABLE identity.profiles (
    user_id uuid PRIMARY KEY REFERENCES identity.users(id) ON DELETE CASCADE,
    display_name varchar(100) NOT NULL,
    photo_url varchar(500),
    phone varchar(30),
    institution varchar(200),
    description text,
    visibility varchar(20) NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private')),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identity.oauth_connections (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    provider varchar(30) NOT NULL CHECK (provider IN ('google_calendar', 'google_drive')),
    access_token text NOT NULL,
    refresh_token text,
    expires_at timestamptz,
    external_account_email varchar(255),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, provider)
);
`

func setupIntegrationPool(t *testing.T) *pgxpool.Pool {
	return setupIntegrationPoolWithMaxConns(t, 0)
}

func setupIntegrationPoolWithMaxConns(t *testing.T, maxConns int32) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("AUTH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AUTH_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatalf("parse test db config: %v", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
		cfg.MinConns = 0
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	// Exigir BD vacía: si identity ya existe, no tocar datos existentes.
	if _, err := pool.Exec(ctx, `CREATE SCHEMA identity`); err != nil {
		pool.Close()
		t.Fatalf("requires an empty test database (CREATE SCHEMA identity failed): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA identity CASCADE`)
		pool.Close()
	})
	if _, err := pool.Exec(ctx, integrationSchemaSQL); err != nil {
		t.Fatalf("apply base schema: %v", err)
	}
	// Aplicar migraciones del repo (ej: 001_password_resets.sql).
	matches, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil {
		t.Fatalf("glob migrations: %v", err)
	}
	for _, m := range matches {
		sqlBytes, err := os.ReadFile(m)
		if err != nil {
			t.Fatalf("read %s: %v", m, err)
		}
		if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
			t.Fatalf("apply %s: %v", m, err)
		}
	}
	return pool
}

func newIntegrationAuthService(pool *pgxpool.Pool) *AuthService {
	return newIntegrationAuthServiceWithRefreshRepo(pool, repository.NewRefreshTokenRepository(pool))
}

func newIntegrationAuthServiceWithRefreshRepo(pool *pgxpool.Pool, refreshRepo RefreshTokenRepository) *AuthService {
	jwtSvc, err := NuevoJWTService(ConfiguracionJWT{
		ClaveSecreta: []byte("integration-test-secret-32-chars-long"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		panic(err)
	}
	return NewAuthService(
		repository.NewUserRepository(pool),
		refreshRepo,
		jwtSvc, 900, 604800,
	)
}

// Flujo register -> login -> refresh -> logout contra Postgres real.
func TestAuthIntegration_RegisterLoginRefreshLogout(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	svc := newIntegrationAuthService(pool)

	user, err := svc.Register(ctx, "integ@test.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.ID == "" || user.Email != "integ@test.invalid" {
		t.Fatalf("register payload inesperado: %+v", user)
	}
	// Perfil creado en la misma tx.
	var display string
	if err := pool.QueryRow(ctx, `SELECT display_name FROM identity.profiles WHERE user_id=$1`, user.ID).Scan(&display); err != nil || display == "" {
		t.Fatalf("profile no creado en tx: %v display=%q", err, display)
	}

	// Duplicado -> email_taken (409 a nivel HTTP).
	if _, err := svc.Register(ctx, "integ@test.invalid", "Pass1234", nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("duplicado debe fallar")
	} else if se, ok := err.(*ServiceError); !ok || se.Code != "email_taken" {
		t.Fatalf("duplicado debe ser email_taken, got %v", err)
	}

	// Login inválido -> 401.
	if _, err := svc.Login(ctx, "integ@test.invalid", "WrongPass1"); err == nil {
		t.Fatal("login inválido debe fallar")
	} else if se, ok := err.(*ServiceError); !ok || se.Code != "invalid_credentials" {
		t.Fatalf("login inválido debe ser invalid_credentials, got %v", err)
	}

	login, err := svc.Login(ctx, "integ@test.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if login.AccessToken == "" || login.RefreshToken == "" || login.ExpiresIn != 900 {
		t.Fatalf("login payload incompleto: %+v", login)
	}
	if _, err := svc.jwt.ValidarAccessToken(login.AccessToken); err != nil {
		t.Fatalf("access token inválido: %v", err)
	}

	refreshed, err := svc.Refresh(ctx, login.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		t.Fatalf("refresh payload incompleto: %+v", refreshed)
	}
	// Reutilizar el viejo debe fallar (rotación).
	if _, err := svc.Refresh(ctx, login.RefreshToken); err == nil {
		t.Fatal("reutilizar refresh viejo debe fallar")
	}

	// Logout idempotente: activo -> nil, repetido -> nil, inexistente -> nil.
	if err := svc.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if err := svc.Logout(ctx, refreshed.RefreshToken); err != nil {
		t.Fatalf("logout repetido debe ser idempotente: %v", err)
	}
	if err := svc.Logout(ctx, "noexiste"+refreshed.RefreshToken); err != nil {
		t.Fatalf("logout inexistente debe ser idempotente: %v", err)
	}
	// Refresh tras logout -> 401 (revocado).
	if _, err := svc.Refresh(ctx, refreshed.RefreshToken); err == nil {
		t.Fatal("refresh tras logout debe fallar con 401")
	}
}

// Migraciones aplicadas desde cero en BD vacía (password_resets existe).
func TestAuthIntegration_MigracionesAplicadas(t *testing.T) {
	pool := setupIntegrationPool(t)
	var columnExists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='identity' AND table_name='refresh_tokens' AND column_name='token_fingerprint')`).Scan(&columnExists); err != nil {
		t.Fatal(err)
	}
	if !columnExists {
		t.Fatal("002_refresh_token_fingerprint debe añadir la huella al esquema preexistente")
	}
	var indexExists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE schemaname='identity' AND indexname='idx_refresh_tokens_fingerprint')`).Scan(&indexExists); err != nil {
		t.Fatal(err)
	}
	if !indexExists {
		t.Fatal("la migración debe crear el índice parcial de huella")
	}
	var resetTableExists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='identity' AND table_name='password_resets')`).Scan(&resetTableExists); err != nil {
		t.Fatal(err)
	}
	if !resetTableExists {
		t.Fatal("identity.password_resets debe existir tras aplicar db/migrations/*.sql")
	}
}

func TestAuthIntegration_LegacyRefreshAndExpiration(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	svc := newIntegrationAuthService(pool)
	user, err := svc.Register(ctx, "legacy@test.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	legacyRaw, legacyHash, err := repository.GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	legacyID, err := repository.NewRefreshTokenRepository(pool).Create(ctx, user.ID, legacyHash, nil, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("insert legacy token: %v", err)
	}
	rotated, err := svc.Refresh(ctx, legacyRaw)
	if err != nil {
		t.Fatalf("refresh legacy token: %v", err)
	}
	var oldRevoked bool
	if err := pool.QueryRow(ctx, `SELECT revoked FROM identity.refresh_tokens WHERE id=$1`, legacyID).Scan(&oldRevoked); err != nil || !oldRevoked {
		t.Fatalf("legacy token should be revoked atomically: revoked=%v err=%v", oldRevoked, err)
	}
	var newFingerprint []byte
	if err := pool.QueryRow(ctx, `SELECT token_fingerprint FROM identity.refresh_tokens WHERE token_fingerprint=$1`, repository.FingerprintRawToken(rotated.RefreshToken)).Scan(&newFingerprint); err != nil {
		t.Fatalf("rotated token fingerprint was not persisted: %v", err)
	}
	if !bytes.Equal(newFingerprint, repository.FingerprintRawToken(rotated.RefreshToken)) {
		t.Fatal("rotated token fingerprint does not match the raw token")
	}
	if _, err := svc.Refresh(ctx, legacyRaw); err == nil {
		t.Fatal("reusing a legacy token after rotation must be rejected")
	}

	expiredRaw, expiredHash, err := repository.GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewRefreshTokenRepository(pool).Create(ctx, user.ID, expiredHash, repository.FingerprintRawToken(expiredRaw), time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("insert expired fingerprint token: %v", err)
	}
	if _, err := svc.Refresh(ctx, expiredRaw); err == nil {
		t.Fatal("expired fingerprint token must be rejected")
	} else if serviceErr, ok := err.(*ServiceError); !ok || serviceErr.Code != "token_expired" {
		t.Fatalf("expired fingerprint token should preserve token_expired, got %v", err)
	}

	expiredLegacyRaw, expiredLegacyHash, err := repository.GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.NewRefreshTokenRepository(pool).Create(ctx, user.ID, expiredLegacyHash, nil, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("insert expired legacy token: %v", err)
	}
	if _, err := svc.Refresh(ctx, expiredLegacyRaw); err == nil {
		t.Fatal("expired legacy token must be rejected")
	}
}

func TestAuthIntegration_ConcurrentRefreshSameTokenRotatesOnce(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	arrivals := make(chan struct{}, 2)
	release := make(chan struct{})
	refreshRepo := &refreshLookupBarrier{
		RefreshTokenRepository: repository.NewRefreshTokenRepository(pool),
		arrivals:               arrivals,
		release:                release,
	}
	svc := newIntegrationAuthServiceWithRefreshRepo(pool, refreshRepo)
	if _, err := svc.Register(ctx, "concurrent@test.invalid", "Pass1234", nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	login, err := svc.Login(ctx, "concurrent@test.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	start := make(chan struct{})
	type outcome struct{ err error }
	outcomes := make(chan outcome, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, refreshErr := svc.Refresh(ctx, login.RefreshToken)
			outcomes <- outcome{err: refreshErr}
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		select {
		case <-arrivals:
		case <-time.After(5 * time.Second):
			close(release)
			workers.Wait()
			t.Fatal("both refresh requests must find the same old token before either rotates it")
		}
	}
	close(release)
	workers.Wait()
	close(outcomes)

	successes, failures := 0, 0
	for result := range outcomes {
		if result.err == nil {
			successes++
			continue
		}
		failures++
		if serviceErr, ok := result.err.(*ServiceError); !ok || serviceErr.Code != "invalid_token" {
			t.Fatalf("losing concurrent refresh should be rejected as invalid_token, got %v", result.err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatalf("same token should rotate exactly once, successes=%d failures=%d", successes, failures)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id=(SELECT id FROM identity.users WHERE email=$1)`, "concurrent@test.invalid").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected login row plus exactly one rotated row, got %d rows", count)
	}
}

type refreshLookupBarrier struct {
	*repository.RefreshTokenRepository
	arrivals chan<- struct{}
	release  <-chan struct{}
}

func (r *refreshLookupBarrier) FindByRawToken(ctx context.Context, raw string) (*repository.RefreshToken, error) {
	token, err := r.RefreshTokenRepository.FindByRawToken(ctx, raw)
	if err != nil || token == nil {
		return token, err
	}
	r.arrivals <- struct{}{}
	<-r.release
	return token, nil
}

func TestAuthIntegration_FingerprintLookupUsesIndexWithLegacyRows(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	svc := newIntegrationAuthService(pool)
	if _, err := svc.Register(ctx, "index@test.invalid", "Pass1234", nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("register: %v", err)
	}
	login, err := svc.Login(ctx, "index@test.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	loginFingerprint := repository.FingerprintRawToken(login.RefreshToken)
	var persistedFingerprint []byte
	if err := pool.QueryRow(ctx, `SELECT token_fingerprint FROM identity.refresh_tokens WHERE token_fingerprint=$1`, loginFingerprint).Scan(&persistedFingerprint); err != nil {
		t.Fatalf("login refresh fingerprint was not persisted: %v", err)
	}
	if !bytes.Equal(persistedFingerprint, loginFingerprint) {
		t.Fatal("login refresh fingerprint does not match the raw token")
	}
	_, legacyHash, err := repository.GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO identity.refresh_tokens (user_id, token_hash, token_fingerprint, expires_at, revoked)
		SELECT (SELECT id FROM identity.users WHERE email=$1), $2, NULL, now() + interval '30 days', false
		FROM generate_series(1, 3000)
	`, "index@test.invalid", legacyHash); err != nil {
		t.Fatalf("insert synthetic legacy rows: %v", err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE identity.refresh_tokens`); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `EXPLAIN SELECT id, user_id, token_hash, token_fingerprint, expires_at, revoked, created_at FROM identity.refresh_tokens WHERE token_fingerprint=$1 LIMIT 1`, loginFingerprint)
	if err != nil {
		t.Fatalf("explain fingerprint lookup: %v", err)
	}
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		plan.WriteString(line)
		plan.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if !strings.Contains(plan.String(), "idx_refresh_tokens_fingerprint") || strings.Contains(plan.String(), "Seq Scan") {
		t.Fatalf("fingerprint lookup did not use its index with 3000 legacy rows; plan:\n%s", plan.String())
	}

	if _, err := svc.Refresh(ctx, login.RefreshToken); err != nil {
		t.Fatalf("refresh with 3000 legacy rows: %v", err)
	}
}
