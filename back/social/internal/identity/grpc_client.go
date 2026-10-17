package identity

import (
	"context"
	"fmt"
	"strings"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// MetadataInternalKey equivale al header HTTP X-Internal-Key
// (misma convención que back/auth).
const MetadataInternalKey = "x-internal-key"

// GRPCClient consume IdentityService por gRPC (tarea 3_4_3). Reemplaza al
// Client HTTP para VerifyToken/GetUser con la misma API y los mismos errores
// de dominio (ErrInvalidToken, ErrUserNotFound).
//
// La conexión es perezosa y compartida: no falla si Auth aún no levantó.
// Sin TLS, tráfico solo dentro de la red de compose/cluster.
// Fail-closed: cualquier fallo de red o status inesperado es error,
// nunca éxito silencioso. No registra tokens ni claves.
type GRPCClient struct {
	client  identityv1.IdentityServiceClient
	conn    *grpc.ClientConn
	key     string
	timeout time.Duration
}

// DialConn abre la conexión gRPC compartida hacia Identity (ej. "auth:9080").
// Perezosa: no falla si Auth aún no levantó (solo una dirección malformada
// es error). Vive lo que el proceso; cerrarla es idempotente.
func DialConn(addr string) (*grpc.ClientConn, error) {
	// ponytail: sin TLS, tráfico solo dentro de la red de compose/cluster.
	conn, err := grpc.NewClient(strings.TrimSpace(addr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("identity grpc: %w", err)
	}
	return conn, nil
}

// NewGRPCClientWithConn envuelve una conexión compartida (ver DialConn).
// Timeout <=0 usa 5s.
func NewGRPCClientWithConn(conn *grpc.ClientConn, internalKey string, timeout time.Duration) *GRPCClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &GRPCClient{
		client:  identityv1.NewIdentityServiceClient(conn),
		conn:    conn,
		key:     strings.TrimSpace(internalKey),
		timeout: timeout,
	}
}

// DialIdentityGRPC conecta con Identity (ej. "auth:9080") con conexión propia.
// Timeout <=0 usa 5s.
func DialIdentityGRPC(addr, internalKey string, timeout time.Duration) (*GRPCClient, error) {
	conn, err := DialConn(addr)
	if err != nil {
		return nil, err
	}
	return NewGRPCClientWithConn(conn, internalKey, timeout), nil
}

// Close libera la conexión compartida.
func (c *GRPCClient) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *GRPCClient) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	return metadata.AppendToOutgoingContext(ctx, MetadataInternalKey, c.key), cancel
}

// Mensajes de dominio emitidos por el servidor Auth que distinguen un token
// de usuario inválido de un fallo de autenticación interna. Dependen del
// texto exacto del servidor (ver back/auth/internal/handler/grpc/
// identity_server.go y su interceptor); si Auth los cambia, estos casos caen
// al error de infraestructura, nunca a un falso inválido (fail-closed).
const (
	msgTokenInvalido = "token inválido"
	msgTokenExpirado = "token expirado"
)

// isTokenRejected dice si un Unauthenticated corresponde al JWT del usuario
// (y no a la clave interna): solo los dos mensajes de dominio del servidor.
func isTokenRejected(err error) bool {
	if status.Code(err) != codes.Unauthenticated {
		return false
	}
	msg, _ := status.FromError(err)
	return msg.Message() == msgTokenInvalido || msg.Message() == msgTokenExpirado
}

// VerifyToken valida un JWT de usuario contra Identity.
// Devuelve el user_id si es válido; ErrInvalidToken solo si Identity rechazó
// el token del usuario. Un fallo de clave interna o de red es error de
// infraestructura, nunca un falso inválido.
func (c *GRPCClient) VerifyToken(ctx context.Context, token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", ErrInvalidToken
	}
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.client.VerifyToken(ctx, &identityv1.VerifyTokenRequest{Token: token})
	if err != nil {
		if isTokenRejected(err) {
			return "", ErrInvalidToken
		}
		return "", fmt.Errorf("identity grpc: verify: %w", err)
	}
	if !resp.GetValid() || strings.TrimSpace(resp.GetUserId()) == "" {
		return "", ErrInvalidToken
	}
	return resp.GetUserId(), nil
}

// GetUser resuelve un usuario por id. Devuelve ErrUserNotFound si no existe.
func (c *GRPCClient) GetUser(ctx context.Context, id string) (*service.UserPublic, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrUserNotFound
	}
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	resp, err := c.client.GetUser(ctx, &identityv1.GetUserRequest{UserId: strings.TrimSpace(id)})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("identity grpc: get user: %w", err)
	}
	return &service.UserPublic{
		UserID:      resp.GetUserId(),
		Email:       resp.GetEmail(),
		DisplayName: resp.DisplayName,
	}, nil
}
