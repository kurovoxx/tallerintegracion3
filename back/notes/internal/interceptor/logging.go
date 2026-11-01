package interceptor

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// NewLogging registra método, duración y código de cada RPC unary.
// Equivalente gRPC de logOp/fail HTTP. El logger se inyecta para poder
// capturarlo en tests; nil usa log.Default().
//
// Copia local a propósito (el microservicio Notes no depende de Social ni
// viceversa; ver documentation/docMartin.md §3_5_3): al copiar a otro
// servicio, mantener idéntico.
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
