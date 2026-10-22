package fake

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// Client es un cliente gRPC de Identity que se conecta a un servidor falso
// en memoria (bufconn). Implementa los mismos métodos que identity.Client
// (HTTP) para poder intercambiarlos en tests sin tocar la red.
type Client struct {
	identityv1.IdentityServiceClient
	conn *grpc.ClientConn
}

// ErrInvalidToken indica que el token no es válido.
var ErrInvalidToken = errors.New("invalid token")

// NewClient crea un cliente gRPC conectado al servidor falso srv a través de
// bufconn (in-memory, sin red).
func NewClient(srv *Server) *Client {
	lis := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, srv)
	go grpcServer.Serve(lis) // nolint: errcheck

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		grpcServer.Stop()
		lis.Close()
		panic(err)
	}

	return &Client{
		IdentityServiceClient: identityv1.NewIdentityServiceClient(conn),
		conn:                  conn,
	}
}

// Close cierra la conexión y detiene el servidor gRPC.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// NewServerAndClient crea un servidor falso y un cliente conectado a él.
// Útil para tests que necesitan un IdentityService gRPC completo.
func NewServerAndClient() (*Server, *Client) {
	srv := NewServer()
	return srv, NewClient(srv)
}

// VerifyToken implementa la misma firma que identity.Client.VerifyToken.
func (c *Client) VerifyToken(ctx context.Context, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := c.IdentityServiceClient.VerifyToken(ctx, &identityv1.VerifyTokenRequest{Token: token})
	if err != nil {
		return "", err
	}
	if !resp.GetValid() || strings.TrimSpace(resp.GetUserId()) == "" {
		return "", ErrInvalidToken
	}
	return resp.GetUserId(), nil
}

// GetUser implementa la misma firma que identity.Client.GetUser.
func (c *Client) GetUser(ctx context.Context, id string) (*service.UserPublic, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := c.IdentityServiceClient.GetUser(ctx, &identityv1.GetUserRequest{UserId: id})
	if err != nil {
		return nil, err
	}
	return &service.UserPublic{
		UserID:      resp.GetUserId(),
		Email:       resp.GetEmail(),
		DisplayName: resp.DisplayName,
	}, nil
}
