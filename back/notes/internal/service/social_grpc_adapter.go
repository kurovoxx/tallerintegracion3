package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	socialv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/socialv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// SocialGRPCAdapter implementa SocialResolver y GroupMemberDirectory contra
// social.v1.SocialService (SOCIAL_GRPC_ADDR), con la misma semántica que
// SocialHTTPAdapter: NotFound = ausencia (no miembro / sin correos); el resto
// de errores se propaga para que el servicio decida (fail-closed en
// membresía y correos, RN-SHARE-10).
type SocialGRPCAdapter struct {
	client  socialv1.SocialServiceClient
	apiKey  string
	timeout time.Duration
}

var (
	_ SocialResolver       = (*SocialGRPCAdapter)(nil)
	_ GroupMemberDirectory = (*SocialGRPCAdapter)(nil)
)

// NewSocialGRPCAdapter crea el cliente (conexión perezosa: no falla si Social
// aún no levantó). Timeout <=0 usa 5s.
func NewSocialGRPCAdapter(addr string, timeout time.Duration, internalAPIKey string) (*SocialGRPCAdapter, error) {
	// ponytail: sin TLS, tráfico solo dentro de la red de compose/cluster.
	conn, err := grpc.NewClient(strings.TrimSpace(addr), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("social grpc: %w", err)
	}
	return newSocialGRPCAdapter(socialv1.NewSocialServiceClient(conn), timeout, internalAPIKey), nil
}

func newSocialGRPCAdapter(client socialv1.SocialServiceClient, timeout time.Duration, internalAPIKey string) *SocialGRPCAdapter {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SocialGRPCAdapter{client: client, apiKey: strings.TrimSpace(internalAPIKey), timeout: timeout}
}

func (a *SocialGRPCAdapter) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", a.apiKey), cancel
}

func (a *SocialGRPCAdapter) IsMember(ctx context.Context, userID, groupID string) (bool, error) {
	ctx, cancel := a.ctx(ctx)
	defer cancel()
	resp, err := a.client.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: groupID, UserId: userID})
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("social grpc: %w", err)
	}
	return resp.GetIsMember(), nil
}

func (a *SocialGRPCAdapter) IsAdmin(ctx context.Context, userID, groupID string) (bool, error) {
	ctx, cancel := a.ctx(ctx)
	defer cancel()
	resp, err := a.client.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: groupID, UserId: userID})
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("social grpc: %w", err)
	}
	return resp.GetIsAdmin(), nil
}

// GetFollowersCount: Social no tiene seguidores (ni HTTP ni gRPC); el
// servicio ya lo trata como best-effort, así que 0 es el mismo resultado.
func (a *SocialGRPCAdapter) GetFollowersCount(context.Context, string) (int, error) {
	return 0, nil
}

func (a *SocialGRPCAdapter) ListMemberEmails(ctx context.Context, groupID string) ([]string, error) {
	ctx, cancel := a.ctx(ctx)
	defer cancel()
	resp, err := a.client.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: groupID})
	if status.Code(err) == codes.NotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("social grpc: %w", err)
	}
	return resp.GetEmails(), nil
}
