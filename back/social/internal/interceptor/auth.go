// Package interceptor provee los interceptores gRPC del servicio social
// (tarea 3_5_3): auth interno servicio-a-servicio + logging.
//
// Copia local a propósito: los microservicios no dependen entre sí
// (ver documentation/docMartin.md §3_5_3). Al copiar a otro servicio,
// mantener idéntico.
//
// Uso típico en el servidor (logging afuera para auditar también rechazos):
//
//	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
//		interceptor.NewLogging(log.Default()),
//		interceptor.NewInternalAuth(cfg.InternalKey),
//	))
package interceptor

import (
	"context"
	"crypto/subtle"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataInternalKey es la metadata gRPC equivalente al header HTTP
// X-Internal-Key (ver back/auth/internal/middleware/internal_middleware.go).
const MetadataInternalKey = "x-internal-key"

// NewInternalAuth exige el secreto compartido servicio-a-servicio en la
// metadata de cada RPC unary. Falla cerrado: sin secreto configurado responde
// Internal (503 HTTP equivalente); sin key o key incorrecta, Unauthenticated.
func NewInternalAuth(expectedSecret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if expectedSecret == "" {
			return nil, status.Error(codes.Internal, "internal auth not configured")
		}
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		keys := md.Get(MetadataInternalKey)
		if len(keys) == 0 || subtle.ConstantTimeCompare([]byte(keys[0]), []byte(expectedSecret)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid internal key")
		}
		return handler(ctx, req)
	}
}
