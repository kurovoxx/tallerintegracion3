package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GRPCalendarGateway obtiene access tokens de Calendar vía
// IdentityService.GetOAuthToken (tarea 3_4_3). Misma interfaz y errores de
// dominio que AuthCalendarGateway, para intercambiarlos sin tocar llamantes.
//
// Traducción segura de Unauthenticated: la clave interna se valida al arrancar
// (ver Validate), así en operación un Unauthenticated significa conexión
// OAuth revocada, no clave mala. Sin esa validación previa no se puede
// distinguir: documentado, no adivinado.
type GRPCalendarGateway struct {
	Client  identityv1.IdentityServiceClient
	Key     string
	Timeout time.Duration
}

var _ CalendarTokenGateway = (*GRPCalendarGateway)(nil)

func (g *GRPCalendarGateway) timeout() time.Duration {
	if g.Timeout <= 0 {
		return 8 * time.Second
	}
	return g.Timeout
}

func (g *GRPCalendarGateway) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout())
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", strings.TrimSpace(g.Key)), cancel
}

// Validate comprueba clave y disponibilidad: VerifyToken con token vacío debe
// dar InvalidArgument (clave aceptada, servidor validando). Llamar una vez al
// arrancar en modo real; ante error, fallar rápido en main en vez de operar
// con una clave que falsearía todos los diagnósticos OAuth.
func (g *GRPCalendarGateway) Validate(ctx context.Context) error {
	ctx, cancel := g.ctx(ctx)
	defer cancel()
	_, err := g.Client.VerifyToken(ctx, &identityv1.VerifyTokenRequest{})
	if status.Code(err) != codes.InvalidArgument {
		return fmt.Errorf("identity grpc: validación inicial: %w", err)
	}
	return nil
}

func (g *GRPCalendarGateway) GetToken(ctx context.Context, userID string) (string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", ErrCalendarNotConnected
	}
	ctx, cancel := g.ctx(ctx)
	defer cancel()
	resp, err := g.Client.GetOAuthToken(ctx, &identityv1.GetOAuthTokenRequest{
		UserId:   userID,
		Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR,
	})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return "", ErrCalendarNotConnected
		case codes.Unauthenticated:
			// Clave validada al arrancar: es conexión revocada/inválida.
			return "", ErrCalendarConnectionInvalid
		default:
			return "", fmt.Errorf("%w: %v", ErrCalendarGatewayUnavailable, err)
		}
	}
	if strings.TrimSpace(resp.GetAccessToken()) == "" {
		return "", ErrCalendarGatewayUnavailable
	}
	return resp.GetAccessToken(), nil
}

// ReportRevoked avisa a Auth que Google rechazó el token (401/403) para que marque revoked_at.
// Best-effort: un fallo aquí solo se propaga como error para log, nunca afecta la reunión.
func (g *GRPCalendarGateway) ReportRevoked(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	ctx, cancel := g.ctx(ctx)
	defer cancel()
	_, err := g.Client.ReportCalendarRevoked(ctx, &identityv1.ReportCalendarRevokedRequest{UserId: userID})
	if err != nil {
		return fmt.Errorf("identity grpc: report revoked: %w", err)
	}
	return nil
}

// DialIdentityClient abre la conexión gRPC compartida hacia Identity
// (ej. "auth:9080"). Sin TLS: solo red interna de compose/cluster.
func DialIdentityClient(addr string) (identityv1.IdentityServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(strings.TrimSpace(addr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("identity grpc: %w", err)
	}
	return identityv1.NewIdentityServiceClient(conn), conn, nil
}
