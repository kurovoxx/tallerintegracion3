// Tests mínimos del servidor gRPC Identity (tarea 3_4_2): los 5 RPC por
// bufconn con la cadena de interceptores de main, usando UsersService y
// JWTService reales más dobles de OAuth/store. Sin Google ni PostgreSQL.
package grpc

import (
	"context"
	"github.com/golang-jwt/jwt/v5"
	"net"
	"testing"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testKey = "test-internal-key"

// fakeUserRepo responde GetPublicByIDs desde un mapa (ausentes se omiten,
// como el repositorio real).
type fakeUserRepo struct {
	users map[string]model.PublicUser
}

func (f *fakeUserRepo) GetPublicByIDs(_ context.Context, ids []string) ([]model.PublicUser, error) {
	var out []model.PublicUser
	for _, id := range ids {
		if u, ok := f.users[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

// fakeTokenSvc programa tokens o errores de dominio por usuario.
type fakeTokenSvc struct {
	tokens map[string]string
	errs   map[string]string // user_id -> código ServiceError
}

func (f *fakeTokenSvc) GetValidAccessToken(_ context.Context, userID string) (string, error) {
	if code, ok := f.errs[userID]; ok {
		return "", service.NewServiceError(code)
	}
	return f.tokens[userID], nil
}

func (f *fakeTokenSvc) ReportCalendarPermissionDenied(_ context.Context, userID string) error {
	f.tokens["revoked:"+userID] = "1"
	return nil
}

// fakeStore devuelve la fila OAuth para metadatos (o nil como sin fila).
type fakeStore struct {
	conns map[string]*model.OAuthConnection
}

func (f *fakeStore) GetByUserIDAndProvider(_ context.Context, userID, _ string) (*model.OAuthConnection, error) {
	return f.conns[userID], nil
}

func newTestIdentity(t *testing.T) identityv1.IdentityServiceClient {
	t.Helper()
	name := "Ada"
	email := "ada@test.invalid"
	repo := &fakeUserRepo{users: map[string]model.PublicUser{
		"u-ada":  {UserID: "u-ada", Email: email, DisplayName: &name},
		"u-solo": {UserID: "u-solo", Email: "solo@test.invalid"},
	}}
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("secreto-de-prueba"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	store := &fakeStore{conns: map[string]*model.OAuthConnection{
		"u-ada": {UserID: "u-ada", AccessToken: "tok-ada", ExpiresAt: &exp, ExternalAccountEmail: &email},
	}}
	drive := &fakeTokenSvc{tokens: map[string]string{"u-ada": "tok-ada"}}
	calendar := &fakeTokenSvc{
		tokens: map[string]string{},
		errs:   map[string]string{"u-nocon": "not_connected", "u-rev": "calendar_connection_invalid", "u-down": "google_unavailable"},
	}

	srv := NewIdentityServer(service.NewUsersService(repo), jwtSvc, drive, calendar, store)
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(nil),
		interceptor.NewInternalAuth(testKey),
	))
	identityv1.RegisterIdentityServiceServer(s, srv)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return identityv1.NewIdentityServiceClient(conn)
}

func authedCtx() context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), interceptor.MetadataInternalKey, testKey)
}

func wantCode(t *testing.T, err error, code codes.Code) {
	t.Helper()
	if status.Code(err) != code {
		t.Fatalf("code = %v, se esperaba %v (err=%v)", status.Code(err), code, err)
	}
}

// TestIdentityServer_AuthInterna verifica la metadata x-internal-key.
func TestIdentityServer_AuthInterna(t *testing.T) {
	c := newTestIdentity(t)
	_, err := c.GetUser(context.Background(), &identityv1.GetUserRequest{UserId: "u-ada"})
	wantCode(t, err, codes.Unauthenticated)
	bad := metadata.AppendToOutgoingContext(context.Background(), interceptor.MetadataInternalKey, "otra")
	_, err = c.GetUser(bad, &identityv1.GetUserRequest{UserId: "u-ada"})
	wantCode(t, err, codes.Unauthenticated)
}

// TestIdentityServer_GetUser cubre válido, inexistente y vacío.
func TestIdentityServer_GetUser(t *testing.T) {
	c := newTestIdentity(t)
	ctx := authedCtx()
	u, err := c.GetUser(ctx, &identityv1.GetUserRequest{UserId: "u-ada"})
	if err != nil || u.GetUserId() != "u-ada" || u.GetEmail() != "ada@test.invalid" || u.GetDisplayName() != "Ada" {
		t.Fatalf("GetUser = %v, %v", u, err)
	}
	s, err := c.GetUser(ctx, &identityv1.GetUserRequest{UserId: "u-solo"})
	if err != nil || s.GetDisplayName() != "" {
		t.Fatalf("sin display_name debe venir ausente: %v, %v", s, err)
	}
	_, err = c.GetUser(ctx, &identityv1.GetUserRequest{UserId: "nadie"})
	wantCode(t, err, codes.NotFound)
	_, err = c.GetUser(ctx, &identityv1.GetUserRequest{})
	wantCode(t, err, codes.InvalidArgument)
}

// TestIdentityServer_LookupUsers cubre lote, vacíos e inexistentes.
func TestIdentityServer_LookupUsers(t *testing.T) {
	c := newTestIdentity(t)
	ctx := authedCtx()
	r, err := c.LookupUsers(ctx, &identityv1.LookupUsersRequest{UserIds: []string{"u-ada", "nadie", "u-solo"}})
	if err != nil || len(r.GetUsers()) != 2 {
		t.Fatalf("LookupUsers = %v, %v", r, err)
	}
	empty, err := c.LookupUsers(ctx, &identityv1.LookupUsersRequest{})
	if err != nil || len(empty.GetUsers()) != 0 {
		t.Fatalf("sin ids debe ser lista vacía: %v, %v", empty, err)
	}
}

// TestIdentityServer_VerifyToken cubre válido, inválido, expirado y vacío.
func TestIdentityServer_VerifyToken(t *testing.T) {
	c := newTestIdentity(t)
	ctx := authedCtx()
	ok, err := c.VerifyToken(ctx, &identityv1.VerifyTokenRequest{Token: mintTestToken(t, "u-ada", time.Now().Add(time.Hour))})
	if err != nil || !ok.GetValid() || ok.GetUserId() != "u-ada" {
		t.Fatalf("VerifyToken válido = %v, %v", ok, err)
	}
	_, err = c.VerifyToken(ctx, &identityv1.VerifyTokenRequest{Token: "no-es-un-jwt"})
	wantCode(t, err, codes.Unauthenticated)
	_, err = c.VerifyToken(ctx, &identityv1.VerifyTokenRequest{Token: mintTestToken(t, "u-ada", time.Now().Add(-time.Hour))})
	wantCode(t, err, codes.Unauthenticated)
	_, err = c.VerifyToken(ctx, &identityv1.VerifyTokenRequest{})
	wantCode(t, err, codes.InvalidArgument)
}

func mintTestToken(t *testing.T, userID string, expiraEn time.Time) string {
	t.Helper()
	claims := service.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "apuntes-auth",
			Audience:  jwt.ClaimStrings{"apuntes-client"},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiraEn),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secreto-de-prueba"))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

// TestIdentityServer_GetOAuthToken cubre drive/calendar, errores de dominio,
// provider inválido y ausencia de refresh tokens en la respuesta.
func TestIdentityServer_GetOAuthToken(t *testing.T) {
	c := newTestIdentity(t)
	ctx := authedCtx()
	d, err := c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{UserId: "u-ada", Provider: identityv1.Provider_PROVIDER_GOOGLE_DRIVE})
	if err != nil || d.GetAccessToken() != "tok-ada" {
		t.Fatalf("drive = %v, %v", d, err)
	}
	if d.GetExpiresAt() == nil || d.GetExternalEmail() != "ada@test.invalid" {
		t.Fatalf("faltan metadatos esperados: %v", d)
	}
	_, err = c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{UserId: "u-ada", Provider: identityv1.Provider_PROVIDER_UNSPECIFIED})
	wantCode(t, err, codes.InvalidArgument)
	_, err = c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{UserId: "u-nocon", Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	wantCode(t, err, codes.NotFound)
	_, err = c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{UserId: "u-rev", Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	wantCode(t, err, codes.Unauthenticated)
	_, err = c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{UserId: "u-down", Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	wantCode(t, err, codes.Unavailable)
	_, err = c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{Provider: identityv1.Provider_PROVIDER_GOOGLE_DRIVE})
	wantCode(t, err, codes.InvalidArgument)
}

// TestIdentityServer_ReportCalendarRevoked cubre idempotencia y validación.
func TestIdentityServer_ReportCalendarRevoked(t *testing.T) {
	c := newTestIdentity(t)
	ctx := authedCtx()
	if _, err := c.ReportCalendarRevoked(ctx, &identityv1.ReportCalendarRevokedRequest{UserId: "u-ada"}); err != nil {
		t.Fatalf("primera llamada: %v", err)
	}
	if _, err := c.ReportCalendarRevoked(ctx, &identityv1.ReportCalendarRevokedRequest{UserId: "u-ada"}); err != nil {
		t.Fatalf("segunda llamada (idempotente): %v", err)
	}
	if _, err := c.ReportCalendarRevoked(ctx, &identityv1.ReportCalendarRevokedRequest{UserId: "nadie"}); err != nil {
		t.Fatalf("sin fila también es OK: %v", err)
	}
	_, err := c.ReportCalendarRevoked(ctx, &identityv1.ReportCalendarRevokedRequest{})
	wantCode(t, err, codes.InvalidArgument)
}
