// Integración gRPC con PostgreSQL real (tarea 3_4_5): cliente →
// interceptores → IdentityServer → servicios → repositorios → PG.
//
// BD desechable propia (AUTH_GRPC_TEST_DATABASE_URL), nunca la de Miguel
// (AUTH_TEST_DATABASE_URL) ni desarrollo: crea su SCHEMA identity y lo borra
// al terminar. Sin AUTH_GRPC_TEST_DATABASE_URL se omite. Sin Google real:
// los providers OAuth son dobles locales.
package grpc

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	identityv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const grpcIntegrationSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE identity.users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email varchar(255) NOT NULL UNIQUE,
    password_hash varchar(255) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
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

// stubDrive simula Google solo para Refresh (Exchange no se usa en lectura).
type stubDrive struct {
	token string
	exp   time.Time
	err   error
	calls *int
}

func (s *stubDrive) Exchange(_ context.Context, _ string) (*service.DriveOAuthResult, error) {
	return nil, service.ErrGoogleUnavailable
}

func (s *stubDrive) Refresh(_ context.Context, _ string) (*service.DriveOAuthResult, error) {
	*s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &service.DriveOAuthResult{AccessToken: s.token, ExpiresAt: &s.exp}, nil
}

// stubCalendar igual para Calendar.
type stubCalendar struct {
	token string
	exp   time.Time
	err   error
	calls *int
}

func (s *stubCalendar) Exchange(_ context.Context, _ string) (*service.CalendarOAuthResult, error) {
	return nil, service.ErrGoogleUnavailable
}

func (s *stubCalendar) Refresh(_ context.Context, _ string) (*service.CalendarOAuthResult, error) {
	*s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return &service.CalendarOAuthResult{AccessToken: s.token, ExpiresAt: &s.exp}, nil
}

func strp(s string) *string { return &s }

// TestIdentityServerPG recorre los 5 RPC contra PostgreSQL real.
func TestIdentityServerPG(t *testing.T) {
	url := os.Getenv("AUTH_GRPC_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("AUTH_GRPC_TEST_DATABASE_URL not configured")
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
	if _, err := pool.Exec(ctx, grpcIntegrationSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	ada, solo := uuid.NewString(), uuid.NewString()
	ghost := uuid.NewString()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	exec(`INSERT INTO identity.users (id, email, password_hash) VALUES ($1, 'ada@test.invalid', 'x')`, ada)
	exec(`INSERT INTO identity.users (id, email, password_hash) VALUES ($1, 'solo@test.invalid', 'x')`, solo)
	exec(`INSERT INTO identity.profiles (user_id, display_name) VALUES ($1, 'Ada')`, ada)
	fut := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	past := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	exec(`INSERT INTO identity.oauth_connections (user_id, provider, access_token, refresh_token, expires_at, external_account_email)
	      VALUES ($1, 'google_drive', 'tok-drive-ok', 'r-ok', $2, 'ada@test.invalid')`, ada, fut)
	exec(`INSERT INTO identity.oauth_connections (user_id, provider, access_token, refresh_token, expires_at)
	      VALUES ($1, 'google_drive', 'tok-viejo', 'r-viejo', $2)`, solo, past)
	exec(`INSERT INTO identity.oauth_connections (user_id, provider, access_token, revoked_at)
	      VALUES ($1, 'google_calendar', 'tok-rev', now())`, solo)
	exec(`INSERT INTO identity.oauth_connections (user_id, provider, access_token, refresh_token, expires_at)
	      VALUES ($1, 'google_calendar', 'tok-cal-viejo', 'r-cal', $2)`, ada, past)

	var driveCalls, calCalls int
	newExp := time.Now().Add(time.Hour).Truncate(time.Second)
	driveSvc := service.NewDriveOAuthService(repository.NewOAuthRepository(pool),
		&stubDrive{token: "tok-drive-nuevo", exp: newExp, calls: &driveCalls})
	calSvc := service.NewCalendarOAuthService(repository.NewOAuthRepository(pool),
		&stubCalendar{token: "tok-cal-nuevo", exp: newExp, calls: &calCalls})
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("grpc-pg-test-secret-32-chars-long!!"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := NewIdentityServer(
		service.NewUsersService(repository.NewUserRepository(pool)),
		jwtSvc, driveSvc, calSvc, repository.NewOAuthRepository(pool))

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(nil),
		interceptor.NewInternalAuth(testKey),
	))
	identityv1.RegisterIdentityServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) { return lis.DialContext(dialCtx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := identityv1.NewIdentityServiceClient(conn)
	gctx := metadata.AppendToOutgoingContext(ctx, interceptor.MetadataInternalKey, testKey)
	wantCode := func(err error, code codes.Code) {
		t.Helper()
		if status.Code(err) != code {
			t.Fatalf("code = %v, se esperaba %v (err=%v)", status.Code(err), code, err)
		}
	}

	// Sin clave también falla contra PG (el interceptor va primero).
	_, err = c.GetUser(ctx, &identityv1.GetUserRequest{UserId: ada})
	wantCode(err, codes.Unauthenticated)

	// GetUser persistido + NotFound + InvalidArgument (malformado, sin PG).
	u, err := c.GetUser(gctx, &identityv1.GetUserRequest{UserId: ada})
	if err != nil || u.GetEmail() != "ada@test.invalid" || u.GetDisplayName() != "Ada" {
		t.Fatalf("GetUser PG = %v, %v", u, err)
	}
	s, err := c.GetUser(gctx, &identityv1.GetUserRequest{UserId: solo})
	if err != nil || s.GetDisplayName() != "" {
		t.Fatalf("sin perfil con nombre debe venir ausente: %v, %v", s, err)
	}
	_, err = c.GetUser(gctx, &identityv1.GetUserRequest{UserId: ghost})
	wantCode(err, codes.NotFound)
	_, err = c.GetUser(gctx, &identityv1.GetUserRequest{UserId: "mal!!"})
	wantCode(err, codes.InvalidArgument)

	// LookupUsers con varios persistidos.
	r, err := c.LookupUsers(gctx, &identityv1.LookupUsersRequest{UserIds: []string{ada, solo, ghost}})
	if err != nil || len(r.GetUsers()) != 2 {
		t.Fatalf("LookupUsers PG = %v, %v", r, err)
	}

	// VerifyToken con JWT real recién emitido.
	login, err := jwtSvc.GenerarAccessToken(service.UsuarioAutenticado{ID: ada})
	if err != nil {
		t.Fatal(err)
	}
	ok, err := c.VerifyToken(gctx, &identityv1.VerifyTokenRequest{Token: login.AccessToken})
	if err != nil || !ok.GetValid() || ok.GetUserId() != ada {
		t.Fatalf("VerifyToken PG = %v, %v", ok, err)
	}

	// Drive vigente: sin refresh, con metadatos.
	d, err := c.GetOAuthToken(gctx, &identityv1.GetOAuthTokenRequest{UserId: ada, Provider: identityv1.Provider_PROVIDER_GOOGLE_DRIVE})
	if err != nil || d.GetAccessToken() != "tok-drive-ok" {
		t.Fatalf("drive vigente = %v, %v", d, err)
	}
	if d.GetExpiresAt() == nil || d.GetExternalEmail() != "ada@test.invalid" {
		t.Fatalf("metadatos drive: %v", d)
	}
	if driveCalls != 0 {
		t.Fatalf("con token vigente no debe refrescar (llamadas=%d)", driveCalls)
	}

	// Drive vencido: refresca con el stub y persiste el nuevo token.
	d2, err := c.GetOAuthToken(gctx, &identityv1.GetOAuthTokenRequest{UserId: solo, Provider: identityv1.Provider_PROVIDER_GOOGLE_DRIVE})
	if err != nil || d2.GetAccessToken() != "tok-drive-nuevo" {
		t.Fatalf("drive refresh = %v, %v", d2, err)
	}
	var persisted string
	if err := pool.QueryRow(ctx, `SELECT access_token FROM identity.oauth_connections WHERE user_id=$1 AND provider='google_drive'`, solo).Scan(&persisted); err != nil || persisted != "tok-drive-nuevo" {
		t.Fatalf("refresh no persistido: %q err=%v", persisted, err)
	}

	// Calendar revocado y sin fila.
	_, err = c.GetOAuthToken(gctx, &identityv1.GetOAuthTokenRequest{UserId: solo, Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	wantCode(err, codes.Unauthenticated)
	_, err = c.GetOAuthToken(gctx, &identityv1.GetOAuthTokenRequest{UserId: ghost, Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	wantCode(err, codes.NotFound)

	// ReportCalendarRevoked marca revoked_at (SQL) y es idempotente.
	if _, err := c.ReportCalendarRevoked(gctx, &identityv1.ReportCalendarRevokedRequest{UserId: ada}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	// revoked_at queda marcado para ada/calendar.
	var hasRevoked bool
	if err := pool.QueryRow(ctx, `SELECT revoked_at IS NOT NULL FROM identity.oauth_connections WHERE user_id=$1 AND provider='google_calendar'`, ada).Scan(&hasRevoked); err != nil || !hasRevoked {
		t.Fatalf("revoked_at no marcado: err=%v", err)
	}
	if _, err := c.ReportCalendarRevoked(gctx, &identityv1.ReportCalendarRevokedRequest{UserId: ada}); err != nil {
		t.Fatalf("revoke idempotente: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1`, ghost).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sin fila no debe crear nada: %d err=%v", n, err)
	}
	if _, err := c.ReportCalendarRevoked(gctx, &identityv1.ReportCalendarRevokedRequest{UserId: ghost}); err != nil {
		t.Fatalf("revoke sin fila: %v", err)
	}
}
