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

// Servicio falso mínimo para probar interceptores sin protos ni red.
// Usa mensajes protobuf reales (empty/string) para el codec por defecto.

type fakeSvc struct{}

// fakeSvcIface existe porque grpc exige que HandlerType sea interfaz.
type fakeSvcIface interface{}

func fakeHandler(fail bool) func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
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
			FullMethod: "/test.v1.Test/Ping",
		}, call)
	}
}

var (
	fakeDescOK = &grpc.ServiceDesc{
		ServiceName: "test.v1.Test",
		HandlerType: (*fakeSvcIface)(nil),
		Methods:     []grpc.MethodDesc{{MethodName: "Ping", Handler: fakeHandler(false)}},
	}
	fakeDescFail = &grpc.ServiceDesc{
		ServiceName: "test.v1.Test",
		HandlerType: (*fakeSvcIface)(nil),
		Methods:     []grpc.MethodDesc{{MethodName: "Ping", Handler: fakeHandler(true)}},
	}
)

// dial arma servidor bufconn + cliente con la cadena dada.
func dial(t *testing.T, desc *grpc.ServiceDesc, interceptors ...grpc.UnaryServerInterceptor) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(interceptors...))
	s.RegisterService(desc, &fakeSvc{})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func callPing(conn *grpc.ClientConn, key string) error {
	ctx := context.Background()
	if key != "" {
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs(MetadataInternalKey, key))
	}
	out := new(wrapperspb.StringValue)
	return conn.Invoke(ctx, "/test.v1.Test/Ping", &emptypb.Empty{}, out)
}

func codeOf(err error) codes.Code {
	s, _ := status.FromError(err)
	return s.Code()
}

func TestInternalAuth_AcceptsCorrectKey(t *testing.T) {
	conn := dial(t, fakeDescOK, NewInternalAuth("secreto"))
	if err := callPing(conn, "secreto"); err != nil {
		t.Fatalf("con key válida no debe fallar: %v", err)
	}
}

func TestInternalAuth_RejectsMissingOrWrongKey(t *testing.T) {
	conn := dial(t, fakeDescOK, NewInternalAuth("secreto"))
	if got := codeOf(callPing(conn, "")); got != codes.Unauthenticated {
		t.Fatalf("sin key: code = %v, se esperaba Unauthenticated", got)
	}
	if got := codeOf(callPing(conn, "otra")); got != codes.Unauthenticated {
		t.Fatalf("key mala: code = %v, se esperaba Unauthenticated", got)
	}
}

func TestInternalAuth_MisconfiguredFailsClosed(t *testing.T) {
	conn := dial(t, fakeDescOK, NewInternalAuth(""))
	if got := codeOf(callPing(conn, "cualquiera")); got != codes.Internal {
		t.Fatalf("sin secreto configurado: code = %v, se esperaba Internal (fail closed)", got)
	}
}

func TestLogging_EmitsMethodAndCode(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	conn := dial(t, fakeDescOK, NewLogging(logger))
	if err := callPing(conn, ""); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "/test.v1.Test/Ping") || !strings.Contains(got, "code=OK") {
		t.Fatalf("log OK inesperado: %q", got)
	}

	buf.Reset()
	connFail := dial(t, fakeDescFail, NewLogging(logger))
	if err := callPing(connFail, ""); codeOf(err) != codes.NotFound {
		t.Fatalf("se esperaba NotFound, got %v", err)
	}
	if got := buf.String(); !strings.Contains(got, "/test.v1.Test/Ping") || !strings.Contains(got, "code=NotFound") {
		t.Fatalf("log de error inesperado: %q", got)
	}
}

func TestChain_LoggingAuditsAuthRejection(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	conn := dial(t, fakeDescOK, NewLogging(logger), NewInternalAuth("secreto"))
	if got := codeOf(callPing(conn, "mala")); got != codes.Unauthenticated {
		t.Fatalf("se esperaba Unauthenticated, got %v", got)
	}
	// Logging afuera: el rechazo quedó auditado.
	if got := buf.String(); !strings.Contains(got, "/test.v1.Test/Ping") || !strings.Contains(got, "code=Unauthenticated") {
		t.Fatalf("el rechazo debe quedar en el log: %q", got)
	}
}
