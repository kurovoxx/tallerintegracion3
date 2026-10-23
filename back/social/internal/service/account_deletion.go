package service

import (
	"context"
	"fmt"
	"time"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/notesv1"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

type NotesAccountCleanup interface {
	UnshareAll(context.Context, *notesv1.UnshareAllRequest, ...grpc.CallOption) (*notesv1.UnshareAllResponse, error)
}

type AccountDeletionService struct {
	store *repository.AccountDeletionRepository
	notes NotesAccountCleanup
	key   string
	conn  *grpc.ClientConn
}

func NewAccountDeletionService(store *repository.AccountDeletionRepository, notesAddr, internalKey string) (*AccountDeletionService, error) {
	if notesAddr == "" {
		return nil, fmt.Errorf("notes account cleanup address required")
	}
	conn, err := grpc.NewClient(notesAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &AccountDeletionService{store: store, notes: notesv1.NewNotesServiceClient(conn), key: internalKey, conn: conn}, nil
}

func (s *AccountDeletionService) Close() {
	if s.conn != nil {
		_ = s.conn.Close()
	}
}

func (s *AccountDeletionService) Prepare(ctx context.Context, userID string) (socialv1.AccountCleanupStatus, error) {
	if err := s.store.Begin(ctx, userID); err != nil {
		return socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR, err
	}
	complete, err := s.store.IsComplete(ctx, userID)
	if err != nil {
		return socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR, err
	}
	if complete {
		return socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE, nil
	}
	return socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, nil
}

func (s *AccountDeletionService) Cleanup(ctx context.Context, userID string) ([]model.SuccessionResult, socialv1.AccountCleanupStatus, error) {
	if _, err := s.Prepare(ctx, userID); err != nil {
		return nil, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR, err
	}
	work, err := s.store.PendingGroups(ctx, userID)
	if err != nil {
		return nil, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR, err
	}
	results := make([]model.SuccessionResult, 0, len(work))
	for _, item := range work {
		if err := ctx.Err(); err != nil {
			return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, err
		}
		groupDeleted := item.GroupDeleted
		if !item.MembershipRemoved {
			result, removeErr := s.store.RemoveMembership(ctx, userID, item.GroupID)
			if removeErr != nil {
				return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, removeErr
			}
			groupDeleted = groupDeleted || result.GroupDeleted
			if err := s.store.MarkMembershipRemoved(ctx, userID, item.GroupID, groupDeleted); err != nil {
				return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, err
			}
			results = append(results, result)
		}
		rpcCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		rpcCtx = metadata.AppendToOutgoingContext(rpcCtx, "x-internal-key", s.key)
		response, callErr := s.notes.UnshareAll(rpcCtx, &notesv1.UnshareAllRequest{
			UserId: userID, GroupId: item.GroupID, GroupDeleted: groupDeleted,
		})
		cancel()
		if callErr != nil {
			return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, callErr
		}
		// Older Notes servers return status UNSPECIFIED; never treat that as a
		// completed permission revocation.
		if response.GetStatus() != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
			if response.GetStatus() == notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR {
				return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR, fmt.Errorf("notes reported account cleanup error")
			}
			return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, nil
		}
		if err := s.store.CompleteGroup(ctx, userID, item.GroupID); err != nil {
			return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, err
		}
	}
	if err := s.store.Complete(ctx, userID); err != nil {
		return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING, err
	}
	return results, socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE, nil
}

func (s *AccountDeletionService) AcquireRequest(ctx context.Context, userID string) (func(), bool, error) {
	return s.store.AcquireRequest(ctx, userID)
}
