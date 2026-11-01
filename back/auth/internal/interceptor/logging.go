package interceptor

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// NewLogging registra método, duración y código de cada RPC unary.
// Equivalente gRPC del log de operaciones HTTP. El logger se inyecta para
// poder capturarlo en tests; nil usa log.Default(). Nunca registra el
// contenido de requests ni metadata (pueden traer tokens).
func NewLogging(logger *log.Logger) grpc.UnaryServerInterceptor {
	if logger == nil {
		logger = log.Default()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		logger.Printf("[grpc] %s code=%s dur=%s", info.FullMethod, status.Code(err), time.Since(start))
		return resp, err
	}
}
