package drive

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/identityv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeIdentity respeta el contrato real: exige clave, solo sirve Drive y
// devuelve códigos de dominio (NotFound/Unauthenticated/Unavailable).
type fakeIdentity struct {
	identityv1.UnimplementedIdentityServiceServer
	key     string
	tokens  map[string]string // user_id -> access token
	revoked map[string]bool   // user_id con conexión inválida
	down    bool
	delay   time.Duration
}

func (f *fakeIdentity) checkKey(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md.Get("x-internal-key")) == 0 || md.Get("x-internal-key")[0] != f.key {
		return status.Error(codes.Unauthenticated, "invalid internal key")
	}
	return nil
}

func (f *fakeIdentity) GetOAuthToken(ctx context.Context, req *identityv1.GetOAuthTokenRequest) (*identityv1.GetOAuthTokenResponse, error) {
	if err := f.checkKey(ctx); err != nil {
		return nil, err
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, status.Error(codes.DeadlineExceeded, ctx.Err().Error())
		}
	}
	if f.down {
		return nil, status.Error(codes.Unavailable, "caído")
	}
	if req.GetProvider() != identityv1.Provider_PROVIDER_GOOGLE_DRIVE {
		return nil, status.Error(codes.InvalidArgument, "provider inválido")
	}
	if strings.TrimSpace(req.GetUserId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id requerido")
	}
	if f.revoked[req.GetUserId()] {
		return nil, status.Error(codes.Unauthenticated, "drive_connection_invalid")
	}
	tok, ok := f.tokens[req.GetUserId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_connected")
	}
	return &identityv1.GetOAuthTokenResponse{AccessToken: tok}, nil
}

func serveFakeIdentity(t *testing.T, fake *fakeIdentity) *GRPCOAuthTokenProvider {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(s, fake)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &GRPCOAuthTokenProvider{Client: identityv1.NewIdentityServiceClient(conn), Key: "k", Timeout: 2 * time.Second}
}

// TestGRPCOAuthTokenProvider_TokenValido cubre el camino feliz con Drive.
func TestGRPCOAuthTokenProvider_TokenValido(t *testing.T) {
	p := serveFakeIdentity(t, &fakeIdentity{key: "k", tokens: map[string]string{"u1": "tok-1"}})
	tok, err := p.GetValidAccessToken(context.Background(), "u1")
	if err != nil || tok != "tok-1" {
		t.Fatalf("token = %q, %v", tok, err)
	}
	if _, err := p.GetValidAccessToken(context.Background(), ""); !IsOAuthError(err) {
		t.Fatalf("vacío debe ser OAuthError, fue %v", err)
	}
}

// TestGRPCOAuthTokenProvider_Errores cubre inexistente, revocado, clave mala,
// caída, timeout y respuesta vacía, sin confundir infraestructura con revocación.
func TestGRPCOAuthTokenProvider_Errores(t *testing.T) {
	p := serveFakeIdentity(t, &fakeIdentity{key: "k", revoked: map[string]bool{"u-rev": true}})
	ctx := context.Background()

	if _, err := p.GetValidAccessToken(ctx, "nadie"); !IsOAuthError(err) {
		t.Fatalf("sin conexión debe ser OAuthError, fue %v", err)
	}
	if _, err := p.GetValidAccessToken(ctx, "u-rev"); !IsOAuthError(err) {
		t.Fatalf("revocado debe ser OAuthError, fue %v", err)
	}

	mala := *p
	mala.Key = "otra"
	if _, err := mala.GetValidAccessToken(ctx, "u1"); !isDrive500(err) {
		t.Fatalf("clave mala debe ser DriveError 500, fue %v", err)
	}
	if _, err := mala.GetValidAccessToken(ctx, "u1"); IsOAuthError(err) {
		t.Fatalf("clave mala jamás debe verse como OAuth del usuario: %v", err)
	}

	lento := serveFakeIdentity(t, &fakeIdentity{key: "k", tokens: map[string]string{"u1": "t"}, delay: 5 * time.Second})
	lento.Timeout = 50 * time.Millisecond
	if _, err := lento.GetValidAccessToken(ctx, "u1"); !isDrive500(err) {
		t.Fatalf("timeout debe ser DriveError 500, fue %v", err)
	}

	caido := serveFakeIdentity(t, &fakeIdentity{key: "k", down: true})
	if _, err := caido.GetValidAccessToken(ctx, "u1"); !isDrive500(err) {
		t.Fatalf("caída debe ser DriveError 500, fue %v", err)
	}

	vacio := serveFakeIdentity(t, &fakeIdentity{key: "k", tokens: map[string]string{"u1": ""}})
	if _, err := vacio.GetValidAccessToken(ctx, "u1"); !IsForbidden(err) {
		t.Fatalf("respuesta vacía debe ser 403, fue %v", err)
	}
}

func isDrive500(err error) bool {
	var de *DriveError
	return errors.As(err, &de) && de.Code == 500
}

// TestGRPCOAuthTokenProvider_Recuperacion cubre Auth caído y vuelto por TCP
// real, sin reiniciar nada del lado Notes.
func TestGRPCOAuthTokenProvider_Recuperacion(t *testing.T) {
	fake := &fakeIdentity{key: "k", tokens: map[string]string{"u1": "tok-1"}}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	s := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(s, fake)
	go func() { _ = s.Serve(lis) }()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	p := &GRPCOAuthTokenProvider{Client: identityv1.NewIdentityServiceClient(conn), Key: "k", Timeout: 2 * time.Second}
	ctx := context.Background()
	if _, err := p.GetValidAccessToken(ctx, "u1"); err != nil {
		t.Fatalf("arriba: %v", err)
	}

	s.Stop() // Identity cae
	if _, err := p.GetValidAccessToken(ctx, "u1"); !isDrive500(err) {
		t.Fatalf("caído debe ser DriveError 500, fue %v", err)
	}

	lis2, err := net.Listen("tcp", addr) // Identity vuelve
	if err != nil {
		t.Fatalf("re-listen: %v", err)
	}
	s2 := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(s2, fake)
	go func() { _ = s2.Serve(lis2) }()
	t.Cleanup(s2.Stop)
	deadline := time.Now().Add(8 * time.Second)
	for {
		tok, err := p.GetValidAccessToken(ctx, "u1")
		if err == nil {
			if tok != "tok-1" {
				t.Fatalf("token inesperado: %q", tok)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("recuperado debe funcionar sin reiniciar: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestGRPCOAuthTokenProvider_MockSinIdentity documenta que el modo mock no
// necesita Auth: el MockClient funciona standalone.
func TestGRPCOAuthTokenProvider_MockSinIdentity(t *testing.T) {
	m := NewMockClient()
	if m == nil {
		t.Fatal("mock debe construirse sin Identity")
	}
}
