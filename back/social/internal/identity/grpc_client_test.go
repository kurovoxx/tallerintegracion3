package identity

import (
	"context"
	"net"
	"testing"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeIdentityGRPC implementa IdentityServiceServer respetando el contrato real:
// exige x-internal-key, valida entradas y devuelve códigos gRPC de dominio.
type fakeIdentityGRPC struct {
	identityv1.UnimplementedIdentityServiceServer
	key     string
	users   map[string]*identityv1.User
	tokens  map[string]string // jwt -> user_id
	delay   time.Duration
	failAll codes.Code
}

func (f *fakeIdentityGRPC) checkKey(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md.Get(MetadataInternalKey)) == 0 || md.Get(MetadataInternalKey)[0] != f.key {
		return status.Error(codes.Unauthenticated, "invalid internal key")
	}
	return nil
}

func (f *fakeIdentityGRPC) maybeDelay(ctx context.Context) error {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return status.Error(codes.DeadlineExceeded, ctx.Err().Error())
		}
	}
	return nil
}

func (f *fakeIdentityGRPC) fail() error {
	if f.failAll != codes.OK {
		return status.Error(f.failAll, "inyectado")
	}
	return nil
}

func (f *fakeIdentityGRPC) GetUser(ctx context.Context, req *identityv1.GetUserRequest) (*identityv1.GetUserResponse, error) {
	if err := f.checkKey(ctx); err != nil {
		return nil, err
	}
	if err := f.maybeDelay(ctx); err != nil {
		return nil, err
	}
	if err := f.fail(); err != nil {
		return nil, err
	}
	u, ok := f.users[req.GetUserId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "user_not_found")
	}
	return &identityv1.GetUserResponse{UserId: u.GetUserId(), Email: u.GetEmail(), DisplayName: u.DisplayName}, nil
}

func (f *fakeIdentityGRPC) LookupUsers(ctx context.Context, req *identityv1.LookupUsersRequest) (*identityv1.LookupUsersResponse, error) {
	if err := f.checkKey(ctx); err != nil {
		return nil, err
	}
	if err := f.maybeDelay(ctx); err != nil {
		return nil, err
	}
	if err := f.fail(); err != nil {
		return nil, err
	}
	out := []*identityv1.User{}
	for _, id := range req.GetUserIds() {
		if u, ok := f.users[id]; ok {
			out = append(out, u)
		}
	}
	return &identityv1.LookupUsersResponse{Users: out}, nil
}

func (f *fakeIdentityGRPC) VerifyToken(ctx context.Context, req *identityv1.VerifyTokenRequest) (*identityv1.VerifyTokenResponse, error) {
	if err := f.checkKey(ctx); err != nil {
		return nil, err
	}
	if err := f.maybeDelay(ctx); err != nil {
		return nil, err
	}
	if req.GetToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "token requerido")
	}
	uid, ok := f.tokens[req.GetToken()]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "token inválido")
	}
	return &identityv1.VerifyTokenResponse{Valid: true, UserId: uid}, nil
}

func serveFakeIdentityGRPC(t *testing.T, fake *fakeIdentityGRPC) *GRPCClient {
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
	return &GRPCClient{client: identityv1.NewIdentityServiceClient(conn), conn: conn, key: "k", timeout: 2 * time.Second}
}

func testUsers() map[string]*identityv1.User {
	ada := "Ada"
	return map[string]*identityv1.User{
		"u-ada":  {UserId: "u-ada", Email: "ada@test.invalid", DisplayName: &ada},
		"u-solo": {UserId: "u-solo", Email: "solo@test.invalid"},
	}
}

// TestGRPCClient_VerifyToken cubre válido, inválido, vacío, clave mala,
// caída y timeout, sin confundir infraestructura con token inválido.
func TestGRPCClient_VerifyToken(t *testing.T) {
	fake := &fakeIdentityGRPC{key: "k", users: testUsers(), tokens: map[string]string{"bueno": "u-ada"}}
	c := serveFakeIdentityGRPC(t, fake)
	ctx := context.Background()

	uid, err := c.VerifyToken(ctx, "bueno")
	if err != nil || uid != "u-ada" {
		t.Fatalf("VerifyToken = %q, %v", uid, err)
	}
	if _, err := c.VerifyToken(ctx, "malo"); err != ErrInvalidToken {
		t.Fatalf("token malo debe ser ErrInvalidToken, fue %v", err)
	}
	if _, err := c.VerifyToken(ctx, ""); err != ErrInvalidToken {
		t.Fatalf("vacío debe ser ErrInvalidToken, fue %v", err)
	}

	// Clave mala: el fake rechaza con "invalid internal key" antes del
	// servicio, así que NO debe verse como token inválido aunque el JWT
	// sea bueno (distingue fallo de infraestructura de JWT rechazado).
	noKey := &GRPCClient{client: c.client, key: "", timeout: 2 * time.Second}
	if _, err := noKey.VerifyToken(ctx, "bueno"); err == nil || err == ErrInvalidToken {
		t.Fatalf("sin clave debe ser error de infraestructura, no éxito ni ErrInvalidToken: %v", err)
	}
	// Servidor caído: error de infraestructura, nunca éxito ni token inválido.
	closedLis := bufconn.Listen(1 << 20)
	closedLis.Close()
	closedConn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return closedLis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer closedConn.Close()
	caido := &GRPCClient{client: identityv1.NewIdentityServiceClient(closedConn), key: "k", timeout: 2 * time.Second}
	if _, err := caido.VerifyToken(ctx, "bueno"); err == nil || err == ErrInvalidToken {
		t.Fatalf("caída debe ser error de infraestructura, no éxito ni ErrInvalidToken: %v", err)
	}

	lento := serveFakeIdentityGRPC(t, &fakeIdentityGRPC{key: "k", delay: 5 * time.Second})
	lento.timeout = 50 * time.Millisecond
	if _, err := lento.VerifyToken(ctx, "bueno"); err == nil {
		t.Fatal("timeout debe fallar")
	} else if err == ErrInvalidToken {
		t.Fatalf("timeout no debe confundirse con token inválido: %v", err)
	}
}

// TestGRPCClient_GetUser cubre válido, inexistente y vacío.
func TestGRPCClient_GetUser(t *testing.T) {
	c := serveFakeIdentityGRPC(t, &fakeIdentityGRPC{key: "k", users: testUsers()})
	ctx := context.Background()
	u, err := c.GetUser(ctx, "u-ada")
	if err != nil || u.UserID != "u-ada" || u.Email != "ada@test.invalid" || u.DisplayName == nil || *u.DisplayName != "Ada" {
		t.Fatalf("GetUser = %+v, %v", u, err)
	}
	if _, err := c.GetUser(ctx, "nadie"); err != ErrUserNotFound {
		t.Fatalf("inexistente debe ser ErrUserNotFound, fue %v", err)
	}
	if _, err := c.GetUser(ctx, ""); err != ErrUserNotFound {
		t.Fatalf("vacío debe ser ErrUserNotFound, fue %v", err)
	}
}
