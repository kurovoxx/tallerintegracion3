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

import (
	"context"
	"os"
	"path/filepath"
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
	t.Helper()
	url := os.Getenv("AUTH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AUTH_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
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
		repository.NewRefreshTokenRepository(pool),
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
	var exists bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='identity' AND table_name='password_resets')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("identity.password_resets debe existir tras aplicar db/migrations/*.sql")
	}
}
