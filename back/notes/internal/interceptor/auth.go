// Package interceptor provee los interceptores gRPC del servicio notes
// (tarea 2): auth JWT de usuario + auth interno servicio-a-servicio
// (x-internal-key, solo UnshareAll) + logging.
//
// A diferencia de Social (todo interno), Notes expone RPCs de usuario: la
// mayoría se autentica con el JWT del llamante (metadata "authorization") y
// solo UnshareAll, que consume Social, usa el secreto compartido. La
// separación es deliberada para no reintroducir el IDOR que en el HTTP se
// parchea validando el JWT de usuario en el handler.
package interceptor

import (
	"context"
	"crypto/subtle"
	"strings"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataInternalKey es la metadata gRPC equivalente al header HTTP
// X-Internal-Key (ver back/social/internal/interceptor/auth.go).
const MetadataInternalKey = "x-internal-key"

// MetadataAuthorization es la metadata gRPC que lleva el JWT de usuario,
// con el mismo formato que el header HTTP: "Bearer {token}".
const MetadataAuthorization = "authorization"

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

// NewJWTAuth valida el access token HS256 del llamante desde la metadata
// "authorization: Bearer {token}" y publica el user_id en el contexto con el
// mismo canal que el middleware HTTP (middleware.WithUserID), de modo que los
// handlers gRPC y HTTP resuelven la identidad igual. Sin token, malformado,
// expirado o inválido -> Unauthenticated.
func NewJWTAuth(validator middleware.JWTValidator, gates ...middleware.AccountAccessGate) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		raw := md.Get(MetadataAuthorization)
		if len(raw) == 0 || strings.TrimSpace(raw[0]) == "" {
			return nil, status.Error(codes.Unauthenticated, "authorization requerido")
		}
		auth := strings.TrimSpace(raw[0])
		if !strings.HasPrefix(auth, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "formato debe ser Bearer {token}")
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "token vacío")
		}
		userID, role, err := validator.ValidarAccessToken(token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "token inválido o expirado")
		}
		ctx = middleware.WithUserID(ctx, userID)
		ctx = middleware.WithRole(ctx, role)
		if len(gates) > 0 && gates[0] != nil {
			release, active, gateErr := gates[0].AcquireRequest(ctx, userID)
			if gateErr != nil {
				return nil, status.Error(codes.Unavailable, "account state unavailable")
			}
			if !active {
				return nil, status.Error(codes.PermissionDenied, "account deletion in progress")
			}
			defer release()
		}
		return handler(ctx, req)
	}
}

// NewAuthRouter enruta la autenticación por método: UnshareAll (servicio-a-
// servicio) usa el interceptor interno; el resto usa JWT de usuario. Así el
// servidor monta un solo interceptor de auth encadenado tras el logging.
func NewAuthRouter(jwt, internal grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if info.FullMethod == notesv1.NotesService_UnshareAll_FullMethodName ||
			info.FullMethod == notesv1.NotesService_BeginAccountDeletion_FullMethodName ||
			info.FullMethod == notesv1.NotesService_AccountDeletionCleanup_FullMethodName {
			return internal(ctx, req, info, handler)
		}
		return jwt(ctx, req, info, handler)
	}
}
