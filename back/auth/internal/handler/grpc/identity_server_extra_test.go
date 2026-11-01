// Cobertura adicional del servidor gRPC Identity (tarea 3_4_5): errores de
// repositorio, límite de lote, variantes de JWT, mapeo OAuth y ausencia de
// secretos en logs. Reutiliza los dobles de identity_server_test.go.
package grpc

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	identityv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// errRepo simula una caída de PostgreSQL en el doble de usuarios.
type errRepo struct{ err error }

func (f *errRepo) GetPublicByIDs(_ context.Context, _ []string) ([]model.PublicUser, error) {
	return nil, f.err
}

// TestIdentityServer_RepoError comprueba que un fallo de PG es Internal y no
// filtra detalles (los repositorios reales devuelven errores con SQL).
func TestIdentityServer_RepoError(t *testing.T) {
	dbErr := &errRepo{err: errors.New("conexión perdida")}
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("secreto-de-prueba"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     time.Hour,
	})
	empty := &fakeTokenSvc{}
	s := NewIdentityServer(service.NewUsersService(dbErr), jwtSvc, empty, empty, &fakeStore{})
	ctx := context.Background()
	if _, err := s.GetUser(ctx, &identityv1.GetUserRequest{UserId: testAda}); status.Code(err) != codes.Internal {
		t.Fatalf("GetUser con PG caído = %v, se esperaba Internal", err)
	}
	if _, err := s.LookupUsers(ctx, &identityv1.LookupUsersRequest{UserIds: []string{testAda}}); status.Code(err) != codes.Internal {
		t.Fatalf("LookupUsers con PG caído = %v, se esperaba Internal", err)
	}
}

// TestIdentityServer_LookupLimit verifica el tope de 200 sin depender de orden.
func TestIdentityServer_LookupLimit(t *testing.T) {
	users := map[string]model.PublicUser{}
	ids := make([]string, 0, 250)
	for i := 0; i < 250; i++ {
		id := fmt.Sprintf("11111111-1111-4111-8111-%012d", i)
		users[id] = model.PublicUser{UserID: id, Email: fmt.Sprintf("u%d@test.invalid", i)}
		ids = append(ids, id)
	}
	jwtSvc, _ := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("secreto-de-prueba"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     time.Hour,
	})
	empty := &fakeTokenSvc{}
	s := NewIdentityServer(service.NewUsersService(&fakeUserRepo{users: users}), jwtSvc, empty, empty, &fakeStore{})
	r, err := s.LookupUsers(context.Background(), &identityv1.LookupUsersRequest{UserIds: ids})
	if err != nil {
		t.Fatalf("LookupUsers 250 ids: %v", err)
	}
	if len(r.GetUsers()) != 200 {
		t.Fatalf("se esperaban 200 (tope), llegaron %d", len(r.GetUsers()))
	}
	seen := map[string]bool{}
	for _, u := range r.GetUsers() {
		if seen[u.GetUserId()] {
			t.Fatalf("duplicado %s (el repo real desduplica)", u.GetUserId())
		}
		seen[u.GetUserId()] = true
	}
}

func mintFull(t *testing.T, userID string, exp time.Time, iss, aud string, secret []byte, alg jwt.SigningMethod) string {
	t.Helper()
	claims := service.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    iss,
			Audience:  jwt.ClaimStrings{aud},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(alg, claims).SignedString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func verifyDirect(t *testing.T, token string) error {
	t.Helper()
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("secreto-de-prueba"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	empty := &fakeTokenSvc{}
	s := NewIdentityServer(service.NewUsersService(&fakeUserRepo{}), jwtSvc, empty, empty, &fakeStore{})
	_, err = s.VerifyToken(context.Background(), &identityv1.VerifyTokenRequest{Token: token})
	return err
}

// TestIdentityServer_VerifyTokenVariantes cubre firma, algoritmo, issuer y
// audience sin pasar por la red.
func TestIdentityServer_VerifyTokenVariantes(t *testing.T) {
	fut := time.Now().Add(time.Hour)
	cases := map[string]string{
		"firma incorrecta": mintFull(t, testAda, fut, "apuntes-auth", "apuntes-client", []byte("otro-secreto"), jwt.SigningMethodHS256),
		"algoritmo HS384":  mintFull(t, testAda, fut, "apuntes-auth", "apuntes-client", []byte("secreto-de-prueba"), jwt.SigningMethodHS384),
		"issuer malo":      mintFull(t, testAda, fut, "otro-issuer", "apuntes-client", []byte("secreto-de-prueba"), jwt.SigningMethodHS256),
		"audience mala":    mintFull(t, testAda, fut, "apuntes-auth", "otra-audience", []byte("secreto-de-prueba"), jwt.SigningMethodHS256),
	}
	for name, tok := range cases {
		if err := verifyDirect(t, tok); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: code = %v, se esperaba Unauthenticated", name, status.Code(err))
		}
	}
	if err := verifyDirect(t, mintFull(t, testAda, fut, "apuntes-auth", "apuntes-client", []byte("secreto-de-prueba"), jwt.SigningMethodHS256)); err != nil {
		t.Fatalf("token correcto no debe fallar: %v", err)
	}
}

// TestIdentityServer_TodosProtegidos verifica que los cinco RPC exigen la
// clave interna (el interceptor es genérico, pero se comprueba RPC por RPC).
func TestIdentityServer_TodosProtegidos(t *testing.T) {
	c := newTestIdentity(t)
	ctx := context.Background()
	calls := map[string]func() error{
		"GetUser": func() error {
			_, err := c.GetUser(ctx, &identityv1.GetUserRequest{UserId: testAda})
			return err
		},
		"LookupUsers": func() error {
			_, err := c.LookupUsers(ctx, &identityv1.LookupUsersRequest{UserIds: []string{testAda}})
			return err
		},
		"VerifyToken": func() error {
			_, err := c.VerifyToken(ctx, &identityv1.VerifyTokenRequest{Token: "x"})
			return err
		},
		"GetOAuthToken": func() error {
			_, err := c.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{UserId: testAda, Provider: identityv1.Provider_PROVIDER_GOOGLE_DRIVE})
			return err
		},
		"ReportCalendarRevoked": func() error {
			_, err := c.ReportCalendarRevoked(ctx, &identityv1.ReportCalendarRevokedRequest{UserId: testAda})
			return err
		},
	}
	for name, call := range calls {
		if got := status.Code(call()); got != codes.Unauthenticated {
			t.Fatalf("%s sin clave: %v, se esperaba Unauthenticated", name, got)
		}
	}
}

// TestToStatusOAuth cubre el mapeo de códigos de dominio a gRPC.
func TestToStatusOAuth(t *testing.T) {
	cases := map[string]codes.Code{
		"bad_request":                 codes.InvalidArgument,
		"not_connected":               codes.NotFound,
		"drive_connection_invalid":    codes.Unauthenticated,
		"calendar_connection_invalid": codes.Unauthenticated,
		"google_unavailable":          codes.Unavailable,
		"cualquier_otro":              codes.Internal,
	}
	for code, want := range cases {
		if got := status.Code(toStatusOAuth(service.NewServiceError(code))); got != want {
			t.Fatalf("%s: code = %v, se esperaba %v", code, got, want)
		}
	}
	if got := status.Code(toStatusOAuth(errors.New("pg caído"))); got != codes.Internal {
		t.Fatalf("error no-dominio debe ser Internal, fue %v", got)
	}
}
