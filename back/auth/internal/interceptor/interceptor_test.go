// Tests de los interceptores gRPC de auth (tarea 3_4_5): clave interna,
// fail-closed sin configuración, logging y ausencia de secretos en logs.
// Servicio falso mínimo por bufconn, mismo criterio que social.
package interceptor

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type fakeSvc struct{}

type fakeSvcIface interface{}

func fakePing(fail bool) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	return func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
		in := new(emptypb.Empty)
		if err := dec(in); err != nil {
			return nil, err
		}
		call := func(ctx context.Context, req any) (any, error) {
			if fail {
				return nil, status.Error(codes.NotFound, "no existe")
			}
			return &wrapperspb.StringValue{Value: "pong"}, nil
		}
		if interceptor == nil {
			return call(ctx, in)
		}
		return interceptor(ctx, in, &grpc.UnaryServerInfo{
			Server:     srv,
			FullMethod: "/identity.v1.IdentityService/Ping",
		}, call)
	}
}

func dialAuth(t *testing.T, fail bool, interceptors ...grpc.UnaryServerInterceptor) *grpc.ClientConn {
	t.Helper()
	methods := []grpc.MethodDesc{{MethodName: "Ping", Handler: fakePing(fail)}}
	desc := &grpc.ServiceDesc{ServiceName: "identity.v1.IdentityService", HandlerType: (*fakeSvcIface)(nil), Methods: methods}
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptors...))
	s.RegisterService(desc, &fakeSvc{})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func invokePing(conn *grpc.ClientConn, pairs ...string) error {
	ctx := context.Background()
	if len(pairs) > 0 {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(pairs...))
	}
	out := new(wrapperspb.StringValue)
	return conn.Invoke(ctx, "/identity.v1.IdentityService/Ping", &emptypb.Empty{}, out)
}

func codeOf(err error) codes.Code {
	s, _ := status.FromError(err)
	return s.Code()
}

func TestInternalAuth_ClaveCorrecta(t *testing.T) {
	conn := dialAuth(t, false, NewInternalAuth("secreto"))
	if err := invokePing(conn, MetadataInternalKey, "secreto"); err != nil {
		t.Fatalf("con key válida no debe fallar: %v", err)
	}
}

func TestInternalAuth_RechazaSinClaveOMala(t *testing.T) {
	conn := dialAuth(t, false, NewInternalAuth("secreto"))
	if got := codeOf(invokePing(conn)); got != codes.Unauthenticated {
		t.Fatalf("sin key: %v, se esperaba Unauthenticated", got)
	}
	if got := codeOf(invokePing(conn, MetadataInternalKey, "otra")); got != codes.Unauthenticated {
		t.Fatalf("key mala: %v, se esperaba Unauthenticated", got)
	}
}

func TestInternalAuth_SinConfiguracionFallaCerrado(t *testing.T) {
	conn := dialAuth(t, false, NewInternalAuth(""))
	if got := codeOf(invokePing(conn, MetadataInternalKey, "cualquiera")); got != codes.Internal {
		t.Fatalf("sin secreto: %v, se esperaba Internal (fail closed)", got)
	}
}

func TestLogging_MetodoYCodigoYSinSecretos(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	conn := dialAuth(t, false, NewLogging(logger), NewInternalAuth("secreto"))
	secreto := "tok-sensible-12345"
	if err := invokePing(conn, MetadataInternalKey, "secreto", "authorization", "Bearer "+secreto); err != nil {
		t.Fatalf("ping: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "/identity.v1.IdentityService/Ping") || !strings.Contains(got, "code=OK") {
		t.Fatalf("log OK inesperado: %q", got)
	}
	if strings.Contains(got, secreto) || strings.Contains(got, "tok-sensible") {
		t.Fatalf("el log filtró un secreto: %q", got)
	}

	buf.Reset()
	connFail := dialAuth(t, true, NewLogging(logger))
	if err := invokePing(connFail); codeOf(err) != codes.NotFound {
		t.Fatalf("se esperaba NotFound, got %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "code=NotFound") {
		t.Fatalf("log de error inesperado: %q", got)
	}
}

func TestChain_LoggingAuditaRechazo(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	conn := dialAuth(t, false, NewLogging(logger), NewInternalAuth("secreto"))
	if got := codeOf(invokePing(conn, MetadataInternalKey, "mala")); got != codes.Unauthenticated {
		t.Fatalf("se esperaba Unauthenticated, got %v", got)
	}
	if got := buf.String(); !strings.Contains(got, "code=Unauthenticated") {
		t.Fatalf("el rechazo debe quedar auditado: %q", got)
	}
}
