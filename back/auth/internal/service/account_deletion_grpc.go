package service

import (
	"context"
	"fmt"
	"time"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/notesv1"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/auth/internal/gen/socialv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// GRPCAccountCleanup speaks only the additive, explicit cleanup contracts.
// Old servers return UNSPECIFIED or Unimplemented, both of which remain
// unconfirmed and can never authorize local Identity deletion.
type GRPCAccountCleanup struct {
	social socialv1.SocialServiceClient
	notes  notesv1.NotesServiceClient
	key    string
	conns  []*grpc.ClientConn
}

func NewGRPCAccountCleanup(socialAddr, notesAddr, key string) (*GRPCAccountCleanup, error) {
	if socialAddr == "" || notesAddr == "" {
		return nil, fmt.Errorf("account deletion gRPC addresses are required")
	}
	socialConn, err := grpc.NewClient(socialAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial social account cleanup: %w", err)
	}
	notesConn, err := grpc.NewClient(notesAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = socialConn.Close()
		return nil, fmt.Errorf("dial notes account cleanup: %w", err)
	}
	return &GRPCAccountCleanup{
		social: socialv1.NewSocialServiceClient(socialConn),
		notes:  notesv1.NewNotesServiceClient(notesConn),
		key:    key,
		conns:  []*grpc.ClientConn{socialConn, notesConn},
	}, nil
}

func (g *GRPCAccountCleanup) Close() {
	for _, conn := range g.conns {
		_ = conn.Close()
	}
}

func (g *GRPCAccountCleanup) BeginNotes(ctx context.Context, userID string) AccountCleanupResult {
	ctx, cancel := cleanupRPCContext(ctx)
	defer cancel()
	response, err := g.notes.BeginAccountDeletion(withCleanupKey(ctx, g.key), &notesv1.BeginAccountDeletionRequest{UserId: userID})
	if err != nil {
		return AccountCleanupResult{Status: AccountCleanupError, Err: err}
	}
	return mapNotesCleanupStatus(response.GetStatus())
}

func (g *GRPCAccountCleanup) BeginSocial(ctx context.Context, userID string) AccountCleanupResult {
	ctx, cancel := cleanupRPCContext(ctx)
	defer cancel()
	response, err := g.social.PrepareAccountDeletion(withCleanupKey(ctx, g.key), &socialv1.PrepareAccountDeletionRequest{UserId: userID})
	if err != nil {
		return AccountCleanupResult{Status: AccountCleanupError, Err: err}
	}
	switch response.GetStatus() {
	case socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE:
		return AccountCleanupResult{Status: AccountCleanupComplete}
	case socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING:
		return AccountCleanupResult{Status: AccountCleanupPending}
	case socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR:
		return AccountCleanupResult{Status: AccountCleanupError, Err: fmt.Errorf("social gate could not be persisted")}
	default:
		return AccountCleanupResult{Status: AccountCleanupUnspecified, Err: fmt.Errorf("social gate response was not explicit")}
	}
}

func (g *GRPCAccountCleanup) CleanupSocial(ctx context.Context, userID string) AccountCleanupResult {
	ctx, cancel := cleanupRPCContext(ctx)
	defer cancel()
	response, err := g.social.AccountDeletionCleanup(withCleanupKey(ctx, g.key), &socialv1.AccountDeletionCleanupRequest{UserId: userID})
	if err != nil {
		return AccountCleanupResult{Status: AccountCleanupError, Err: err}
	}
	switch response.GetStatus() {
	case socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE:
		return AccountCleanupResult{Status: AccountCleanupComplete}
	case socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING:
		return AccountCleanupResult{Status: AccountCleanupPending}
	case socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR:
		return AccountCleanupResult{Status: AccountCleanupError, Err: fmt.Errorf("social cleanup reported error")}
	default:
		return AccountCleanupResult{Status: AccountCleanupUnspecified, Err: fmt.Errorf("social cleanup response was not explicit")}
	}
}

func (g *GRPCAccountCleanup) CleanupNotes(ctx context.Context, userID string) AccountCleanupResult {
	ctx, cancel := cleanupRPCContext(ctx)
	defer cancel()
	response, err := g.notes.AccountDeletionCleanup(withCleanupKey(ctx, g.key), &notesv1.AccountDeletionCleanupRequest{UserId: userID})
	if err != nil {
		return AccountCleanupResult{Status: AccountCleanupError, Err: err}
	}
	return mapNotesCleanupStatus(response.GetStatus())
}

func mapNotesCleanupStatus(status notesv1.AccountCleanupStatus) AccountCleanupResult {
	switch status {
	case notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE:
		return AccountCleanupResult{Status: AccountCleanupComplete}
	case notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING:
		return AccountCleanupResult{Status: AccountCleanupPending}
	case notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR:
		return AccountCleanupResult{Status: AccountCleanupError, Err: fmt.Errorf("notes cleanup reported error")}
	default:
		return AccountCleanupResult{Status: AccountCleanupUnspecified, Err: fmt.Errorf("notes cleanup response was not explicit")}
	}
}

func cleanupRPCContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 30*time.Second)
}

func withCleanupKey(ctx context.Context, key string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", key)
}
