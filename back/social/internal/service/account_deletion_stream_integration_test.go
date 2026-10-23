package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
)

func TestIntegrationAccountDeletionStreamFailureSurvivesServiceRestart(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOCIAL_TEST_DATABASE_URL no definida")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := repository.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	targetID, remainingID := uuid.NewString(), uuid.NewString()
	groups := repository.NewGroupRepository(pool)
	group, err := groups.CreateWithOwner(ctx, "stream-retry-"+uuid.NewString()[:8], nil, targetID)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM social.account_deletion_users WHERE user_id=$1::uuid`, targetID)
		_ = groups.DeleteGroup(context.Background(), group.ID)
	})
	if err := groups.JoinWithInviteToken(ctx, group.ID, group.InviteToken, remainingID); err != nil {
		t.Fatalf("join remaining member: %v", err)
	}

	channelID := ChannelIDForGroup(group.ID)
	chat := &retryableChat{
		failuresRemaining: 1,
		members: map[string]map[string]bool{
			channelID: {targetID: true, remainingID: true},
		},
	}
	deletionRepo := repository.NewAccountDeletionRepository(pool)
	defer func() { deletionRepo.Close() }()
	newService := func(store accountDeletionStore) *AccountDeletionService {
		svc := &AccountDeletionService{store: store, notes: completedNotesCleanup{}}
		svc.SetChatMembership(chat)
		return svc
	}

	_, status, err := newService(deletionRepo).Cleanup(ctx, targetID)
	if err == nil || status != socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING {
		t.Fatalf("first cleanup=(%s,%v), want pending Stream error", status, err)
	}
	var membershipRemoved, streamRemoved bool
	var groupStatus string
	if err := pool.QueryRow(ctx, `SELECT membership_removed,stream_removed,status FROM social.account_deletion_groups WHERE user_id=$1::uuid AND group_id=$2::uuid`, targetID, group.ID).Scan(&membershipRemoved, &streamRemoved, &groupStatus); err != nil {
		t.Fatalf("read durable group job: %v", err)
	}
	if !membershipRemoved || streamRemoved || groupStatus != "pending" {
		t.Fatalf("job after Stream failure=(membership=%v stream=%v status=%s), want true/false/pending", membershipRemoved, streamRemoved, groupStatus)
	}
	if chat.hasMember(channelID, targetID) == false || chat.hasMember(channelID, remainingID) == false {
		t.Fatal("failed Stream request should preserve both members")
	}

	// Recreate both the service and repository facade. PostgreSQL retains the
	// pending stage; only the target member is removed on retry.
	deletionRepo.Close()
	deletionRepo = repository.NewAccountDeletionRepository(pool)
	_, status, err = newService(deletionRepo).Cleanup(ctx, targetID)
	if err != nil || status != socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("retry cleanup=(%s,%v), want COMPLETE", status, err)
	}
	if chat.hasMember(channelID, targetID) || !chat.hasMember(channelID, remainingID) {
		t.Fatal("retry must remove the target and preserve the remaining member")
	}
	role, err := groups.GetMemberRole(ctx, group.ID, remainingID)
	if err != nil || role != "admin" {
		t.Fatalf("remaining member role=%q, err=%v; want admin succession", role, err)
	}
	if err := pool.QueryRow(ctx, `SELECT stream_removed,status FROM social.account_deletion_groups WHERE user_id=$1::uuid AND group_id=$2::uuid`, targetID, group.ID).Scan(&streamRemoved, &groupStatus); err != nil {
		t.Fatalf("read completed group job: %v", err)
	}
	if !streamRemoved || groupStatus != "complete" {
		t.Fatalf("job after retry=(stream=%v status=%s), want true/complete", streamRemoved, groupStatus)
	}
	if got := chat.callCount(); got != 2 {
		t.Fatalf("Stream attempts=%d, want one failure and one successful retry", got)
	}

	_, status, err = newService(deletionRepo).Cleanup(ctx, targetID)
	if err != nil || status != socialv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("repeat cleanup=(%s,%v), want COMPLETE", status, err)
	}
	if got := chat.callCount(); got != 2 {
		t.Fatalf("completed cleanup repeated Stream side effect: %d attempts", got)
	}
}
