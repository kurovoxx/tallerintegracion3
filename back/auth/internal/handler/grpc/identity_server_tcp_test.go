// Test TCP aislado del servidor gRPC Identity (cierre 3_4_2): el mismo
// servidor y la misma cadena de interceptores, pero por TCP real en
// 127.0.0.1:0 (puerto libre) en vez de bufconn. Sin PostgreSQL ni Google.
package grpc

import (
	"context"
	"net"
	"testing"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"time"
)

// TestIdentityServer_TCPReal comprueba transporte TCP real: un RPC funciona
// con clave, falla sin ella, y tras el cierre el puerto queda liberado y las
// llamadas fallan (no cuelgan ni responden en falso).
func TestIdentityServer_TCPReal(t *testing.T) {
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("secreto-de-prueba"),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	repo := &fakeUserRepo{users: map[string]model.PublicUser{
		testAda: {UserID: testAda, Email: "ada@test.invalid"},
	}}
	empty := &fakeTokenSvc{}
	srv := NewIdentityServer(service.NewUsersService(repo), jwtSvc, empty, empty, &fakeStore{})

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen TCP: %v", err)
	}
	addr := lis.Addr().String()
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(nil),
		interceptor.NewInternalAuth(testKey),
	))
	identityv1.RegisterIdentityServiceServer(s, srv)
	serveErr := make(chan error, 1)
	go func() { serveErr <- s.Serve(lis) }()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial TCP: %v", err)
	}
	defer conn.Close()
	c := identityv1.NewIdentityServiceClient(conn)
	ctx := metadata.AppendToOutgoingContext(context.Background(), interceptor.MetadataInternalKey, testKey)

	u, err := c.GetUser(ctx, &identityv1.GetUserRequest{UserId: testAda})
	if err != nil || u.GetUserId() != testAda {
		t.Fatalf("GetUser por TCP = %v, %v", u, err)
	}
	if _, err := c.GetUser(context.Background(), &identityv1.GetUserRequest{UserId: testAda}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("sin clave debió ser Unauthenticated: %v", err)
	}

	// Cierre: el servidor se detiene, el puerto se libera y las llamadas
	// posteriores fallan en vez de colgar.
	s.Stop()
	// Serve termina (nil si el Stop ganó la carrera al arranque, o
	// ErrServerStopped); lo importante es que termina y libera el puerto.
	<-serveErr
	if _, err := c.GetUser(ctx, &identityv1.GetUserRequest{UserId: testAda}); err == nil {
		t.Fatal("tras Stop la llamada debió fallar")
	}
	probe, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("el puerto %s no quedó liberado: %v", addr, err)
	}
	probe.Close()
}
