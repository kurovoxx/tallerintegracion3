package service

import (
	"context"
	"errors"
	"net"
	"strings"
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

// fakeIdentityServer respeta el contrato real: exige clave, valida entradas y
// devuelve códigos de dominio (NotFound/Unauthenticated/Unavailable).
type fakeIdentityServer struct {
	identityv1.UnimplementedIdentityServiceServer
	key      string
	tokens   map[string]string // user_id -> access token
	revoked  map[string]bool   // user_id sin conexión válida
	down     bool
	delay    time.Duration
	reported []string
}

func (f *fakeIdentityServer) checkKey(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(md.Get("x-internal-key")) == 0 || md.Get("x-internal-key")[0] != f.key {
		return status.Error(codes.Unauthenticated, "invalid internal key")
	}
	return nil
}

func (f *fakeIdentityServer) GetOAuthToken(ctx context.Context, req *identityv1.GetOAuthTokenRequest) (*identityv1.GetOAuthTokenResponse, error) {
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
	if req.GetProvider() != identityv1.Provider_PROVIDER_GOOGLE_CALENDAR {
		return nil, status.Error(codes.InvalidArgument, "provider inválido")
	}
	if strings.TrimSpace(req.GetUserId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id requerido")
	}
	if f.revoked[req.GetUserId()] {
		return nil, status.Error(codes.Unauthenticated, "calendar_connection_invalid")
	}
	tok, ok := f.tokens[req.GetUserId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_connected")
	}
	return &identityv1.GetOAuthTokenResponse{AccessToken: tok}, nil
}

func (f *fakeIdentityServer) ReportCalendarRevoked(ctx context.Context, req *identityv1.ReportCalendarRevokedRequest) (*identityv1.ReportCalendarRevokedResponse, error) {
	if err := f.checkKey(ctx); err != nil {
		return nil, err
	}
	f.reported = append(f.reported, req.GetUserId())
	return &identityv1.ReportCalendarRevokedResponse{}, nil
}

func (f *fakeIdentityServer) VerifyToken(ctx context.Context, req *identityv1.VerifyTokenRequest) (*identityv1.VerifyTokenResponse, error) {
	if err := f.checkKey(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.GetToken()) == "" {
		return nil, status.Error(codes.InvalidArgument, "token requerido")
	}
	return &identityv1.VerifyTokenResponse{Valid: true, UserId: "u"}, nil
}

func serveFakeCalendar(t *testing.T, fake *fakeIdentityServer) *GRPCalendarGateway {
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
	return &GRPCalendarGateway{Client: identityv1.NewIdentityServiceClient(conn), Key: "k", Timeout: 2 * time.Second}
}

// TestGRPCalendarGateway_GetToken cubre vigente, errores de dominio,
// timeouts, clave mala y caída, sin confundir infraestructura con revocación.
func TestGRPCalendarGateway_GetToken(t *testing.T) {
	fake := &fakeIdentityServer{key: "k", tokens: map[string]string{"u1": "tok-1"}, revoked: map[string]bool{"u-rev": true}}
	gw := serveFakeCalendar(t, fake)
	ctx := context.Background()

	tok, err := gw.GetToken(ctx, "u1")
	if err != nil || tok != "tok-1" {
		t.Fatalf("GetToken = %q, %v", tok, err)
	}
	if _, err := gw.GetToken(ctx, ""); !errors.Is(err, ErrCalendarNotConnected) {
		t.Fatalf("vacío debe ser NotConnected, fue %v", err)
	}
	if _, err := gw.GetToken(ctx, "nadie"); !errors.Is(err, ErrCalendarNotConnected) {
		t.Fatalf("sin conexión debe ser NotConnected, fue %v", err)
	}
	if _, err := gw.GetToken(ctx, "u-rev"); !errors.Is(err, ErrCalendarConnectionInvalid) {
		t.Fatalf("revocado debe ser ConnectionInvalid, fue %v", err)
	}

	mala := *gw
	mala.Key = "otra"
	if _, err := mala.GetToken(ctx, "u1"); !errors.Is(err, ErrCalendarConnectionInvalid) {
		t.Fatalf("clave mala llega como ConnectionInvalid (clave validada al arrancar): %v", err)
	}

	lento := serveFakeCalendar(t, &fakeIdentityServer{key: "k", tokens: map[string]string{"u1": "tok-1"}, delay: 5 * time.Second})
	lento.Timeout = 50 * time.Millisecond
	if _, err := lento.GetToken(ctx, "u1"); !errors.Is(err, ErrCalendarGatewayUnavailable) {
		t.Fatalf("timeout debe ser Unavailable, fue %v", err)
	}

	caido := serveFakeCalendar(t, &fakeIdentityServer{key: "k", down: true})
	if _, err := caido.GetToken(ctx, "u1"); !errors.Is(err, ErrCalendarGatewayUnavailable) {
		t.Fatalf("caída debe ser Unavailable, fue %v", err)
	}
}

// TestGRPCalendarGateway_Validate cubre la validación de arranque.
func TestGRPCalendarGateway_Validate(t *testing.T) {
	gw := serveFakeCalendar(t, &fakeIdentityServer{key: "k"})
	if err := gw.Validate(context.Background()); err != nil {
		t.Fatalf("Validate con clave buena: %v", err)
	}
	mala := *gw
	mala.Key = "otra"
	if err := mala.Validate(context.Background()); err == nil {
		t.Fatal("Validate con clave mala debe fallar")
	}
}

// TestGRPCalendarGateway_ReportRevoked cubre el reporte best-effort.
func TestGRPCalendarGateway_ReportRevoked(t *testing.T) {
	fake := &fakeIdentityServer{key: "k"}
	gw := serveFakeCalendar(t, fake)
	ctx := context.Background()
	if err := gw.ReportRevoked(ctx, "u1"); err != nil {
		t.Fatalf("ReportRevoked: %v", err)
	}
	if len(fake.reported) != 1 || fake.reported[0] != "u1" {
		t.Fatalf("reportado = %v", fake.reported)
	}
	if err := gw.ReportRevoked(ctx, ""); err != nil {
		t.Fatalf("vacío es no-op: %v", err)
	}
	if len(fake.reported) != 1 {
		t.Fatalf("vacío no debe reportar: %v", fake.reported)
	}
}
