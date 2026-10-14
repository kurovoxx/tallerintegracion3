package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// GRPCalendarGateway obtiene access tokens de Calendar vía
// IdentityService.GetOAuthToken (tarea 3_4_3). Misma interfaz y errores de
// dominio que AuthCalendarGateway, para intercambiarlos sin tocar llamantes.
//
// Traducción segura de Unauthenticated: solo el mensaje de dominio
// "calendar_connection_invalid" significa OAuth revocado; cualquier otro
// Unauthenticated (clave mala/ausente, "missing metadata", etc.) es error de
// infraestructura, nunca falsa revocación. Los mensajes dependen del texto
// del servidor Auth (ver back/auth/internal/handler/grpc/identity_server.go
// y su interceptor); si cambian, esos casos caen a Unavailable (fail-closed).
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
			if oauthRevoked(err) {
				return "", ErrCalendarConnectionInvalid
			}
			return "", fmt.Errorf("%w: %v", ErrCalendarGatewayUnavailable, err)
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

// oauthRevoked dice si un Unauthenticated corresponde a OAuth revocado (y no
// a la clave interna): solo el mensaje de dominio del servidor Auth.
func oauthRevoked(err error) bool {
	if status.Code(err) != codes.Unauthenticated {
		return false
	}
	msg, _ := status.FromError(err)
	return msg.Message() == "calendar_connection_invalid"
}
