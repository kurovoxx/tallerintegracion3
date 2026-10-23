package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationAccountDeletionKeepsDurableGroupWorkAndSuccession(t *testing.T) {
	groups, pool := testRepo(t)
	ctx := context.Background()
	owner, successor := uuid.NewString(), uuid.NewString()
	recoverable := NewAccountDeletionRepository(pool)

	sharedGroup := newTestGroup(t, groups, owner)
	joinUser(t, groups, sharedGroup, successor)
	lastMemberGroup := newTestGroup(t, groups, owner)

	if err := recoverable.Begin(ctx, owner); err != nil {
		t.Fatalf("begin deletion snapshot: %v", err)
	}
	work, err := recoverable.PendingGroups(ctx, owner)
	if err != nil || len(work) != 2 {
		t.Fatalf("durable group snapshot = %d, %v; want two groups", len(work), err)
	}
	if _, active, err := recoverable.AcquireRequest(ctx, owner); err != nil || active {
		t.Fatalf("old JWT should be gated, active=%v err=%v", active, err)
	}

	result, err := recoverable.RemoveMembership(ctx, owner, sharedGroup.ID)
	if err != nil {
		t.Fatalf("remove admin from shared group: %v", err)
	}
	if result.GroupDeleted || result.PromotedUserID == nil || *result.PromotedUserID != successor {
		t.Fatalf("administrator succession result = %+v", result)
	}
	if err := recoverable.MarkMembershipRemoved(ctx, owner, sharedGroup.ID, result.GroupDeleted); err != nil {
		t.Fatal(err)
	}

	last, err := recoverable.RemoveMembership(ctx, owner, lastMemberGroup.ID)
	if err != nil {
		t.Fatalf("remove last group member: %v", err)
	}
	if !last.GroupDeleted {
		t.Fatal("last-member group was not deleted")
	}
	if err := recoverable.MarkMembershipRemoved(ctx, owner, lastMemberGroup.ID, true); err != nil {
		t.Fatal(err)
	}

	// The task survives both disappearance of the membership rows and deletion
	// of the last-member group until Notes confirms its part.
	var membershipCount, groupCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM social.group_memberships WHERE user_id=$1::uuid`, owner).Scan(&membershipCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM social.account_deletion_groups WHERE user_id=$1::uuid AND status='pending'`, owner).Scan(&groupCount); err != nil {
		t.Fatal(err)
	}
	if membershipCount != 0 || groupCount != 2 {
		t.Fatalf("memberships=%d durable pending groups=%d; want 0 and 2", membershipCount, groupCount)
	}
	if err := recoverable.CompleteGroup(ctx, owner, sharedGroup.ID); err == nil {
		t.Fatal("group must remain pending until Stream confirms removal")
	}
	for _, groupID := range []string{sharedGroup.ID, lastMemberGroup.ID} {
		if err := recoverable.MarkStreamRemoved(ctx, owner, groupID); err != nil {
			t.Fatalf("mark Stream removal for %s: %v", groupID, err)
		}
	}
	if err := recoverable.CompleteGroup(ctx, owner, sharedGroup.ID); err != nil {
		t.Fatal(err)
	}
	if err := recoverable.CompleteGroup(ctx, owner, lastMemberGroup.ID); err != nil {
		t.Fatal(err)
	}
	if err := recoverable.Complete(ctx, owner); err != nil {
		t.Fatal(err)
	}
	complete, err := recoverable.IsComplete(ctx, owner)
	if err != nil || !complete {
		t.Fatalf("complete tombstone = %v, %v", complete, err)
	}
}

func TestIntegrationAccountDeletionFenceDrainsInflightRequestAndRejectsLaterWrites(t *testing.T) {
	_, pool := testRepo(t)
	ctx := context.Background()
	userID := uuid.NewString()
	deletion := NewAccountDeletionRepository(pool)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM social.account_deletion_users WHERE user_id=$1::uuid`, userID)
	})

	release, active, err := deletion.AcquireRequest(ctx, userID)
	if err != nil || !active {
		t.Fatalf("acquire request fence: active=%v err=%v", active, err)
	}
	beginDone := make(chan error, 1)
	go func() { beginDone <- deletion.Begin(ctx, userID) }()
	select {
	case err := <-beginDone:
		t.Fatalf("deletion fence passed an in-flight request: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err := <-beginDone:
		if err != nil {
			t.Fatalf("begin deletion after request drained: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deletion fence did not continue after request release")
	}
	_, active, err = deletion.AcquireRequest(ctx, userID)
	if err != nil || active {
		t.Fatalf("new request accepted after deletion fence: active=%v err=%v", active, err)
	}
}

func TestIntegrationAccountGateDoesNotConsumeBusinessPool(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOCIAL_TEST_DATABASE_URL not configured")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	deletion := NewAccountDeletionRepository(pool)
	t.Cleanup(deletion.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var releases []func()
	for range 2 {
		release, active, err := deletion.AcquireRequest(ctx, uuid.NewString())
		if err != nil || !active {
			t.Fatalf("request gate active=%v err=%v", active, err)
		}
		releases = append(releases, release)
	}
	// The authenticated actor has two simultaneous requests holding the actor
	// lock pool; a third-party target check must still have its own lock slot.
	targetRelease, active, err := deletion.AcquireTargetRequest(ctx, uuid.NewString())
	if err != nil || !active {
		t.Fatalf("target gate active=%v err=%v", active, err)
	}
	releases = append(releases, targetRelease)
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	var one int
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("business pool blocked while two gate requests were active: value=%d err=%v", one, err)
	}
}
